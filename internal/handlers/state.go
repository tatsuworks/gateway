package state

import (
	"context"
	"database/sql"

	"github.com/olivere/elastic"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"git.friday.cafe/fndevs/state/pb"
)

var _ pb.StateServer = &Server{}

// Server ...
type Server struct {
	log *zap.Logger

	PDB  *sql.DB
	FDB  fdb.Database
	EDB  *elastic.Client
	Subs *Subspaces
}

// NewServer creates a new state Server.
func NewServer(logger *zap.Logger, psql *sql.DB, elastic *elastic.Client) (*Server, error) {
	fdb.MustAPIVersion(510)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"state"}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open directory")
	}

	return &Server{
		log:  logger,
		FDB:  db,
		EDB:  elastic,
		Subs: NewSubspaces(dir),
	}, nil
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

func liftPDB(err error, msg string) error {
	if err == nil {
		return nil
	}

	if errors.Cause(err) == sql.ErrNoRows {
		return status.Error(codes.NotFound, errors.Wrap(err, msg).Error())
	}

	return status.Error(codes.Internal, errors.Wrap(err, msg).Error())
}
