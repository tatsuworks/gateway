package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"go.uber.org/zap"
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
	flag.StringVar(&redisHost, "redis", "localhost:6380", "localhost:6379")
	flag.StringVar(&etcdHost, "etcd", "http://10.0.0.3:2379,http://10.0.0.3:4001", "http://10.0.0.3:2379,http://10.0.0.3:4001")
	flag.StringVar(&pprof, "pprof", "localhost:6060", "localhost:6060")
	flag.IntVar(&shards, "shards", 1, "1")
	flag.BoolVar(&prod, "prod", false, "enable production logging")
	flag.StringVar(&addr, "addr", "127.0.0.1:8000", "address to listen on")

	flag.IntVar(&start, "start", 0, "0")
	flag.IntVar(&stop, "stop", 1, "1")

	flag.Parse()
}

func logger() *zap.Logger {
	var (
		logger *zap.Logger
		err    error
	)

	if prod {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}

	if err != nil {
		panic(err)
	}

	return logger
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := logger()

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		logger.Info("closing")
		cancel()
	}()

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}

	wg := &sync.WaitGroup{}
	m := manager.New(ctx, logger, wg, Token, shards, redisHost, etcdHost)

	err = m.Start(start, stop)
	if err != nil {
		logger.Fatal(err.Error())
	}

	go func() {
		srv := grpc.NewServer()
		gatewaypb.RegisterGatewayServer(srv, m)
		reflection.Register(srv)
		srv.Serve(lis)
	}()

	<-ctx.Done()
	logger.Info("waiting for shards to disconnect")
	wg.Wait()
}
