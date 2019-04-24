package manager

import (
	"context"
	"github.com/fngdevs/gateway/internal/gatewayws"
	"time"

	"go.uber.org/zap"
)

func New(ctx context.Context, logger *zap.Logger, token string, shards int) *Manager {
	return &Manager{
		ctx: ctx,
		log: logger,

		token:  token,
		shards: shards,
	}
}

type Manager struct {
	ctx context.Context
	log *zap.Logger

	token  string
	shards int

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
	s := gatewayws.NewSession(m.log, m.token, shard, m.shards)

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
	}

}
