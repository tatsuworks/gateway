package state

import (
	"context"

	"go.uber.org/zap"

	"github.com/pkg/errors"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtChannelKey(guild, channel string) fdb.Key {
	return s.Subs.Channels.Pack(tuple.Tuple{guild, channel})
}

func (s *Server) GetChannel(ctx context.Context, req *pb.GetChannelRequest) (*pb.GetChannelResponse, error) {
	ch := new(pb.Channel)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtChannelKey(req.GuildId, req.Id)).MustGet()
		if raw == nil {
			ch = nil
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, ch.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetChannelResponse{
		Channel: ch,
	}, nil
}

func (s *Server) SetChannel(ctx context.Context, req *pb.SetChannelRequest) (*pb.SetChannelResponse, error) {
	raw, err := req.Channel.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtChannelKey(req.Channel.GuildId, req.Channel.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateChannel(ctx context.Context, req *pb.UpdateChannelRequest) (*pb.UpdateChannelResponse, error) {
	ch := new(pb.Channel)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtChannelKey(req.GuildId, req.Id)).MustGet()

		err := ch.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Channel.Name != nil {
			ch.Name = req.Channel.Name.Value
		}
		if req.Channel.Topic != nil {
			ch.Topic = req.Channel.Topic.Value
		}
		if req.Channel.Nsfw != nil {
			ch.Nsfw = req.Channel.Nsfw.Value
		}
		if req.Channel.Position != nil {
			ch.Position = req.Channel.Position.Value
		}
		if req.Channel.Bitrate != nil {
			ch.Bitrate = req.Channel.Bitrate.Value
		}
		if req.Channel.Overwrites != nil {
			ch.Overwrites = req.Channel.Overwrites
		}
		if req.Channel.ParentId != nil {
			ch.ParentId = req.Channel.ParentId.Value
		}

		raw, err = ch.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtChannelKey(ch.GuildId, ch.Id), raw)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateChannelResponse{
		Channel: ch,
	}, nil
}

func (s *Server) DeleteChannel(ctx context.Context, req *pb.DeleteChannelRequest) (*pb.DeleteChannelResponse, error) {
	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtChannelKey(req.GuildId, req.Id))

		// clear messages
		pre, _ := fdb.PrefixRange(s.fmtMessageKey(req.Id, "").FDBKey())
		tx.ClearRange(pre)

		return nil, nil
	})

	return nil, err
}

func (s *Server) guildForChannel(ctx context.Context, channel, guild string) error {
	const sqlstr = `
		INSERT INTO public.channels (
			"id", "guild"
		) VALUES (
			$1, $2
		) ON CONFLICT ("id") DO NOTHING
	`

	_, err := s.PDB.ExecContext(ctx, sqlstr, channel, guild)
	return errors.Wrap(err, "failed to set guild for channel")
}

func (s *Server) guildFromChannel(channel string) (g string, err error) {
	const sqlstr = `
		SELECT "guild" FROM public.channels WHERE "id" = $1
	`

	err = errors.Wrap(
		s.PDB.QueryRow(sqlstr, channel).Scan(&g),
		"failed to query guild from channel",
	)
	return
}

func (s *Server) channelsFromGuild(ctx context.Context, guild string) (channels []string, err error) {
	const sqlstr = `
		SELECT "id" FROM public.channels WHERE "guild" = $1
	`

	q, err := s.PDB.QueryContext(ctx, sqlstr, guild)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query channels from guild")
	}

	res := []string{}
	for q.Next() {
		var c string

		err := q.Scan(&c)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan channel from guild")
		}

		res = append(res, c)
	}

	return res, nil
}

func (s *Server) deleteChannelFromID(ctx context.Context, id string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.channels where "id" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, id)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete channel from id")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}

func (s *Server) deleteChannelsFromGuild(ctx context.Context, guild string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.channels where "guild" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, guild)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete channels from guild")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}
