package manager

import (
	"context"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"

	"github.com/tatsuworks/gateway/internal/gatewayws"
)

type Manager struct {
	ctx context.Context
	log *zap.Logger

	token  string
	shards int

	up int
}

func New(
	ctx context.Context,
	logger *zap.Logger,
	token string,
	shards int,
	redisAddr string,
) *Manager {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err := client.Ping().Result()
	if err != nil {
		logger.Fatal("failed to ping redis", zap.Error(err))
	}

	return &Manager{
		ctx: ctx,
		log: logger,

		token:  token,
		shards: shards,
	}
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
	s, err := gatewayws.NewSession(m.log, m.token, shard, m.shards)
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
