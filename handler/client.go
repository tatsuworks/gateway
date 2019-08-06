package handler

import (
	"github.com/tatsuworks/gateway/internal/state"
	"golang.org/x/xerrors"
)

type Client struct {
	db *state.DB
}

func NewClient() (*Client, error) {
	db, err := state.NewDB()
	if err != nil {
		return nil, xerrors.Errorf("failed to create state db: %w", err)
	}

	return &Client{
		db: db,
	}, nil
}
