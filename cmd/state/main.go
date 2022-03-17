package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogjson"
	"github.com/google/gops/agent"
	"github.com/tatsuworks/gateway/internal/state"
	"github.com/tatsuworks/gateway/internal/state/api"
	"github.com/tatsuworks/gateway/internal/state/db/statefdb"
	"github.com/tatsuworks/gateway/internal/state/db/statepsql"
)

var (
	prod     string
	usePprof bool
	addr     string
	usePsql  bool
	psqlAddr string

	Version string
)

func init() {
	flag.StringVar(&prod, "prod", "", "Enable production logging")
	flag.BoolVar(&usePprof, "pprof", false, "add pprof debugging")
	flag.StringVar(&addr, "addr", "0.0.0.0:8080", "0.0.0.0:80")
	flag.StringVar(&psqlAddr, "psql", "", "Postgres address")
	flag.Parse()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logger slog.Logger
	var err error
	if prod != "" {
		logger = slogjson.Make(os.Stderr)
	} else {
		logger = sloghuman.Make(os.Stderr)
	}

	defer logger.Sync()

	var statedb state.DB
	if psqlAddr != "" {
		statedb, err = statepsql.NewDB(ctx, psqlAddr)
		if err != nil {
			logger.Fatal(ctx, "failed to init Postgres state", slog.Error(err))
		}
	} else {
		statedb, err = statefdb.NewDB()
		if err != nil {
			logger.Fatal(ctx, "failed to init fdb state", slog.Error(err))
		}
	}

	if usePprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if err := agent.Listen(agent.Options{}); err != nil {
		logger.Fatal(ctx, "failed to create gops agent", slog.Error(err))
	}

	state, err := api.NewServer(logger, statedb, Version)
	if err != nil {
		logger.Fatal(ctx, "failed to create state", slog.Error(err))
	}

	state.Init()
	logger.Fatal(ctx, "failed to run server", slog.Error(state.Start(addr)))
}
