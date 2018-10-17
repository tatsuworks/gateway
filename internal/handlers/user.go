package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtUserKey(user string) fdb.Key {
	return s.Subs.Users.Pack(tuple.Tuple{user})
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	u := new(pb.User)

	_, err := s.DB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtUserKey(req.Id)).MustGet()
		return nil, u.Unmarshal(raw)
	})

	return &pb.GetUserResponse{
		User: u,
	}, err
}

func (s *Server) SetUser(ctx context.Context, req *pb.SetUserRequest) (*pb.SetUserResponse, error) {
	raw, err := req.User.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtUserKey(req.User.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	return nil, nil
}

func (s *Server) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	return nil, nil
}
