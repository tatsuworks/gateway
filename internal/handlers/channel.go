package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) SetChannel(context.Context, *pb.SetChannelRequest) (*pb.SetChannelResponse, error) {
	panic("not implemented")
}

func (s *Server) GetChannel(context.Context, *pb.GetChannelRequest) (*pb.GetChannelResponse, error) {
	panic("not implemented")
}
