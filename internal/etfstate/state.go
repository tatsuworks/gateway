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
	"github.com/valyala/fasthttp/reuseport"
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
	s.router.POST(path.Join(base, "guild_ban_add"), wrapHandler(s.handleGuildBanAdd))
	s.router.POST(path.Join(base, "guild_ban_remove"), wrapHandler(s.handleGuildBanRemove))
	s.router.POST(path.Join(base, "guild_role_create"), wrapHandler(s.handleRoleCreate))
	s.router.POST(path.Join(base, "guild_role_update"), wrapHandler(s.handleRoleCreate))
	s.router.POST(path.Join(base, "guild_role_delete"), wrapHandler(s.handleRoleDelete))

	s.router.POST(path.Join(base, "guild_members_chunk"), wrapHandler(s.handleMemberChunk))
	s.router.POST(path.Join(base, "guild_member_add"), wrapHandler(s.handleMemberAdd))
	s.router.POST(path.Join(base, "guild_member_update"), wrapHandler(s.handleMemberAdd))
	s.router.POST(path.Join(base, "guild_member_remove"), wrapHandler(s.handleMemberRemove))
	s.router.POST(path.Join(base, "presence_update"), wrapHandler(s.handlePresenceUpdate))

	s.router.POST(path.Join(base, "channel_create"), wrapHandler(s.handleChannelCreate))
	s.router.POST(path.Join(base, "channel_update"), wrapHandler(s.handleChannelCreate))
	s.router.POST(path.Join(base, "channel_delete"), wrapHandler(s.handleChannelDelete))
	s.router.POST(path.Join(base, "voice_state_update"), wrapHandler(s.handleVoiceStateUpdate))

	s.router.POST(path.Join(base, "message_create"), wrapHandler(s.handleMessageCreate))
	s.router.POST(path.Join(base, "message_update"), wrapHandler(s.handleMessageCreate))
	s.router.POST(path.Join(base, "message_delete"), wrapHandler(s.handleMessageDelete))
	s.router.POST(path.Join(base, "message_reaction_add"), wrapHandler(s.handleMessageReactionAdd))
	s.router.POST(path.Join(base, "message_reaction_remove"), wrapHandler(s.handleMessageReactionRemove))
	s.router.POST(path.Join(base, "message_reaction_remove_all"), wrapHandler(s.handleMessageReactionRemoveAll))

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
	srv := fasthttp.Server{
		Handler:            s.router.Handler,
		MaxRequestBodySize: fasthttp.DefaultMaxRequestBodySize * 1000,
	}

	ln, err := reuseport.Listen("tcp4", "localhost:8080")
	if err != nil {
		return err
	}

	return srv.Serve(ln)
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

func (s *Server) getBuf() *bytes.Buffer {
	return s.bufs.Get().(*bytes.Buffer)
}

func (s *Server) putBuf(buf *bytes.Buffer) {
	buf.Reset()
	s.bufs.Put(buf)
}
