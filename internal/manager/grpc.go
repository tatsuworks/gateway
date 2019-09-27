package manager

import (
	"context"

	"github.com/tatsuworks/gateway/gatewaypb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
