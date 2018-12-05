package main

import (
	"database/sql"
	"flag"
	"log"
	"net"
	"os"

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
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"

	"git.abal.moe/tatsu/state/internal/handlers"
	"git.abal.moe/tatsu/state/pb"
)

var (
	verbose  bool
	usePsql  bool
	useEs    bool
	usePprof bool
	port     string
)

func init() {
	flag.BoolVar(&verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&usePsql, "psql", false, "use postgres")
	flag.BoolVar(&useEs, "elastic", false, "use elasticsearch")
	flag.BoolVar(&usePprof, "pprof", false, "add pprof debugging")
	flag.StringVar(&port, "port", ":8080", ":8080")
	flag.Parse()
}

func main() {
	if port == "" {
		panic("please provide an address to listen on with -port :port")
	}

	if usePprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal(err)
	}

	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.WarnLevel
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.WarnLevel
	})

	toConsole := zapcore.Lock(os.Stderr)
	consoleEncoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())

	cores := []zapcore.Core{
		zapcore.NewCore(consoleEncoder, toConsole, highPriority),
	}

	if verbose {
		cores = append(cores, zapcore.NewCore(consoleEncoder, toConsole, lowPriority))
	}

	core := zapcore.NewTee(cores...)

	grpcLogger := zap.New(core)
	defer grpcLogger.Sync()

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	var (
		psql *sql.DB
		ec   *elastic.Client
	)

	if usePsql {
		psql, err = sql.Open("postgres", "host=localhost user=state dbname=state sslmode=disable")
		if err != nil {
			usePsql = false
			logger.Error("failed to connect to postgres, continuing without support", zap.Error(err))
		}
	} else {
		logger.Info("skipping postgres connectivity")
	}

	if useEs {
		ec, err = elastic.NewClient(elastic.SetURL("http://localhost:9200"), elastic.SetSniff(true))
		if err != nil {
			useEs = false
			logger.Error("failed to connect to elastic, continuing without support", zap.Error(err))
		}
	} else {
		logger.Info("skipping elastic connectivity")
	}

	ss, err := state.NewServer(logger, psql, ec, usePsql, useEs)
	if err != nil {
		panic(err)
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_prometheus.UnaryServerInterceptor,
				grpc_zap.UnaryServerInterceptor(grpcLogger),
				grpc_recovery.UnaryServerInterceptor(),
				state.RequiredFieldsInterceptor(),
			),
		),
	)
	pb.RegisterStateServer(srv, ss)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		panic(err)
	}

	logger.Info("listening at " + port)
	srv.Serve(lis)
}
