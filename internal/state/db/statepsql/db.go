package statepsql

import (
	"database/sql"
	"database/sql/driver"

	"github.com/jmoiron/sqlx"
	"github.com/tatsuworks/gateway/internal/state"
	"golang.org/x/xerrors"
)

type db struct {
	sql *sqlx.DB
}

func NewDB(psql *sql.DB) (state.DB, error) {
	sqlx := sqlx.NewDb(psql, "postgres")
	return &db{sqlx}, nil
}

type DataRow struct {
	Data RawJSON `db:"data"`
}

type RawJSON []byte

func (r RawJSON) Value() (driver.Value, error) {
	return []byte(r), nil
}

func (r *RawJSON) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return xerrors.Errorf("unexpected value type. wanted []byte got %T", value)
	}

	// Overwrite contents of r with a copy of b.
	*r = append((*r)[0:0], b...)
	return nil
}
