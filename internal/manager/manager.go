package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coreos/etcd/clientv3"
	"github.com/go-redis/redis"
	"github.com/tatsuworks/gateway/internal/gatewayws"
	"github.com/tatsuworks/gateway/internal/state"
)

type Manager struct {
	ctx  context.Context
	name string
	log  slog.Logger
	wg   *sync.WaitGroup
	db   state.DB

	token      string
	intents    gatewayws.Intents
	shardCount int

	shardMu sync.Mutex
	shards  map[int]*gatewayws.Session

	rdb        *redis.Client
	rdbClients []*redis.Client
	etcd       *clientv3.Client

	bufferPool        *sync.Pool
	whitelistedEvents map[string]struct{}
}

type Config struct {
	Name              string
	Logger            slog.Logger
	DB                state.DB
	Wg                *sync.WaitGroup
	Token             string
	Shards            int
	Intents           gatewayws.Intents
	RedisAddr         string
	EtcdAddr          string
	PodID             string
	WhitelistedEvents map[string]struct{}
}

func New(ctx context.Context, cfg *Config) *Manager {
	multiRedisEnv := os.Getenv("multi_redis")
	var multiRedisAddresses []string
	var rc *redis.Client
	var rdbClients []*redis.Client
	var err error

	if multiRedisEnv != "" {
		err = json.Unmarshal([]byte(multiRedisEnv), &multiRedisAddresses)
		if err != nil {
			cfg.Logger.Fatal(ctx, "invalid multi_redis", slog.Error(err))
		}
		for _, addr := range multiRedisAddresses {
			var mrc *redis.Client
			mrc, err = createRedisClient(addr, cfg.Name, cfg.PodID)
			if err != nil {
				// It is not fatal if one multiRedis client did not connect.
				cfg.Logger.Warn(ctx, "createRedisClient", slog.Error(err))
				continue
			}
			rdbClients = append(rdbClients, mrc)
		}

		// No multi redis clients were connected, or all failed to connect.
		if len(rdbClients) == 0 {
			cfg.Logger.Fatal(ctx, "multiRedisEnv is set, but all redis clients failed to connect.")
		}
	} else {
		rc, err = createRedisClient(cfg.RedisAddr, cfg.Name, cfg.PodID)
		if err != nil {
			cfg.Logger.Fatal(ctx, "createRedisClient", slog.Error(err))
		}
	}

	etcdc, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(cfg.EtcdAddr, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		cfg.Logger.Fatal(ctx, "failed to connect to etcd", slog.Error(err))
	}

	return &Manager{
		ctx:  ctx,
		name: cfg.Name,
		log:  cfg.Logger,
		wg:   cfg.Wg,
		db:   cfg.DB,

		token:      cfg.Token,
		intents:    cfg.Intents,
		shardCount: cfg.Shards,

		shards: map[int]*gatewayws.Session{},

		rdb:        rc,
		rdbClients: rdbClients,
		etcd:       etcdc,

		bufferPool: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},

		whitelistedEvents: cfg.WhitelistedEvents,
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

	go m.logHealth()
	return nil
}

func (m *Manager) startShard(shard int) {
	s, err := gatewayws.NewSession(&gatewayws.SessionConfig{
		Name:              m.name,
		Logger:            m.log,
		DB:                m.db,
		WorkGroup:         m.wg,
		Redis:             m.rdb,
		MultiRedis:        m.rdbClients,
		Etcd:              m.etcd,
		Token:             m.token,
		Intents:           m.intents,
		ShardID:           shard,
		ShardCount:        m.shardCount,
		BufferPool:        m.bufferPool,
		WhitelistedEvents: m.whitelistedEvents,
	})
	if err != nil {
		m.log.Error(m.ctx, "make gateway session", slog.Error(err))
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
				// if !xerrors.Is(err, context.Canceled) {
				m.log.Error(m.ctx, "websocket closed", slog.F("shard", shard), slog.Error(err))
				// }
			}

			time.Sleep(time.Second)
		}
	}()
}

const ManagerLogInterval = 5 * time.Minute

func (m *Manager) logHealth() {
	var (
		t   = time.NewTicker(ManagerLogInterval)
		ctx = m.ctx
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		var out []string
		for _, session := range m.shards {
			if session != nil && session.LongLastAck(ManagerLogInterval) {
				out = append(out, session.Status())
			}
		}

		if len(out) > 0 {
			m.log.Info(
				m.ctx,
				"shard report",
				slog.F("event", out),
			)
		}
	}
}

func createRedisClient(addr, name, podID string) (*redis.Client, error) {
	rc := redis.NewClient(&redis.Options{
		Addr: addr,
		OnConnect: func(c *redis.Conn) error {
			if podID != "" {
				c.ClientSetName(name + "-" + podID)
			} else {
				c.ClientSetName(name)
			}

			return nil
		},
	})

	_, err := rc.Ping().Result()
	if err != nil {
		return nil, err
	}

	return rc, nil
}
