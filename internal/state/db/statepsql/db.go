package statepsql

import (
	"context"
	"database/sql/driver"
	"os"
	"strconv"

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
	memberEventCh   chan<- MemberEvent
	presenceEventCh chan<- PresenceEvent
	guildEventCh    chan<- GuildEvent
	logger        slog.Logger
}

func NewDB(ctx context.Context, addr string, logger slog.Logger) (state.DB, error) {
	sqlx, err := sqlx.Open("postgres", addr)
	if err != nil {
		return nil, xerrors.Errorf("open sqlx: %w", err)
	}

	maxConns := 4
	if v, err := strconv.Atoi(os.Getenv("PSQL_MAX_CONNS")); err == nil && v > 0 {
		maxConns = v
	}
	sqlx.SetMaxOpenConns(maxConns)
	sqlx.SetMaxIdleConns(maxConns)

	err = sqlx.Ping()
	if err != nil {
		return nil, xerrors.Errorf("ping postgres: %w", err)
	}
	dbInstance := &db{sql: sqlx, logger: logger}
	dbInstance.memberEventCh = BatchWorker(ctx, 1000, maxConns, 100*time.Millisecond, dbInstance.processMemberBatch, logger)
	dbInstance.presenceEventCh = BatchWorker(ctx, 1000, maxConns, 100*time.Millisecond, dbInstance.processPresenceBatch, logger)
	dbInstance.guildEventCh = BatchWorker(ctx, 500, maxConns, 100*time.Millisecond, dbInstance.processGuildBatch, logger)
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
