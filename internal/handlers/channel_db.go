package state

import (
	"context"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

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
