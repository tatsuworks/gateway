package state

import (
	"git.friday.cafe/fndevs/state/pb"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"go.uber.org/zap"
)

var _ pb.StateServer = &Server{}

// Server ...
type Server struct {
	log *zap.Logger

	DB fdb.Database
}

// NewServer creates a new state Server.
func NewServer() *Server {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}

	fdb.MustAPIVersion(510)
	db := fdb.MustOpenDefault()

	return &Server{
		log: logger,
		DB:  db,
	}
}
