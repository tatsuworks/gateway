package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/tatsuworks/gateway/internal/manager"
)

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
	m := manager.New(ctx, logger, Token, 1, "localhost:6380")

	// change 5 to the number of shards you want to start up
	// for example, your bot may require 400 shards but you only want
	// to start up 5
	err = m.Start(1)
	if err != nil {
		logger.Fatal(err.Error())
	}

	time.Sleep(5 * time.Second)
}
