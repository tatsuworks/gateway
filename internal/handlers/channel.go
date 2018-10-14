package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

func (s *Server) fmtChannelKey(guild, channel string) fdb.Key {
	return s.Subs.Channels.Pack(tuple.Tuple{guild, channel})
}

// SetChannel is the handler for updating a channel's state.
func (s *Server) SetChannel(ctx context.Context, req *pb.SetChannelRequest) (*pb.SetChannelResponse, error) {
	raw, err := req.Channel.Marshal()
	if err != nil {
		return nil, err
	}

	s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtChannelKey(req.Channel.GuildId, req.Channel.Id), raw)
		return nil, nil
	})

	return nil, nil
}

// GetChannel is the handler for retrieving a channel from the state.
func (s *Server) GetChannel(ctx context.Context, req *pb.GetChannelRequest) (*pb.GetChannelResponse, error) {
	ch := new(pb.Channel)

	_, err := s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtChannelKey(req.GuildId, req.Id)).MustGet()

		err := ch.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetChannelResponse{
		Channel: ch,
	}, nil
}
