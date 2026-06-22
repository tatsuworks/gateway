package gatewayws

import (
	"context"
	"database/sql"
	"sync/atomic"

	"cdr.dev/slog"
	"golang.org/x/xerrors"
)

func (s *Session) persistShardInfo() {
	seq, sessID, resumeURL := atomic.LoadInt64(&s.seq), s.sessID, s.resumeURL
	if atomic.LoadInt32(&s.forceIdentify) == 1 {
		// A force-identify is pending: the resume tuple is being discarded, so
		// never persist it. This covers the connection's own deferred persist on
		// shutdown — between ForceIdentify and the next Open clearing the state —
		// so a crash in that window can't leave a resumable row behind.
		seq, sessID, resumeURL = 0, "", ""
	}
	err := s.stateDB.SetShardInfo(context.Background(), s.shardID, s.name, seq, sessID, resumeURL)
	if err != nil {
		s.log.Error(s.ctx, "save shard info", slog.Error(err))
	}
}

func (s *Session) loadSeq() {
	var err error
	s.seq, err = s.stateDB.GetSequence(context.Background(), s.shardID, s.name)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		s.log.Error(s.ctx, "load session id", slog.Error(err))
	}
}

func (s *Session) loadSessID() {
	var err error
	s.sessID, err = s.stateDB.GetSessionID(context.Background(), s.shardID, s.name)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		s.log.Error(s.ctx, "load session id", slog.Error(err))
	}
}

func (s *Session) loadResumeURL() {
	url, err := s.stateDB.GetResumeGatewayURL(context.Background(), s.shardID, s.name)
	if err != nil {
		s.log.Error(s.ctx, "load resume gateway url", slog.Error(err))
		return
	}
	s.resumeURL = url
}

func (s *Session) persistStatus() {
	err := s.stateDB.SetStatus(context.Background(), s.shardID, s.name, s.curState)
	if err != nil {
		s.log.Error(s.ctx, "save status", slog.Error(err))
	}
}
