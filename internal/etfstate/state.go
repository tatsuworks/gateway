package etfstate

import (
	"bytes"
	"path"
	"sync"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/fasthttp/router"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type Server struct {
	log *zap.Logger

	router *router.Router

	fdb fdb.Database

	subs *Subspaces

	bufs sync.Pool
}

func NewServer(
	logger *zap.Logger,
) (*Server, error) {
	r := router.New()

	fdb.MustAPIVersion(510)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"etfstate"}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open directory")
	}

	return &Server{
		log:    logger,
		router: r,
		fdb:    db,
		subs:   NewSubspaces(dir),
		bufs: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}, nil
}

func (s *Server) Init() {
	base := "/v1/events"
	s.router.POST(path.Join(base, "guild_create"), wrapHandler(s.handleGuildCreate))
	s.router.POST(path.Join(base, "guild_update"), wrapHandler(s.handleGuildCreate))
	s.router.POST(path.Join(base, "guild_delete"), wrapHandler(s.handleGuildDelete))

	s.router.POST(path.Join(base, "guild_members_chunk"), wrapHandler(s.handleMemberChunk))
	s.router.POST(path.Join(base, "guild_member_add"), wrapHandler(s.handleMemberAdd))
	s.router.POST(path.Join(base, "guild_member_update"), wrapHandler(s.handleMemberAdd))
	s.router.POST(path.Join(base, "guild_member_remove"), wrapHandler(s.handleMemberRemove))

	s.router.POST(path.Join(base, "guild_channel_create"), wrapHandler(s.handleChannelCreate))
	s.router.POST(path.Join(base, "guild_channel_update"), wrapHandler(s.handleChannelCreate))
	s.router.POST(path.Join(base, "guild_channel_delete"), wrapHandler(s.handleChannelDelete))
}

func (s *Server) Start(addr string) error {
	srv := fasthttp.Server{
		Handler:            s.router.Handler,
		MaxRequestBodySize: fasthttp.DefaultMaxRequestBodySize * 1000,
	}
	return srv.ListenAndServe(addr)
}

// Subspaces is a struct containing all of the different subspaces used.
type Subspaces struct {
	Channels subspace.Subspace
	Guilds   subspace.Subspace
	Members  subspace.Subspace
	Messages subspace.Subspace
	Users    subspace.Subspace
	Roles    subspace.Subspace
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
	// UserSubspaceName is the enum for the user subspace.
	UserSubspaceName
	// RoleSubspaceName is the enum for the role subspace.
	RoleSubspaceName
)

// NewSubspaces returns an instantiated Subspaces with the correct subspaces.
func NewSubspaces(dir directory.DirectorySubspace) *Subspaces {
	return &Subspaces{
		Channels: dir.Sub(ChannelSubspaceName),
		Guilds:   dir.Sub(GuildSubspaceName),
		Members:  dir.Sub(MemberSubspaceName),
		Messages: dir.Sub(MessageSubspaceName),
		Users:    dir.Sub(UserSubspaceName),
		Roles:    dir.Sub(RoleSubspaceName),
	}
}

func (s *Server) getBuf() *bytes.Buffer {
	return s.bufs.Get().(*bytes.Buffer)
}

func (s *Server) putBuf(buf *bytes.Buffer) {
	buf.Reset()
	s.bufs.Put(buf)
}
