package manager

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/go-redis/redis"
	"github.com/tatsuworks/gateway/internal/gatewayws"
	"go.coder.com/slog"
	"golang.org/x/xerrors"
)

type Manager struct {
	ctx context.Context
	log slog.Logger
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
	logger slog.Logger,
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
		logger.Fatal(ctx, "failed to ping redis", slog.Error(err))
	}

	etcdc, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(etcdAddr, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		logger.Fatal(ctx, "failed to connect to etcd", slog.Error(err))
	}

	return &Manager{
		ctx: ctx,
		log: logger,
		wg:  wg,

		token:      token,
		shardCount: shards,

		shards: map[int]*gatewayws.Session{},

		rdb:  rc,
		etcd: etcdc,
	}
}

func (m *Manager) Start(start, stop int) error {
	for i := start; i < stop; i++ {
		m.log.Info(m.ctx, "starting shard", slog.F("shard", i), slog.F("total", m.shardCount))

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
		m.log.Error(m.ctx, "failed to make gateway session", slog.Error(err))
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

			m.log.Info(m.ctx, "attempting shard connect", slog.F("shard", shard))
			err := s.Open(m.ctx, m.token)
			if err != nil {
				if !xerrors.Is(err, context.Canceled) {
					m.log.Error(m.ctx, "websocket closed", slog.F("shard", shard), slog.Error(err))
				}
			}

			time.Sleep(time.Second)
		}
	}()
}
