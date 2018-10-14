package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) SetMember(context.Context, *pb.SetMemberRequest) (*pb.SetMemberResponse, error) {
	panic("not implemented")
}

func (s *Server) GetMember(context.Context, *pb.GetMemberRequest) (*pb.GetMemberResponse, error) {
	panic("not implemented")
}
