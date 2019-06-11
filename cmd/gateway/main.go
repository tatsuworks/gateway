package main

import (
	"context"
	"fmt"
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

	m := manager.New(ctx, logger, Token, 1, "localhost:6380")

	err = m.Start(1)
	if err != nil {
		fmt.Println(err)
	}

	time.Sleep(5 * time.Second)
}
