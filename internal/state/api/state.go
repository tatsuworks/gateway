package api

import (
	"net/http"
	"path"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/julienschmidt/httprouter"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp/reuseport"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/xerrors"
)

var (
	FDBRangeWantAll = fdb.RangeOptions{Mode: fdb.StreamingModeWantAll}
)

type Server struct {
	log     *zap.Logger
	version string

	router *httprouter.Router

	fdb fdb.Database

	subs *Subspaces

	bufs *bytebufferpool.Pool
}

func NewServer(
	logger *zap.Logger,
	version string,
) (*Server, error) {
	fdb.MustAPIVersion(510)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"state"}, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to open fdb directory: %w", err)
	}

	return &Server{
		log:     logger,
		router:  httprouter.New(),
		version: version,
		fdb:     db,
		subs:    NewSubspaces(dir),
	}, nil
}

func (s *Server) Init() {
	base := "/v1/events"

	s.router.GET("/v", wrapHandler(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Version: " + s.version))
		return nil
	}))

	s.router.GET(path.Join(base, "channels"), wrapHandler(s.getChannels))
	s.router.GET(path.Join(base, "channels", ":channel"), wrapHandler(s.getChannel))
	s.router.GET(path.Join(base, "channels", ":channel", "messages", ":message"), wrapHandler(s.getChannelMessage))
	s.router.GET(path.Join(base, "guilds", ":guild"), wrapHandler(s.getGuild))
	s.router.GET(path.Join(base, "guilds", ":guild", "channels"), wrapHandler(s.getGuildChannels))
	s.router.GET(path.Join(base, "guilds", ":guild", "members"), wrapHandler(s.getGuildMembers))
	s.router.GET(path.Join(base, "guilds", ":guild", "members", ":member"), wrapHandler(s.getGuildMember))
	s.router.GET(path.Join(base, "guilds", ":guild", "roles"), wrapHandler(s.getGuildRoles))
	s.router.GET(path.Join(base, "guilds", ":guild", "roles", ":role"), wrapHandler(s.getGuildRole))
}

func (s *Server) Start(addr string) error {
	srv := new(http.Server)
	http2.ConfigureServer(srv, nil)

	ln, err := reuseport.Listen("tcp4", "0.0.0.0:8080")
	if err != nil {
		return err
	}

	srv.Handler = s.router
	return srv.ServeTLS(ln, "localhost.cert", "localhost.key")
}

// Subspaces is a struct containing all of the different subspaces used.
type Subspaces struct {
	Channels    subspace.Subspace
	Guilds      subspace.Subspace
	Members     subspace.Subspace
	Messages    subspace.Subspace
	Presences   subspace.Subspace
	Users       subspace.Subspace
	Roles       subspace.Subspace
	VoiceStates subspace.Subspace
}

// If new enums need to be added, always append. If you are deprecating an enum never delete it.
const (
	// ChannelSubspaceName is the enum for the channel subspace.
	ChannelSubspaceName uint8 = iota
	// GuildSubspaceName is the enum for the guild subspace.
	GuildSubspaceName
	// MemberSubspaceName is the enum for the member subspace.
	MemberSubspaceName
	// MessageSubspaceName is the enum for the message subspace.
	MessageSubspaceName
	// PresenceSubspaceName is the enum for the presence subspace.
	PresenceSubspaceName
	// UserSubspaceName is the enum for the user subspace.
	UserSubspaceName
	// RoleSubspaceName is the enum for the role subspace.
	RoleSubspaceName
	// VoiceStateSubspaceName is the enum for the voice state subspace.
	VoiceStateSubspaceName
)

// NewSubspaces returns an instantiated Subspaces with the correct subspaces.
func NewSubspaces(dir directory.DirectorySubspace) *Subspaces {
	return &Subspaces{
		Channels:    dir.Sub(ChannelSubspaceName),
		Guilds:      dir.Sub(GuildSubspaceName),
		Members:     dir.Sub(MemberSubspaceName),
		Messages:    dir.Sub(MessageSubspaceName),
		Presences:   dir.Sub(PresenceSubspaceName),
		Users:       dir.Sub(UserSubspaceName),
		Roles:       dir.Sub(RoleSubspaceName),
		VoiceStates: dir.Sub(VoiceStateSubspaceName),
	}
}
