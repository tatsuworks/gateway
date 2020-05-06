package handler

import (
	"sync/atomic"

	"cdr.dev/slog"
	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/internal/state"
)

type Client struct {
	log slog.Logger
	db  state.DB
	enc discord.Encoding

	waitingQueries int64
}

func (c *Client) WaitingQueries() int64 {
	return atomic.LoadInt64(&c.waitingQueries)
}

func NewClient(log slog.Logger, db state.DB) *Client {
	return &Client{
		log: log,
		db:  db,
		enc: db.Encoding(),
	}
}
