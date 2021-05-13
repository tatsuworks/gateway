package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogjson"
	"cdr.dev/slog/sloggers/slogstackdriver"
	"cloud.google.com/go/profiler"
	"github.com/tatsuworks/gateway/gatewaypb"
	"github.com/tatsuworks/gateway/internal/gatewayws"
	"github.com/tatsuworks/gateway/internal/manager"
	"github.com/tatsuworks/gateway/internal/state"
	"github.com/tatsuworks/gateway/internal/state/db/statefdb"
	"github.com/tatsuworks/gateway/internal/state/db/statepsql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	name       string
	token      string
	redisHost  string
	etcdHost   string
	playedHost string
	pprof      string
	prod       string
	psql       bool
	psqlAddr   string
	addr       string
	intents    string
	podId      string

	shards, start, stop int
)

func init() {
	flag.StringVar(&name, "name", "gateway", "name of gateway")
	flag.StringVar(&token, "token", "", "token for the bot")
	flag.StringVar(&redisHost, "redis", "localhost:6379", "localhost:6379")
	flag.StringVar(&etcdHost, "etcd", "http://localhost:2379,http://localhost:4001", "")
	flag.StringVar(&playedHost, "played", "", "Played")
	flag.StringVar(&pprof, "pprof", "localhost:6060", "Address for pprof to listen on")
	flag.StringVar(&prod, "prod", "", "Enable production logging")
	flag.StringVar(&psqlAddr, "psqlAddr", "", "Address to connect to Postgres on")
	flag.StringVar(&addr, "addr", "localhost:80", "Management address to listen on")
	flag.StringVar(&intents, "intents", "default", "default, played, all")
	flag.StringVar(&podId, "podId", "", "0, 1, 2, 3...")

	flag.IntVar(&shards, "shards", 1, "Total shards")
	flag.IntVar(&start, "start", 0, "First shard to start (inclusive)")
	flag.IntVar(&stop, "stop", 1, "Last shard (non-inclusive)")

	flag.Parse()
	psql = psqlAddr != ""

	// // load .env file
	// err := godotenv.Load("../../.env")

	// if err != nil {
	// 	log.Fatalf("Error loading .env file")
	// }

}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var logger slog.Logger

	var (
		statedb state.DB
		err     error
	)

	cfg := profiler.Config{
		Service:        "gateway",
		ServiceVersion: "1.0.0",
	}

	// Profiler initialization, best done as early as possible.
	err = profiler.Start(cfg)
	if err != nil {
		if prod != "" {
			logger = slogjson.Make(os.Stderr)
		} else {
			logger = sloghuman.Make(os.Stderr)
		}
		logger.Error(ctx, "profiler could not start", slog.F("err", err))
	} else {
		// running on gcp, so use slogstackdriver instead
		logger = slogstackdriver.Make(os.Stderr)
	}
	defer logger.Sync()

	if psql {
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

	var ints gatewayws.Intents
	switch intents {
	case "default":
		ints = gatewayws.DefaultIntents
	case "played":
		ints = gatewayws.PresencesOnly
	case "all":
		ints = gatewayws.AllIntents
	default:
		logger.Fatal(ctx, "unknown intents", slog.F("intent", intents))
	}

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		logger.Info(ctx, "closing")
		cancel()
	}()

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal(ctx, "listen", slog.Error(err))
	}

	events := os.Getenv("WHITELIST_EVENTS")
	fmt.Println("events", events)
	var whitelistedEventLookup map[string]struct{}
	if events != "" {
		whitelistedEvents := strings.Split(events, ",")
		whitelistedEventLookup = make(map[string]struct{}, len(whitelistedEvents))
		var empty struct{}
		for _, evt := range whitelistedEvents {
			whitelistedEventLookup[evt] = empty
		}
	}

	wg := &sync.WaitGroup{}
	m := manager.New(ctx, &manager.Config{
		Name:              name,
		Logger:            logger,
		Wg:                wg,
		DB:                statedb,
		Token:             token,
		Shards:            shards,
		Intents:           ints,
		RedisAddr:         redisHost,
		EtcdAddr:          etcdHost,
		PlayedAddr:        playedHost,
		PodID:             podId,
		WhitelistedEvents: whitelistedEventLookup,
	})

	logger.Info(ctx, "starting manager",
		slog.F("shards", shards),
		slog.F("start", start),
		slog.F("stop", stop),
		slog.F("redis_host", redisHost),
		slog.F("etcd_host", etcdHost),
	)

	err = m.Start(start, stop)
	if err != nil {
		logger.Fatal(ctx, "start shard manager", slog.Error(err))
	}

	go func() {
		srv := grpc.NewServer()
		gatewaypb.RegisterGatewayServer(srv, m)
		reflection.Register(srv)
		srv.Serve(lis)
	}()

	<-ctx.Done()
	logger.Info(ctx, "waiting for shards to disconnect")
	wg.Wait()
}
