package main

import (
	"net"
	"os"

	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"git.friday.cafe/fndevs/state/internal/handlers"
	"git.friday.cafe/fndevs/state/pb"
)

func main() {
	if len(os.Args) < 2 {
		panic("please provide an address to listen on")
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	ss, err := state.NewServer(logger)
	if err != nil {
		panic(err)
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_prometheus.UnaryServerInterceptor,
				grpc_zap.UnaryServerInterceptor(logger),
				grpc_recovery.UnaryServerInterceptor(),
				state.RequiredFieldsInterceptor(),
			),
		),
	)
	pb.RegisterStateServer(srv, ss)

	lis, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		panic(err)
	}

	logger.Info("listening at " + os.Args[1])
	srv.Serve(lis)
}
