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

	s.router.GET("/healthz", wrapHandler(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
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
	s.router.GET(path.Join(base, "guilds", ":guild", "emojis"), wrapHandler(s.getGuildEmojis))
	s.router.GET(path.Join(base, "guilds", ":guild", "emojis", ":emoji"), wrapHandler(s.getGuildEmoji))
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
