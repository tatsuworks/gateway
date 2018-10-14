package main

import (
	"net"
	"os"

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

	state, err := state.NewServer(logger)
	if err != nil {
		panic(err)
	}

	srv := grpc.NewServer()
	pb.RegisterStateServer(srv, state)

	lis, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		panic(err)
	}

	logger.Info("listening...")
	srv.Serve(lis)
}
