package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtEmojiKey(guild, id string) fdb.Key {
	return s.Subs.Emojis.Pack(tuple.Tuple{guild, id})
}

func (s *Server) GetEmoji(ctx context.Context, req *pb.GetEmojiRequest) (*pb.GetEmojiResponse, error) {
	em := new(pb.Emoji)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtEmojiKey(req.GuildId, req.Id)).MustGet()
		if raw == nil {
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, em.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetEmojiResponse{
		Emoji: em,
	}, nil
}

func (s *Server) SetEmoji(ctx context.Context, req *pb.SetEmojiRequest) (*pb.SetEmojiResponse, error) {
	raw, err := req.Emoji.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtEmojiKey(req.Emoji.GuildId, req.Emoji.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateEmoji(ctx context.Context, req *pb.UpdateEmojiRequest) (*pb.UpdateEmojiResponse, error) {
	em := new(pb.Emoji)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtEmojiKey(req.GuildId, req.Id)).MustGet()

		err := em.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Emoji.Name != nil {
			em.Name = req.Emoji.Name.Value
		}
		if req.Emoji.Roles != nil {
			em.Roles = req.Emoji.Roles
		}
		if req.Emoji.Managed != nil {
			em.Managed = req.Emoji.Managed.Value
		}
		if req.Emoji.RequireColons != nil {
			em.RequireColons = req.Emoji.RequireColons.Value
		}

		raw, err = req.Emoji.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtEmojiKey(req.GuildId, req.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) DeleteEmoji(ctx context.Context, req *pb.DeleteEmojiRequest) (*pb.DeleteEmojiResponse, error) {
	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtEmojiKey(req.GuildId, req.Id))
		return nil, nil
	})

	return nil, err
}

func (s *Server) guildForEmoji(ctx context.Context, emoji, guild string) error {
	const sqlstr = `
		INSERT INTO public.emojis (
			"id", "guild"
		) VALUES (
			$1, $2
		) ON CONFLICT ("id") DO NOTHING
	`

	_, err := s.PDB.ExecContext(ctx, sqlstr, emoji, guild)
	return errors.Wrap(err, "failed to set guild for emoji")
}

func (s *Server) guildFromEmoji(emoji string) (g string, err error) {
	const sqlstr = `
		SELECT "guild" FROM public.emojis WHERE "id" = $1
	`

	err = errors.Wrap(
		s.PDB.QueryRow(sqlstr, emoji).Scan(&g),
		"failed to query guild from emoji",
	)
	return
}

func (s *Server) deleteEmojiFromID(ctx context.Context, id string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.emojis where "id" = $1
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

func (s *Server) deleteEmojisFromGuild(ctx context.Context, guild string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.emojis where "guild" = $1
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
