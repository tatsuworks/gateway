package statepsql

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/tatsuworks/gateway/internal/state"
)

type db struct {
	sql *sqlx.DB
}

func NewDB(psql *sql.DB) (state.DB, error) {
	sqlx := sqlx.NewDb(psql, "postgres")
	return &db{sqlx}, nil
}
