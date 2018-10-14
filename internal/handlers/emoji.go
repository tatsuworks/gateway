package state

import (
	"context"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) SetEmoji(context.Context, *pb.SetEmojiRequest) (*pb.SetEmojiResponse, error) {
	panic("not implemented")
}

func (s *Server) GetEmoji(context.Context, *pb.GetEmojiRequest) (*pb.GetEmojiResponse, error) {
	panic("not implemented")
}
