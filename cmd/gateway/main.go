package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/tatsuworks/gateway/internal/manager"
)

var (
	redisHost           string
	pprof               string
	shards, start, stop int
)

func init() {
	flag.StringVar(&redisHost, "redis", "localhost:6380", "localhost:6379")
	flag.StringVar(&pprof, "pprof", "localhost:6060", "localhost:6060")
	flag.IntVar(&shards, "shards", 1, "1")

	flag.IntVar(&start, "start", 0, "0")
	flag.IntVar(&stop, "stop", 1, "1")

	flag.Parse()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		logger.Info("closing")
		cancel()
	}()

	wg := &sync.WaitGroup{}
	m := manager.New(ctx, logger, wg, Token, shards, redisHost)

	err = m.Start(start, stop)
	if err != nil {
		logger.Fatal(err.Error())
	}

	<-ctx.Done()
	logger.Info("waiting for shards to disconnect")
	time.Sleep(time.Second)
}
