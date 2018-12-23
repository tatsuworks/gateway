package grpcstate

import (
	"context"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (s *Server) guildForRole(ctx context.Context, role, guild string) error {
	const sqlstr = `
		INSERT INTO public.roles (
			"id", "guild"
		) VALUES (
			$1, $2
		) ON CONFLICT ("id") DO NOTHING
	`

	_, err := s.PDB.ExecContext(ctx, sqlstr, role, guild)
	return errors.Wrap(err, "failed to set guild for role")
}

func (s *Server) guildFromRole(role string) (g string, err error) {
	const sqlstr = `
		SELECT "guild" FROM public.roles WHERE "id" = $1
	`

	err = errors.Wrap(
		s.PDB.QueryRow(sqlstr, role).Scan(&g),
		"failed to query guild from role",
	)
	return
}

func (s *Server) deleteRoleFromID(ctx context.Context, id string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.roles where "id" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, id)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete role from id")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}

func (s *Server) deleteRolesFromGuild(ctx context.Context, guild string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.roles where "guild" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, guild)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete roles from guild")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}
