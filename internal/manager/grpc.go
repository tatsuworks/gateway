package manager

import (
	"context"

	"cdr.dev/slog"
	"github.com/tatsuworks/gateway/gatewaypb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ gatewaypb.GatewayServer = &Manager{}

func (m *Manager) Stats(ctx context.Context, req *gatewaypb.EmptyRequest) (*gatewaypb.StatsResponse, error) {
	return &gatewaypb.StatsResponse{}, nil
}

func (m *Manager) Version(ctx context.Context, req *gatewaypb.EmptyRequest) (*gatewaypb.VersionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Version not implemented")
}

func (m *Manager) RestartShard(ctx context.Context, req *gatewaypb.RestartShardRequest) (*gatewaypb.EmptyResponse, error) {
	m.shardMu.Lock()
	s, ok := m.shards[int(req.Shard)]
	m.shardMu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown shard")
	}

	if req.ForceIdentify {
		m.log.Info(ctx, "force identifying shard", slog.F("shard", req.Shard))
		s.ForceIdentify()
	} else {
		s.Cancel()
	}
	// Cancel only reaches a live connection. If the shard is between attempts it
	// has no live connection to cancel, so wake its reconnect loop too -- and
	// with backoff that window is now up to a full delay, not a flat second.
	m.wakeShard(int(req.Shard))

	return &gatewaypb.EmptyResponse{}, nil
}

func (m *Manager) RequestGuildMembers(ctx context.Context, req *gatewaypb.RequestGuildMembersRequest) (*gatewaypb.EmptyResponse, error) {
	m.shardMu.Lock()
	s, ok := m.shards[int(req.Shard)]
	m.shardMu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown shard")
	}

	m.log.Info(ctx, "requesting members for guild", slog.F("guild", req.GuildId), slog.F("shard", req.Shard))
	s.RequestGuildMembers(req.GuildId)
	return &gatewaypb.EmptyResponse{}, nil
}
