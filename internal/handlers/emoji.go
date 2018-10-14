package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtEmojiKey(guild, id string) fdb.Key {
	return s.Subs.Emojis.Pack(tuple.Tuple{guild, id})
}

func (s *Server) SetEmoji(ctx context.Context, req *pb.SetEmojiRequest) (*pb.SetEmojiResponse, error) {
	raw, err := req.Emoji.Marshal()
	if err != nil {
		return nil, err
	}

	s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtEmojiKey(req.Emoji.GuildId, req.Emoji.Id), raw)
		return nil, nil
	})

	return nil, nil
}

func (s *Server) GetEmoji(ctx context.Context, req *pb.GetEmojiRequest) (*pb.GetEmojiResponse, error) {
	em := new(pb.Emoji)

	_, err := s.DB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtEmojiKey(req.GuildId, req.Id)).MustGet()

		err := em.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetEmojiResponse{
		Emoji: em,
	}, nil
}
