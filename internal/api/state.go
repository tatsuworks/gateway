package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp/reuseport"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
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

	dir, err := directory.CreateOrOpen(db, []string{"etfstate"}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open directory")
	}

	return &Server{
		log:     logger,
		router:  httprouter.New(),
		version: version,
		fdb:     db,
		subs:    NewSubspaces(dir),
		bufs:    new(bytebufferpool.Pool),
	}, nil
}

func (s *Server) Init() {
	base := "/v1/events"

	s.router.GET("/v", wrapHandler(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		w.WriteHeader(http.StatusOK)
		//w.Write([]byte("Version: " + s.version))
		return nil
	}))

	s.router.POST(path.Join(base, strings.ToUpper("guild_create")), wrapHandler(s.handleGuildCreate))
	s.router.POST(path.Join(base, strings.ToUpper("guild_update")), wrapHandler(s.handleGuildCreate))
	s.router.POST(path.Join(base, strings.ToUpper("guild_delete")), wrapHandler(s.handleGuildDelete))
	s.router.POST(path.Join(base, strings.ToUpper("guild_ban_add")), wrapHandler(s.handleGuildBanAdd))
	s.router.POST(path.Join(base, strings.ToUpper("guild_ban_remove")), wrapHandler(s.handleGuildBanRemove))
	s.router.POST(path.Join(base, strings.ToUpper("guild_role_create")), wrapHandler(s.handleRoleCreate))
	s.router.POST(path.Join(base, strings.ToUpper("guild_role_update")), wrapHandler(s.handleRoleCreate))
	s.router.POST(path.Join(base, strings.ToUpper("guild_role_delete")), wrapHandler(s.handleRoleDelete))

	s.router.POST(path.Join(base, strings.ToUpper("guild_members_chunk")), wrapHandler(s.handleMemberChunk))
	s.router.POST(path.Join(base, strings.ToUpper("guild_member_add")), wrapHandler(s.handleMemberAdd))
	s.router.POST(path.Join(base, strings.ToUpper("guild_member_update")), wrapHandler(s.handleMemberAdd))
	s.router.POST(path.Join(base, strings.ToUpper("guild_member_remove")), wrapHandler(s.handleMemberRemove))
	s.router.POST(path.Join(base, strings.ToUpper("presence_update")), wrapHandler(s.handlePresenceUpdate))

	s.router.POST(path.Join(base, strings.ToUpper("channel_create")), wrapHandler(s.handleChannelCreate))
	s.router.POST(path.Join(base, strings.ToUpper("channel_update")), wrapHandler(s.handleChannelCreate))
	s.router.POST(path.Join(base, strings.ToUpper("channel_delete")), wrapHandler(s.handleChannelDelete))
	s.router.POST(path.Join(base, strings.ToUpper("voice_state_update")), wrapHandler(s.handleVoiceStateUpdate))

	s.router.POST(path.Join(base, strings.ToUpper("message_create")), wrapHandler(s.handleMessageCreate))
	s.router.POST(path.Join(base, strings.ToUpper("message_update")), wrapHandler(s.handleMessageCreate))
	s.router.POST(path.Join(base, strings.ToUpper("message_delete")), wrapHandler(s.handleMessageDelete))
	s.router.POST(path.Join(base, strings.ToUpper("message_reaction_add")), wrapHandler(s.handleMessageReactionAdd))
	s.router.POST(path.Join(base, strings.ToUpper("message_reaction_remove")), wrapHandler(s.handleMessageReactionRemove))
	s.router.POST(path.Join(base, strings.ToUpper("message_reaction_remove_all")), wrapHandler(s.handleMessageReactionRemoveAll))

	s.router.GET(path.Join(base, "channels", ":guild"), wrapHandler(s.getChannels))
	s.router.GET(path.Join(base, "channels", ":guild", ":channel"), wrapHandler(s.getChannel))
	s.router.GET(path.Join(base, "guilds", ":guild"), wrapHandler(s.getGuild))
	s.router.GET(path.Join(base, "roles", ":guild"), wrapHandler(s.getRoles))
	s.router.GET(path.Join(base, "roles", ":guild", ":role"), wrapHandler(s.getRole))
	s.router.GET(path.Join(base, "messages", ":channel", ":message"), wrapHandler(s.getMessage))
	s.router.GET(path.Join(base, "members", ":guild"), wrapHandler(s.getMembers))
	s.router.GET(path.Join(base, "members", ":guild", ":member"), wrapHandler(s.getMember))

	// fn := func(m *nats.Msg) {
	// 	termStart := time.Now()
	// 	ev, err := discordetf.DecodeT(m.Data)
	// 	if err != nil {
	// 		s.log.Error("failed to decode t", zap.Error(err))
	// 		return
	// 	}

	// 	p, err := discordetf.DecodePresence(ev.D)
	// 	if err != nil {
	// 		s.log.Error("failed to decode presence", zap.Error(err))
	// 		return
	// 	}

	// 	termStop := time.Since(termStart)
	// 	fdbStart := time.Now()

	// 	err = s.Transact(func(t fdb.Transaction) error {
	// 		t.Set(s.fmtPresenceKey(p.Guild, p.Id), p.Raw)
	// 		return nil
	// 	})
	// 	if err != nil {
	// 		s.log.Error("failed to fdb transact", zap.Error(err))
	// 		return
	// 	}

	// 	fdbStop := time.Since(fdbStart)
	// 	_ = termStop
	// 	_ = fdbStop
	// 	s.log.Info(
	// 		"finished presence_update",
	// 		zap.Duration("decode", termStop),
	// 		zap.Duration("fdb", fdbStop),
	// 		zap.Duration("total", termStop+fdbStop),
	// 	)
	// }

	// for range make([]struct{}, 5) {
	// 	nc, err := nats.Connect(nats.DefaultURL)
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	_, err = nc.QueueSubscribe("PRESENCE_UPDATE", "workers", fn)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }
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
	ChannelSubspaceName = iota
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
