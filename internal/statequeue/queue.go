package statequeue

import (
	"context"
	"unsafe"

	"cdr.dev/slog"
	"github.com/go-redis/redis"
)

type Server struct {
	log   slog.Logger
	shard int

	queue *redis.Client
}

func (s *Server) runShard(ctx context.Context) {
	for {
		parts, err := s.queue.BLPop(0, "gateway:cache").Result()
		if err != nil {
			s.log.Error(ctx, "failed to pop event", slog.Error(err))
		}

		var (
			key  = parts[0]
			ev   = unsafeBytes(&parts[1])
			_, _ = key, ev
		)
	}
}

func (s *Server) getEvent() {
}

func unsafeBytes(str *string) []byte {
	return *(*[]byte)(unsafe.Pointer(str))
}
