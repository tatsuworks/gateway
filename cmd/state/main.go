package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"

	"github.com/google/gops/agent"
	_ "github.com/lib/pq"
	"github.com/tatsuworks/state/internal/api"
	"go.uber.org/zap"
)

var (
	verbose   bool
	usePsql   bool
	useEs     bool
	usePprof  bool
	port      string
	redisAddr string

	Version string
)

func init() {
	flag.BoolVar(&verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&usePsql, "psql", false, "use postgres")
	flag.BoolVar(&useEs, "elastic", false, "use elasticsearch")
	flag.BoolVar(&usePprof, "pprof", false, "add pprof debugging")
	flag.StringVar(&port, "port", ":80", ":80")
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
		logger.Fatal("failed to create gops agent", zap.Error(err))
	}

	state, err := api.NewServer(logger, Version)
	if err != nil {
		logger.Panic("failed to create etfstate", zap.Error(err))
	}

	state.Init()
	logger.Fatal("failed to run server", zap.Error(state.Start(":8080")))
}
