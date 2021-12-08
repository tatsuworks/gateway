package gatewayws

import (
	"context"
	"database/sql"

	"cdr.dev/slog"
	"golang.org/x/xerrors"
)

func (s *Session) persistReadyGuilds() {

}

func (s *Session) persistSeq() {
	err := s.stateDB.SetSequence(context.Background(), s.shardID, s.name, s.seq)
	if err != nil {
		s.log.Error(s.ctx, "save seq", slog.Error(err))
	}
}

func (s *Session) loadSeq() {
	var err error
	s.seq, err = s.stateDB.GetSequence(context.Background(), s.shardID, s.name)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		s.log.Error(s.ctx, "load session id", slog.Error(err))
	}
}

func (s *Session) persistSessID() {
	err := s.stateDB.SetSessionID(context.Background(), s.shardID, s.name, s.sessID)
	if err != nil {
		s.log.Error(s.ctx, "save seq", slog.Error(err))
	}
}

func (s *Session) loadSessID() {
	var err error
	s.sessID, err = s.stateDB.GetSessionID(context.Background(), s.shardID, s.name)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		s.log.Error(s.ctx, "load session id", slog.Error(err))
	}
}

func (s *Session) persistStatus() {
	err := s.stateDB.SetStatus(context.Background(), s.shardID, s.name, s.curState)
	if err != nil {
		s.log.Error(s.ctx, "save status", slog.Error(err))
	}
}
