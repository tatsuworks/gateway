package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtGuildKey(guild string) fdb.Key {
	return s.Subs.Guilds.Pack(tuple.Tuple{guild})
}

func (s *Server) GetGuild(ctx context.Context, req *pb.GetGuildRequest) (*pb.GetGuildResponse, error) {
	g := new(pb.Guild)

	_, err := s.DB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtGuildKey(req.Id)).MustGet()

		err := g.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetGuildResponse{
		Guild: g,
	}, nil
}

func (s *Server) SetGuild(ctx context.Context, req *pb.SetGuildRequest) (*pb.SetGuildResponse, error) {
	raw, err := req.Guild.Marshal()
	if err != nil {
		return nil, err
	}

	s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtGuildKey(req.Guild.Id), raw)
		return nil, nil
	})

	return nil, nil
}

func (s *Server) UpdateGuild(ctx context.Context, req *pb.UpdateGuildRequest) (*pb.UpdateGuildResponse, error) {
	return nil, nil
}

func (s *Server) DeleteGuild(ctx context.Context, req *pb.DeleteGuildRequest) (*pb.DeleteGuildResponse, error) {
	return nil, nil
}
