package statepsql

import (
	"database/sql"

	"github.com/tatsuworks/gateway/internal/state"
)

type db struct {
	sql *sql.DB
}

func NewDB(psql *sql.DB) (state.DB, error) {
	return &db{}, nil
}
