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
	"github.com/redis/go-redis/v9"
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
	// wake holds one depth-1 channel per shard, used to interrupt that shard's
	// reconnect backoff on an explicit management request. Guarded by shardMu.
	wake map[int]chan struct{}

	rdb  []*redis.Client
	etcd *clientv3.Client

	bufferPool *sync.Pool

	whitelistedEvents map[string]map[string]struct{}
}

type Config struct {
	Name     string
	Logger   slog.Logger
	DB       state.DB
	Wg       *sync.WaitGroup
	Token    string
	Shards   int
	Intents  gatewayws.Intents
	EtcdAddr string
	PodID    string
}

func New(ctx context.Context, cfg *Config) *Manager {
	multiRedisEnv := os.Getenv("MULTI_REDIS")
	if multiRedisEnv == "" {
		cfg.Logger.Fatal(ctx, "MULTI_REDIS environment variable is required")
	}

	var rdbClients []*redis.Client
	var redisWhitelistedEvents = make(map[string]map[string]struct{})
	var err error

	var multiRedisConfig map[string][]string
	err = json.Unmarshal([]byte(multiRedisEnv), &multiRedisConfig)
	if err != nil {
		cfg.Logger.Fatal(ctx, "invalid MULTI_REDIS format", slog.Error(err))
	}

	for address, events := range multiRedisConfig {
		var mrc *redis.Client
		mrc, err = createRedisClient(ctx, address, cfg.Name, cfg.PodID)
		if err != nil {
			if os.Getenv("PROD") != "" {
				cfg.Logger.Fatal(ctx, "createRedisClient",
					slog.F("address", address),
					slog.Error(err))
			}

			// Not all multiRedis clients need to connect in development.
			cfg.Logger.Warn(ctx, "createRedisClient",
				slog.F("address", address),
				slog.Error(err))
			continue
		}
		rdbClients = append(rdbClients, mrc)

		if len(events) > 0 {
			whitelistForClient := make(map[string]struct{})
			for _, event := range events {
				whitelistForClient[event] = struct{}{}
			}
			redisWhitelistedEvents[mrc.Options().Addr] = whitelistForClient
		}
	}

	// No multi redis clients were connected, or all failed to connect.
	if len(rdbClients) == 0 {
		cfg.Logger.Fatal(ctx, "all redis clients failed to connect")
	}

	// This collects the addresses of all CONNECTED redis clients.
	addresses := make([]string, 0, len(rdbClients))
	for _, c := range rdbClients {
		addresses = append(addresses, c.Options().Addr)
	}
	cfg.Logger.Info(ctx, "initialized redis clients", slog.F("count", len(rdbClients)), slog.F("addrs", strings.Join(addresses, ",")))

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
		wake:   map[int]chan struct{}{},

		rdb:  rdbClients,
		etcd: etcdc,

		bufferPool: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},

		whitelistedEvents: redisWhitelistedEvents,
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

	wake := make(chan struct{}, 1)

	m.shardMu.Lock()
	m.shards[shard] = s
	m.wake[shard] = wake
	m.shardMu.Unlock()

	go func() {
		// Consecutive failed connect attempts, driving the reconnect backoff.
		// Reset by any connection that stays up for reconnectHealthyUptime.
		var failures int

		for {
			select {
			case <-m.ctx.Done():
				return
			default:
			}

			m.log.Info(m.ctx, "attempting shard connect", slog.F("shard", shard))
			start := time.Now()
			err := s.Open(m.ctx, m.token)
			uptime := time.Since(start)

			failures = nextFailureCount(failures, uptime, s.TakeResumeDiscarded())
			delay := reconnectDelay(failures)

			if err != nil {
				// if !xerrors.Is(err, context.Canceled) {
				m.log.Error(m.ctx, "websocket closed",
					slog.F("shard", shard),
					slog.F("uptime", uptime),
					slog.F("consecutive_failures", failures),
					slog.F("retry_in", delay),
					slog.Error(err))
				// }
			}

			ok, woken := waitBeforeReconnect(m.ctx, wake, delay)
			if !ok {
				return
			}
			if woken {
				// An operator asked for this reconnect, so give them the prompt
				// first attempt rather than an escalated delay -- and do not let
				// their own cancellation of a short-lived connection, counted as
				// a failure just above, escalate the ladder.
				m.log.Info(m.ctx, "reconnect wait interrupted by management request", slog.F("shard", shard))
				failures = 0
			}
		}
	}()
}

// wakeShard nudges a shard's reconnect loop to attempt now instead of waiting
// out its backoff. Open clears Session.cur on return, so Session.Cancel is a
// no-op for the whole of that wait; without this a RestartShard would report
// success and do nothing for up to a full backoff interval.
//
// Non-blocking so the gRPC handler can never stall, and depth-1 buffered so
// signals coalesce and one sent while the loop is not waiting is held for its
// next wait rather than lost. An unknown shard is a no-op: a send on a nil
// channel blocks, so the default arm is taken.
func (m *Manager) wakeShard(shard int) {
	m.shardMu.Lock()
	ch := m.wake[shard]
	m.shardMu.Unlock()

	select {
	case ch <- struct{}{}:
	default:
	}
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

func createRedisClient(ctx context.Context, addr, name, podID string) (*redis.Client, error) {
	clientName := name
	if podID != "" {
		clientName = name + "-" + podID
	}

	rc := redis.NewClient(&redis.Options{
		Addr:       addr,
		ClientName: clientName,
	})

	_, err := rc.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return rc, nil
}
