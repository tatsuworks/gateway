package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/namsral/flag"
	"go.coder.com/slog"
	"go.coder.com/slog/sloggers/sloghuman"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/tatsuworks/gateway/gatewaypb"
	"github.com/tatsuworks/gateway/internal/manager"
)

var (
	redisHost string
	etcdHost  string
	pprof     string
	prod      bool
	addr      string

	shards, start, stop int
)

func init() {
	flag.StringVar(&redisHost, "redis", "10.64.132.51:6379", "localhost:6379")
	flag.StringVar(&etcdHost, "etcd", "http://10.64.132.51:2379,http://10.64.132.51:4001", "http://10.0.0.3:2379,http://10.0.0.3:4001")
	flag.StringVar(&pprof, "pprof", "localhost:6060", "localhost:6060")
	flag.IntVar(&shards, "shards", 1, "1")
	flag.BoolVar(&prod, "prod", false, "enable production logging")
	flag.StringVar(&addr, "addr", "0.0.0.0:80", "address to listen on")

	flag.IntVar(&start, "start", 0, "0")
	flag.IntVar(&stop, "stop", 1, "1")

	flag.Parse()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := sloghuman.Make(os.Stderr)
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
		logger.Fatal(ctx, "failed to listen", slog.Error(err))
	}

	wg := &sync.WaitGroup{}
	m := manager.New(ctx, logger, wg, Token, shards, redisHost, etcdHost)

	err = m.Start(start, stop)
	if err != nil {
		logger.Fatal(ctx, "failed to start shard manager", slog.Error(err))
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
