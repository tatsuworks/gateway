package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/google/gops/agent"
	"github.com/tatsuworks/gateway/internal/state/api"
	"go.uber.org/zap"
)

var (
	verbose   bool
	usePsql   bool
	useEs     bool
	usePprof  bool
	addr      string
	redisAddr string

	Version string
)

func init() {
	flag.BoolVar(&verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&usePsql, "psql", false, "use postgres")
	flag.BoolVar(&useEs, "elastic", false, "use elasticsearch")
	flag.BoolVar(&usePprof, "pprof", false, "add pprof debugging")
	flag.StringVar(&addr, "addr", "0.0.0.0:8080", "0.0.0.0:80")
	flag.StringVar(&redisAddr, "redis", "localhost:6379", "localhost:6379")
	flag.Parse()
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	if usePprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if err := agent.Listen(agent.Options{}); err != nil {
		logger.Fatal("create gops agent", zap.Error(err))
	}

	state, err := api.NewServer(logger, Version)
	if err != nil {
		logger.Panic("create state", zap.Error(err))
	}

	state.Init()
	logger.Fatal("run server", zap.Error(state.Start(addr)))
}
