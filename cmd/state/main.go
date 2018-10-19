package main

import (
	"database/sql"
	"log"
	"net"
	"os"
	"runtime/debug"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/google/gops/agent"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	_ "github.com/lib/pq"
	"github.com/olivere/elastic"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"git.friday.cafe/fndevs/state/internal/handlers"
	"git.friday.cafe/fndevs/state/pb"
)

func main() {
	if len(os.Args) < 2 {
		panic("please provide an address to listen on")
	}

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal(err)
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			time.Sleep(10 * time.Second)
			logger.Info("freeing memory")
			debug.FreeOSMemory()
		}
	}()

	psql, err := sql.Open("postgres", "host=localhost user=state dbname=state sslmode=disable")
	if err != nil {
		logger.Fatal("failed to connect to postgres", zap.Error(err))
	}

	elastic, err := elastic.NewClient(elastic.SetURL("http://localhost:9200"), elastic.SetSniff(true))
	if err != nil {
		logger.Warn("continuing without elastic support...")
	}

	ss, err := state.NewServer(logger, psql, elastic)
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
