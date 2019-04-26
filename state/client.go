package state

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http2"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/fngdevs/gateway/discordetf"
	"github.com/pkg/errors"
)

type Client struct {
	client *http.Client

	url url.URL

	fdb fdb.Database

	subs *Subspaces
}

func NewClient(url url.URL) (*Client, error) {
	fdb.MustAPIVersion(600)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"etfstate"}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open directory")
	}

	return &Client{
		client: &http.Client{
			Transport: &http2.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		subs: NewSubspaces(dir),
		fdb:  db,
		url:  url,
	}, nil
}

func (c *Client) fmtURL(event string) url.URL {
	swp := c.url
	swp.Path = fmt.Sprintf("/v1/events/%s", strings.ToLower(event))
	return swp
}

func (c *Client) SendEvent(e *discordetf.Event, full []byte) error {
	if e.T == "nil" {
		return nil
	}

	u := c.fmtURL(e.T)

	req, _ := http.NewRequest("POST", u.String(), bytes.NewReader(full))
	res, err := c.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send state request")
	}

	defer res.Body.Close()
	_, err = io.Copy(ioutil.Discard, res.Body)
	if err != nil {
		return errors.Wrap(err, "failed to discard body")
	}

	return nil
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
