package state

import (
	"git.friday.cafe/fndevs/state/pb"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"go.uber.org/zap"
)

var _ pb.StateServer = &Server{}

// Server ...
type Server struct {
	log *zap.Logger

	DB   fdb.Database
	Subs *Subspaces
}

// NewServer creates a new state Server.
func NewServer() *Server {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}

	fdb.MustAPIVersion(510)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"state"}, nil)
	if err != nil {
		panic("failed to create state directory:" + err.Error())
	}

	return &Server{
		log:  logger,
		DB:   db,
		Subs: NewSubspaces(dir),
	}
}

type Subspaces struct {
	Channels subspace.Subspace
	Emojis   subspace.Subspace
	Members  subspace.Subspace
	Messages subspace.Subspace
	Users    subspace.Subspace
}

type SubspaceName int

const (
	ChannelSubspaceName SubspaceName = iota
	EmojiSubspaceName
	MemberSubspaceName
	MessageSubspaceName
	UserSubspaceName
)

// NewSubspaces returns an instantiated Subspaces with the correct subspaces.
func NewSubspaces(dir directory.DirectorySubspace) *Subspaces {
	return &Subspaces{
		Channels: dir.Sub(ChannelSubspaceName),
		Emojis:   dir.Sub(EmojiSubspaceName),
		Members:  dir.Sub(MemberSubspaceName),
		Messages: dir.Sub(MessageSubspaceName),
		Users:    dir.Sub(UserSubspaceName),
	}
}
