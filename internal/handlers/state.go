package state

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"git.abal.moe/tatsu/state/pb"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/go-redis/redis"
	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ pb.StateServer = &Server{}

// Server ...
type Server struct {
	log *zap.Logger

	usePsql bool
	PDB     *sql.DB

	FDB fdb.Database

	useEs bool
	EDB   *elastic.Client

	RDB *redis.Client

	Subs *Subspaces

	indexMember chan *pb.Member
}

// NewServer creates a new state Server.
func NewServer(
	logger *zap.Logger,
	psql *sql.DB,
	ec *elastic.Client,
	rdb *redis.Client,
	usePsql, useEs bool,
) (*Server, error) {
	fdb.MustAPIVersion(510)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"state"}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open directory")
	}

	if useEs {
		err = initEDB(logger, ec)
		if err != nil {
			return nil, err
		}
	}

	srv := &Server{
		log: logger,
		FDB: db,

		EDB:   ec,
		useEs: useEs,

		PDB:     psql,
		usePsql: usePsql,

		RDB: rdb,

		Subs: NewSubspaces(dir),

		indexMember: make(chan *pb.Member, 4096),
	}

	go srv.listenIndexes()

	return srv, nil
}

func (s *Server) listenIndexes() {
	var (
		mu sync.Mutex
	)

	if !s.useEs {
		for {
			<-s.indexMember
		}
	}

	bulk := s.EDB.Bulk().Index("members").Type("doc")

	reset := func() {
		waiting := len(s.indexMember)
		if bulk.NumberOfActions() == 0 {
			return
		}

		bulkOld := bulk
		bulk = s.EDB.Bulk().Index("members").Type("doc")

		go func() {
			start := time.Now()
			_, err := bulkOld.Do(context.Background())
			if err != nil {
				s.log.Error("failed to send members bulk request", zap.Error(err))
				return
			}
			s.log.Info("sent bulk index request",
				zap.String("took", time.Since(start).String()),
				zap.Int("amt_waiting", waiting),
			)
		}()
	}

	go func() {
		for {
			select {
			case <-time.After(10 * time.Second):
				mu.Lock()
				reset()
				mu.Unlock()
			}
		}
	}()

	for m := range s.indexMember {
		mu.Lock()
		if bulk.NumberOfActions() >= 256 {
			reset()
		}

		// if m.Id == "" {
		// 	s.log.Warn("member id is empty", zap.Any("member", *m))
		// }

		bulk.Add(elastic.NewBulkIndexRequest().Id(fmtMembersIndex(m.GuildId, m.Id)).Doc(m))
		mu.Unlock()
	}
}

// Subspaces is a struct containing all of the different subspaces used.
type Subspaces struct {
	Channels subspace.Subspace
	Guilds   subspace.Subspace
	Members  subspace.Subspace
	Messages subspace.Subspace
	Users    subspace.Subspace
	Roles    subspace.Subspace
}

// If new enums need to be added, always append. If you are deprecating an enum never delete it.
const (
	// ChannelSubspaceName is the enum for the channel subspace.
	ChannelSubspaceName = iota
	// GuildSubspaceName is the enum for the guild subspace.
	GuildSubspaceName
	// MemberSubspaceName is the enum for the member subspace.
	MemberSubspaceName
	// MessageSubspaceName is the enum for the message subspace.
	MessageSubspaceName
	// UserSubspaceName is the enum for the user subspace.
	UserSubspaceName
	// RoleSubspaceName is the enum for the role subspace.
	RoleSubspaceName
)

// NewSubspaces returns an instantiated Subspaces with the correct subspaces.
func NewSubspaces(dir directory.DirectorySubspace) *Subspaces {
	return &Subspaces{
		Channels: dir.Sub(ChannelSubspaceName),
		Guilds:   dir.Sub(GuildSubspaceName),
		Members:  dir.Sub(MemberSubspaceName),
		Messages: dir.Sub(MessageSubspaceName),
		Users:    dir.Sub(UserSubspaceName),
		Roles:    dir.Sub(RoleSubspaceName),
	}
}

func RequiredFieldsInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if t, ok := req.(interface{ GetId() string }); ok {
			if t.GetId() == "" {
				return nil, status.Error(codes.InvalidArgument, "id must not be empty")
			}
		}

		if t, ok := req.(interface{ GetGuildId() string }); ok {
			if t.GetGuildId() == "" {
				return nil, status.Error(codes.InvalidArgument, "guild_id must not be empty")
			}
		}

		if t, ok := req.(interface{ GetChannelId() string }); ok {
			if t.GetChannelId() == "" {
				return nil, status.Error(codes.InvalidArgument, "channel_id must not be empty")
			}
		}

		return handler(ctx, req)
	}
}

func initEDB(logger *zap.Logger, e *elastic.Client) error {
	logger.Info("ensuring elastic indexes...")

	exists, err := e.IndexExists("members").Do(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to index exists")
	}

	if exists {
		logger.Info("index already exists. skipping...")
		return nil
	}

	c, err := e.CreateIndex("members").Do(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to create elastic members index")
	}

	logger.Info("members index created", zap.Bool("shards_acknowledged", c.ShardsAcknowledged))

	return nil
}

func liftPDB(err error, msg string) error {
	if err == nil {
		return nil
	}

	if errors.Cause(err) == sql.ErrNoRows {
		return status.Error(codes.NotFound, errors.Wrap(err, msg).Error())
	}

	return status.Error(codes.Internal, errors.Wrap(err, msg).Error())
}

func (s *Server) AddPendingOp(guild string) (int32, error) {
	ops, err := s.RDB.Incr(fmtPendingOpsKey(guild)).Result()
	return int32(ops), err
}

func (s *Server) OpDone(guild string) {
	err := s.RDB.Decr(fmtPendingOpsKey(guild)).Err()
	if err != nil {
		s.log.Error("failed to decrement pending operations", zap.Error(err))
	}
}

func fmtPendingOpsKey(guild string) string {
	return fmt.Sprintf("state:pending_operations:%s", guild)
}
