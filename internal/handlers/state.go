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

// Subspaces is a struct containing all of the different subspaces used.
type Subspaces struct {
	Channels subspace.Subspace
	Emojis   subspace.Subspace
	Guilds   subspace.Subspace
	Members  subspace.Subspace
	Messages subspace.Subspace
	Users    subspace.Subspace
}

// SubspaceName is an enum used to separate different subspaces.
type SubspaceName int

// If new enums need to be added, always append. If you are deprecating an enum never delete it.
const (
	// ChannelSubspaceName is the enum for the channel subspace.
	ChannelSubspaceName SubspaceName = iota
	// EmojiSubspaceName is the enum for the emoji subspace.
	EmojiSubspaceName
	// GuildSubspaceName is the enum for the guild subspace.
	GuildSubspaceName
	// MemberSubspaceName is the enum for the member subspace.
	MemberSubspaceName
	// MessageSubspaceName is the enum for the message subspace.
	MessageSubspaceName
	// UserSubspaceName is the enum for the user subspace.
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
