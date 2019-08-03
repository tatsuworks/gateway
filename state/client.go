package state

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
)

type Client struct {
	fdb  fdb.Database
	subs *Subspaces
}

func NewClient() *Client {
	fdb.MustAPIVersion(610)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"etfstate"}, nil)
	if err != nil {
		panic(err.Error())
	}

	return &Client{
		subs: NewSubspaces(dir),
		fdb:  db,
	}
}

// Subspaces is a struct containing all of the different subspaces used.
type Subspaces struct {
	Channels    subspace.Subspace
	Guilds      subspace.Subspace
	Members     subspace.Subspace
	Messages    subspace.Subspace
	Presences   subspace.Subspace
	Users       subspace.Subspace
	Roles       subspace.Subspace
	VoiceStates subspace.Subspace
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
	// PresenceSubspaceName is the enum for the presence subspace.
	PresenceSubspaceName
	// UserSubspaceName is the enum for the user subspace.
	UserSubspaceName
	// RoleSubspaceName is the enum for the role subspace.
	RoleSubspaceName
	// VoiceStateSubspaceName is the enum for the voice state subspace.
	VoiceStateSubspaceName
)

// NewSubspaces returns an instantiated Subspaces with the correct subspaces.
func NewSubspaces(dir directory.DirectorySubspace) *Subspaces {
	return &Subspaces{
		Channels:    dir.Sub(ChannelSubspaceName),
		Guilds:      dir.Sub(GuildSubspaceName),
		Members:     dir.Sub(MemberSubspaceName),
		Messages:    dir.Sub(MessageSubspaceName),
		Presences:   dir.Sub(PresenceSubspaceName),
		Users:       dir.Sub(UserSubspaceName),
		Roles:       dir.Sub(RoleSubspaceName),
		VoiceStates: dir.Sub(VoiceStateSubspaceName),
	}
}
