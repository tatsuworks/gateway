package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/tatsuworks/gateway/gatewaypb"
	"github.com/tatsuworks/gateway/internal/manager"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	redisHost  string
	etcdHost   string
	playedHost string
	pprof      string
	prod       bool
	addr       string

	shards, start, stop int
)

func init() {
	flag.StringVar(&redisHost, "redis", "localhost:6379", "localhost:6379")
	flag.StringVar(&etcdHost, "etcd", "http://localhost:2379,http://localhost:4001", "")
	flag.StringVar(&playedHost, "played", "ws://localhost:8089", "Played")
	flag.StringVar(&pprof, "pprof", "localhost:6060", "Address for pprof to listen on")
	flag.BoolVar(&prod, "prod", false, "Enable production logging")
	flag.StringVar(&addr, "addr", "localhost:80", "Management address to listen on")

	flag.IntVar(&shards, "shards", 1, "Total shards")
	flag.IntVar(&start, "start", 0, "First shard to start (inclusive)")
	flag.IntVar(&stop, "stop", 1, "Last shard (non-inclusive)")

	flag.Parse()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slogjson.Make(os.Stderr)
	defer logger.Sync()

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

	wg := &sync.WaitGroup{}
	m := manager.New(ctx, logger, wg, Token, shards, redisHost, etcdHost, playedHost)

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
