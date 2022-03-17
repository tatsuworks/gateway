package api

import (
	"net/http"
	"path"

	"cdr.dev/slog"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/valyala/fasthttp/reuseport"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/tatsuworks/gateway/internal/state"
)

var (
	FDBRangeWantAll = fdb.RangeOptions{Mode: fdb.StreamingModeWantAll}
)

type Server struct {
	log     slog.Logger
	version string

	router *httprouter.Router

	db  state.DB
	enc string
}

type EmptyObj struct {
	Id      string `json:"id"`
	IsEmpty bool   `json:"is_empty"`
}

func NewServer(
	logger slog.Logger,
	db state.DB,
	version string,
) (*Server, error) {
	return &Server{
		log:     logger,
		router:  httprouter.New(),
		version: version,
		db:      db,
		enc:     db.Encoding().Name(),
	}, nil
}

func (s *Server) Init() {
	base := "/v1/events"

	s.router.GET("/healthz", wrapHandler(s.log, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Version: " + s.version))
		return nil
	}))

	s.router.GET(path.Join(base, "users", ":user"), wrapHandler(s.log, s.getUser))
	// s.router.GET(path.Join(base, "channels"), wrapHandler(s.log, s.getChannels))
	s.router.GET(path.Join(base, "channels", ":channel"), wrapHandler(s.log, s.getChannel))
	s.router.GET(path.Join(base, "channels", ":channel", "messages", ":message"), wrapHandler(s.log, s.getChannelMessage))
	s.router.GET(path.Join(base, "guilds", ":guild"), wrapHandler(s.log, s.getGuild))
	s.router.GET(path.Join(base, "guilds", ":guild", "channels"), wrapHandler(s.log, s.getGuildChannels))
	s.router.GET(path.Join(base, "guilds", ":guild", "members"), wrapHandler(s.log, s.getGuildMembers))
	s.router.GET(path.Join(base, "guilds", ":guild", "members_with_role", ":role"), wrapHandler(s.log, s.getGuildMembersWithRole))
	s.router.GET(path.Join(base, "guilds", ":guild", "members", ":member"), wrapHandler(s.log, s.getGuildMember))
	s.router.GET(path.Join(base, "guilds", ":guild", "members", ":member", "presence"), wrapHandler(s.log, s.getUserPresence))
	s.router.GET(path.Join(base, "guilds", ":guild", "roles"), wrapHandler(s.log, s.getGuildRoles))
	s.router.GET(path.Join(base, "guilds", ":guild", "roles", ":role"), wrapHandler(s.log, s.getGuildRole))
	s.router.GET(path.Join(base, "guilds", ":guild", "emojis"), wrapHandler(s.log, s.getGuildEmojis))
	s.router.GET(path.Join(base, "guilds", ":guild", "emojis", ":emoji"), wrapHandler(s.log, s.getGuildEmoji))

	s.router.GET(path.Join(base, "guilds_count"), wrapHandler(s.log, s.getGuildCount))

	s.router.GET(path.Join(base, "threads"), wrapHandler(s.log, s.getThreads))
	s.router.GET(path.Join(base, "guilds", ":guild", "threads"), wrapHandler(s.log, s.getGuildThreads))
	s.router.GET(path.Join(base, "channels", ":channel", "threads"), wrapHandler(s.log, s.getChannelThreads))
	s.router.GET(path.Join(base, "threads", ":thread"), wrapHandler(s.log, s.getThread))
}

func (s *Server) Start(addr string) error {
	var (
		h1s = new(http.Server)
		h2s = new(http2.Server)
	)

	ln, err := reuseport.Listen("tcp4", addr)
	if err != nil {
		return err
	}

	h1s.Handler = h2c.NewHandler(s.router, h2s)
	return h1s.Serve(ln)
}
