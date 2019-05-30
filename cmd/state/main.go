package main

import (
	"database/sql"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/go-redis/redis"
	"github.com/google/gops/agent"
	_ "github.com/lib/pq"
	"github.com/olivere/elastic"
	"github.com/tatsuworks/state/internal/api"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	verbose   bool
	usePsql   bool
	useEs     bool
	usePprof  bool
	port      string
	redisAddr string

	Version string
)

func init() {
	flag.BoolVar(&verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&usePsql, "psql", false, "use postgres")
	flag.BoolVar(&useEs, "elastic", false, "use elasticsearch")
	flag.BoolVar(&usePprof, "pprof", false, "add pprof debugging")
	flag.StringVar(&port, "port", ":80", ":80")
	flag.StringVar(&redisAddr, "redis", "localhost:6379", "localhost:6379")
	flag.Parse()
}

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}

	if usePprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if err := agent.Listen(agent.Options{}); err != nil {
		logger.Fatal("failed to create gops agent", zap.Error(err))
	}

	state, err := api.NewServer(logger, Version)
	if err != nil {
		logger.Panic("failed to create etfstate", zap.Error(err))
	}

	state.Init()
	logger.Fatal("failed to run server", zap.Error(state.Start(":8080")))

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

	var (
		psql *sql.DB
		ec   *elastic.Client
		_, _ = psql, ec
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

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	err = rdb.Ping().Err()
	if err != nil {
		logger.Panic("failed to ping redis", zap.Error(err))
	}
}
