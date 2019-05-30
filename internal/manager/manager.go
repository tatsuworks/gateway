package manager

import (
	"context"
	"github.com/tatsuworks/gateway/internal/gatewayws"
	"time"

	"go.uber.org/zap"
)

func New(ctx context.Context, logger *zap.Logger, token string, shards int, stateURL string) *Manager {
	return &Manager{
		ctx: ctx,
		log: logger,

		token:  token,
		shards: shards,

		stateURL: stateURL,
	}
}

type Manager struct {
	ctx context.Context
	log *zap.Logger

	token  string
	shards int

	stateURL string

	up int
}

func (m *Manager) Start(stopAt int) error {
	for i := 0; i < stopAt; i++ {
		select {
		case <-m.ctx.Done():
			return m.ctx.Err()
		default:
		}

		m.log.Info("starting shard", zap.Int("shard", i), zap.Int("total", m.shards))
		go m.startShard(i)
		time.Sleep(5100 * time.Millisecond)
	}

	select {
	case <-m.ctx.Done():
	}
	return nil
}

func (m *Manager) startShard(shard int) {
	s, err := gatewayws.NewSession(m.log, m.token, shard, m.shards, m.stateURL)
	if err != nil {
		m.log.Error("failed to make gateway session", zap.Error(err))
		return
	}

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		m.log.Info("attempting shard connect", zap.Int("shard", shard))
		err := s.Open(m.ctx, m.token)
		if err != nil {
			m.log.Error("websocket closed", zap.Int("shard", shard), zap.Error(err))
		}

		time.Sleep(time.Second)
	}

}
