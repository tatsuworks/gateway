package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtRoleKey(guild, id string) fdb.Key {
	return s.Subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (s *Server) GetRole(ctx context.Context, req *pb.GetRoleRequest) (*pb.GetRoleResponse, error) {
	r := new(pb.Role)

	_, err := s.DB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtRoleKey(req.GuildId, req.Id)).MustGet()
		return nil, r.Unmarshal(raw)
	})

	return &pb.GetRoleResponse{
		Role: r,
	}, err
}

func (s *Server) SetRole(ctx context.Context, req *pb.SetRoleRequest) (*pb.SetRoleResponse, error) {
	raw, err := req.Role.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtRoleKey(req.Role.GuildId, req.Role.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateRole(ctx context.Context, req *pb.UpdateRoleRequest) (*pb.UpdateRoleResponse, error) {
	return nil, nil
}

func (s *Server) DeleteRole(ctx context.Context, req *pb.DeleteRoleRequest) (*pb.DeleteRoleResponse, error) {
	return nil, nil
}
