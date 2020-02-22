package handler

import (
	"context"

	"cdr.dev/slog"
	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/internal/state"
)

var defaultCtx = context.Background()

type Client struct {
	log slog.Logger
	db  state.DB
	enc discord.Encoding
}

func NewClient(log slog.Logger, db state.DB) *Client {
	return &Client{
		log: log,
		db:  db,
		enc: db.Encoding(),
	}
}
