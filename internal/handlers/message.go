package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) SetMessage(context.Context, *pb.SetMessageRequest) (*pb.SetMessageResponse, error) {
	panic("not implemented")
}

func (s *Server) GetMessage(context.Context, *pb.GetMessageRequest) (*pb.GetMessageResponse, error) {
	panic("not implemented")
}
