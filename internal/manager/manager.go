package manager

import (
	"context"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/go-redis/redis"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/internal/gatewayws"
)

type Manager struct {
	ctx context.Context
	log *zap.Logger
	wg  *sync.WaitGroup

	token      string
	shardCount int

	shardMu sync.Mutex
	shards  map[int]*gatewayws.Session

	rdb  *redis.Client
	etcd *clientv3.Client
}

func New(
	ctx context.Context,
	logger *zap.Logger,
	wg *sync.WaitGroup,
	token string,
	shards int,
	redisAddr string,
	etcdAddr string,
) *Manager {
	rc := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	_, err := rc.Ping().Result()
	if err != nil {
		logger.Fatal("failed to ping redis", zap.Error(err))
	}

	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://10.0.0.3:2379", "http://10.0.0.3:4001"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		logger.Fatal("failed to connect to etcd", zap.Error(err))
	}

	return &Manager{
		ctx: ctx,
		log: logger,
		wg:  wg,

		token:      token,
		shardCount: shards,

		shards: map[int]*gatewayws.Session{},

		rdb:  rc,
		etcd: etcdCli,
	}
}

func (m *Manager) Start(start, stop int) error {
	for i := start; i < stop; i++ {
		m.log.Info("starting shard", zap.Int("shard", i), zap.Int("total", m.shardCount))

		select {
		case <-m.ctx.Done():
			return nil
		default:
			m.startShard(i)
		}
	}

	return nil
}

func (m *Manager) startShard(shard int) {
	s, err := gatewayws.NewSession(m.log, m.wg, m.rdb, m.etcd, m.token, shard, m.shardCount)
	if err != nil {
		m.log.Error("failed to make gateway session", zap.Error(err))
		return
	}

	m.shardMu.Lock()
	m.shards[shard] = s
	m.shardMu.Unlock()

	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			default:
			}

			m.log.Info("attempting shard connect", zap.Int("shard", shard))
			err := s.Open(m.ctx, m.token)
			if err != nil {
				if !xerrors.Is(err, context.Canceled) {
					m.log.Error("websocket closed", zap.Int("shard", shard), zap.Error(err))
				}
			}

			time.Sleep(time.Second)
		}
	}()
}
