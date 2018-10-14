package state

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"go.uber.org/zap"
)

// Server ...
type Server struct {
	log *zap.Logger

	DB fdb.Database
}
