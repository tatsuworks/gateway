package main

import (
	"context"

	"github.com/tatsuworks/gateway/protos/gatewaypb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger, _ := zap.NewDevelopment()

	conn, err := grpc.Dial("127.0.0.1:8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Fatal("connect", zap.Error(err))
	}

	cli := gatewaypb.NewGatewayClient(conn)

	_, err = cli.RestartShard(context.Background(), &gatewaypb.RestartShardRequest{Shard: 1})
	if err != nil {
		logger.Fatal("send request", zap.Error(err))
	}
}
