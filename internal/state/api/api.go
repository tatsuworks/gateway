package api

import (
	"net/http"
	"path"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/valyala/fasthttp/reuseport"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/internal/state"
)

var (
	FDBRangeWantAll = fdb.RangeOptions{Mode: fdb.StreamingModeWantAll}
)

type Server struct {
	log     *zap.Logger
	version string

	router *httprouter.Router

	db *state.DB
}

func NewServer(
	logger *zap.Logger,
	version string,
) (*Server, error) {
	db, err := state.NewDB()
	if err != nil {
		return nil, xerrors.Errorf("failed to create state db: %w", err)
	}

	return &Server{
		log:     logger,
		router:  httprouter.New(),
		version: version,
		db:      db,
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

	ln, err := reuseport.Listen("tcp4", addr)
	if err != nil {
		return err
	}

	srv.Handler = s.router
	return srv.ServeTLS(ln, "localhost.cert", "localhost.key")
}
