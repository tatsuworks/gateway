package manager

import (
	"context"

	"cdr.dev/slog"
	"github.com/tatsuworks/gateway/gatewaypb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ gatewaypb.GatewayServer = &Manager{}

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

	s.Cancel()
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
