package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
)

func (s *Server) fmtUserKey(user string) fdb.Key {
	return nil
}

func (s *Server) SetUser(ctx context.Context, req *pb.SetUserRequest) (*pb.SetUserResponse, error) {
	panic("not implemented")
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	panic("not implemented")
}
