package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) SetGuild(context.Context, *pb.SetGuildRequest) (*pb.SetGuildResponse, error) {
	panic("not implemented")
}

func (s *Server) GetGuild(context.Context, *pb.GetGuildRequest) (*pb.GetGuildResponse, error) {
	panic("not implemented")
}
