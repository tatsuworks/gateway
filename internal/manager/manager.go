package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
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

	rdb  []*redis.Client
	etcd *clientv3.Client

	bufferPool *sync.Pool

	whitelistedEvents map[string]map[string]struct{}

	// stabilizeSem caps how many shards may be in the post-IDENTIFY
	// backfill window concurrently. Shared across all sessions started
	// by this manager. nil disables the gate.
	stabilizeSem         chan struct{}
	stabilizeDuration    time.Duration
	stabilizeMaxDuration time.Duration
	stabilizeUseDrain    bool
	identifyPacing       time.Duration
	divergenceRatio      float64

	sweepEnabled        bool
	sweepRequestsPerSec int
	sweepBatch          int

	// shardStart/shardStop are this process's owned shard range, set
	// by Start. logHealth and the sweep both need them; capturing once
	// avoids threading the values through every helper.
	shardStart int
	shardStop  int
}

// envDuration parses a positive integer-seconds env var, falling back
// to def when unset, empty, or invalid (with a warn log on invalid).
func envDuration(logger slog.Logger, ctx context.Context, name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		logger.Warn(ctx, "invalid duration env var, using default",
			slog.F("var", name), slog.F("raw", raw), slog.F("default", def.String()))
		return def
	}
	return time.Duration(n) * time.Second
}

func envInt(logger slog.Logger, ctx context.Context, name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		logger.Warn(ctx, "invalid int env var, using default",
			slog.F("var", name), slog.F("raw", raw), slog.F("default", def))
		return def
	}
	return n
}

func envBool(logger slog.Logger, ctx context.Context, name string, def bool) bool {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	switch raw {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		logger.Warn(ctx, "invalid bool env var, using default",
			slog.F("var", name), slog.F("raw", raw), slog.F("default", def))
		return def
	}
}

func envFloat(logger slog.Logger, ctx context.Context, name string, def float64) float64 {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < 0 {
		logger.Warn(ctx, "invalid float env var, using default",
			slog.F("var", name), slog.F("raw", raw), slog.F("default", def))
		return def
	}
	return f
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

	stabilizeUseDrain := envBool(cfg.Logger, ctx, "IDENTIFY_STABILIZE_USE_DRAIN", true)
	stabilizeConcurrency := envInt(cfg.Logger, ctx, "IDENTIFY_STABILIZE_CONCURRENCY", 1)
	// With drain on, the drain signal does the gating; the floor only
	// matters for shards that have nothing to drain, so a small value
	// suffices. With drain off (rollback), restore Phase 1's fixed-hold
	// behaviour by defaulting to the larger floor.
	defaultFloor := gatewayws.IdentifyStabilizeTime
	if stabilizeUseDrain {
		defaultFloor = 5 * time.Second
	}
	stabilizeDuration := envDuration(cfg.Logger, ctx, "IDENTIFY_STABILIZE_SECONDS", defaultFloor)
	stabilizeMax := envDuration(cfg.Logger, ctx, "IDENTIFY_STABILIZE_SECONDS_MAX", 120*time.Second)
	identifyPacing := envDuration(cfg.Logger, ctx, "IDENTIFY_PACING_SECONDS", gatewayws.IdentifyWaitTime)
	// Default divergence ratio is 0 (disabled): the divergence check
	// requires a per-GUILD_CREATE COUNT(*) on members which is too
	// expensive without a maintained guilds.member_count column. Ops
	// can opt in (e.g. 0.05) once that column lands or the cost is
	// validated. Until then, behavior matches pre-Phase-3 — every
	// GUILD_CREATE triggers a Request Guild Members.
	divergenceRatio := envFloat(cfg.Logger, ctx, "BACKFILL_DIVERGENCE_RATIO", 0)
	sweepEnabled := os.Getenv("BACKFILL_SWEEP_ENABLED") != "false"
	sweepRPS := envInt(cfg.Logger, ctx, "BACKFILL_SWEEP_REQUESTS_PER_SECOND", 12)
	sweepBatch := envInt(cfg.Logger, ctx, "BACKFILL_SWEEP_BATCH", 200)

	var stabilizeSem chan struct{}
	if stabilizeConcurrency > 0 && stabilizeDuration > 0 {
		stabilizeSem = make(chan struct{}, stabilizeConcurrency)
	}
	cfg.Logger.Info(ctx, "identify pacing config",
		slog.F("pacing", identifyPacing.String()),
		slog.F("stabilize_use_drain", stabilizeUseDrain),
		slog.F("stabilize_concurrency", stabilizeConcurrency),
		slog.F("stabilize_floor", stabilizeDuration.String()),
		slog.F("stabilize_max", stabilizeMax.String()),
		slog.F("divergence_ratio", divergenceRatio),
		slog.F("sweep_enabled", sweepEnabled),
		slog.F("sweep_rps", sweepRPS),
		slog.F("sweep_batch", sweepBatch))

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

		rdb:  rdbClients,
		etcd: etcdc,

		bufferPool: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},

		whitelistedEvents: redisWhitelistedEvents,

		stabilizeSem:         stabilizeSem,
		stabilizeDuration:    stabilizeDuration,
		stabilizeMaxDuration: stabilizeMax,
		stabilizeUseDrain:    stabilizeUseDrain,
		identifyPacing:       identifyPacing,
		divergenceRatio:      divergenceRatio,

		sweepEnabled:        sweepEnabled,
		sweepRequestsPerSec: sweepRPS,
		sweepBatch:          sweepBatch,
	}
}

func (m *Manager) Start(start, stop int) error {
	m.shardStart = start
	m.shardStop = stop

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
	go m.runGuildBackfillSweep(start, stop)
	return nil
}

func (m *Manager) startShard(shard int) {
	s, err := gatewayws.NewSession(&gatewayws.SessionConfig{
		Name:                 m.name,
		Logger:               m.log,
		DB:                   m.db,
		WorkGroup:            m.wg,
		Redis:                m.rdb,
		Etcd:                 m.etcd,
		Token:                m.token,
		Intents:              m.intents,
		ShardID:              shard,
		ShardCount:           m.shardCount,
		BufferPool:           m.bufferPool,
		WhitelistedEvents:    m.whitelistedEvents,
		StabilizeSem:         m.stabilizeSem,
		StabilizeDuration:    m.stabilizeDuration,
		StabilizeMaxDuration: m.stabilizeMaxDuration,
		StabilizeUseDrain:    m.stabilizeUseDrain,
		IdentifyPacing:       m.identifyPacing,
		DivergenceRatio:      m.divergenceRatio,
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

		// Cheap instability proxy: count shards whose persisted curState
		// is not in the steady-state inner loop. Persisted at ~1 minute
		// resolution by Session.logTotalEvents, so trends are what
		// matter, not single-tick spikes. Healthy fleet should trend
		// toward zero. Skipped if the DB backend doesn't support it.
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.log.Debug(ctx, "CountUnstableShards unsupported on this backend", slog.F("panic", r))
				}
			}()
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			n, err := m.db.CountUnstableShards(cctx, m.name, m.shardStart, m.shardStop)
			if err != nil {
				m.log.Warn(ctx, "CountUnstableShards failed", slog.Error(err))
				return
			}
			m.log.Info(ctx, "shard stability",
				slog.F("unstable", n),
				slog.F("total", m.shardStop-m.shardStart))
		}()

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
