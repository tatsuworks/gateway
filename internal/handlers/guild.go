package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtGuildKey(guild string) fdb.Key {
	return s.Subs.Guilds.Pack(tuple.Tuple{guild})
}

func (s *Server) GetGuild(ctx context.Context, req *pb.GetGuildRequest) (*pb.GetGuildResponse, error) {
	g := new(pb.Guild)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtGuildKey(req.Id)).MustGet()
		if raw == nil {
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, g.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetGuildResponse{
		Guild: g,
	}, nil
}

func (s *Server) SetGuild(ctx context.Context, req *pb.SetGuildRequest) (*pb.SetGuildResponse, error) {
	raw, err := req.Guild.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtGuildKey(req.Guild.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateGuild(ctx context.Context, req *pb.UpdateGuildRequest) (*pb.UpdateGuildResponse, error) {
	g := new(pb.Guild)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtGuildKey(req.Id)).MustGet()

		err := g.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Guild.Name != nil {
			g.Name = req.Guild.Name.Value
		}
		if req.Guild.Icon != nil {
			g.Icon = req.Guild.Icon.Value
		}
		if req.Guild.Region != nil {
			g.Region = req.Guild.Region.Value
		}
		if req.Guild.AfkChannelId != nil {
			g.AfkChannelId = req.Guild.AfkChannelId.Value
		}
		if req.Guild.EmbedChannelId != nil {
			g.EmbedChannelId = req.Guild.EmbedChannelId.Value
		}
		if req.Guild.OwnerId != nil {
			g.OwnerId = req.Guild.OwnerId.Value
		}
		if req.Guild.JoinedAt != nil {
			g.JoinedAt = req.Guild.JoinedAt.Value
		}
		if req.Guild.Splash != nil {
			g.Splash = req.Guild.Splash.Value
		}
		if req.Guild.AfkTimeout != nil {
			g.AfkTimeout = req.Guild.AfkTimeout.Value
		}
		if req.Guild.MemberCount != nil {
			g.MemberCount = req.Guild.MemberCount.Value
		}
		if req.Guild.VerificationLevel != nil {
			g.VerificationLevel = req.Guild.VerificationLevel.Value
		}
		if req.Guild.EmbedEnabled != nil {
			g.EmbedEnabled = req.Guild.EmbedEnabled.Value
		}
		if req.Guild.Large != nil {
			g.Large = req.Guild.Large.Value
		}
		if req.Guild.DefaultMessageNotifications != nil {
			g.DefaultMessageNotifications = req.Guild.DefaultMessageNotifications.Value
		}

		raw, err = req.Guild.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtGuildKey(req.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) DeleteGuild(ctx context.Context, req *pb.DeleteGuildRequest) (*pb.DeleteGuildResponse, error) {
	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtGuildKey(req.Id))

		// clear channels
		preChan, _ := fdb.PrefixRange(s.fmtChannelKey(req.Id, ""))
		tx.ClearRange(preChan)

		// clear emojis
		preEm, _ := fdb.PrefixRange(s.fmtEmojiKey(req.Id, ""))
		tx.ClearRange(preEm)

		// clear members
		preMem, _ := fdb.PrefixRange(s.fmtMemberKey(req.Id, ""))
		tx.ClearRange(preMem)

		// TODO: clear messages

		// clear roles
		preRole, _ := fdb.PrefixRange(s.fmtRoleKey(req.Id, ""))
		tx.ClearRange(preRole)

		return nil, nil
	})

	return &pb.DeleteGuildResponse{}, err
}
