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
	return &Server{}
}
