package statepsql

import (
	"context"
	"database/sql/driver"

	"cdr.dev/slog"

	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/discord/discordjson"
	"github.com/tatsuworks/gateway/internal/state"
	"golang.org/x/xerrors"
)

var _ state.DB = &db{}

type db struct {
	sql           *sqlx.DB
	memberEventCh chan<- MemberEvent
	logger        slog.Logger
}

func NewDB(ctx context.Context, addr string, logger slog.Logger) (state.DB, error) {
	sqlx, err := sqlx.Open("postgres", addr)
	if err != nil {
		return nil, xerrors.Errorf("open sqlx: %w", err)
	}

	sqlx.SetMaxOpenConns(4)
	sqlx.SetMaxIdleConns(4)

	err = sqlx.Ping()
	if err != nil {
		return nil, xerrors.Errorf("ping postgres: %w", err)
	}
	dbInstance := &db{sql: sqlx, logger: logger}
	// Now start the worker and assign the channel
	dbInstance.memberEventCh = dbInstance.StartMemberWorker(ctx, 100, 100*time.Millisecond)

	return dbInstance, nil
}

func (db *db) Encoding() discord.Encoding {
	return discordjson.Encoding
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
