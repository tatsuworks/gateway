package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/tatsuworks/gateway/internal/manager"
)

var (
	redisHost           string
	shards, start, stop int
)

func init() {
	flag.StringVar(&redisHost, "redis", "localhost:6380", "localhost:6379")
	flag.IntVar(&shards, "shards", 1, "1")

	// both do not work
	flag.IntVar(&start, "start", 0, "1")
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

	// change 5 to the total number of shards you want
	m := manager.New(ctx, logger, Token, shards, redisHost)

	err = m.Start(shards)
	if err != nil {
		logger.Fatal(err.Error())
	}

	time.Sleep(5 * time.Second)
	<-ctx.Done()
}
