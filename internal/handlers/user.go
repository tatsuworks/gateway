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

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtUserKey(req.Id)).MustGet()
		if raw == nil {
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, u.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetUserResponse{
		User: u,
	}, nil
}

func (s *Server) SetUser(ctx context.Context, req *pb.SetUserRequest) (*pb.SetUserResponse, error) {
	raw, err := req.User.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtUserKey(req.User.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	u := new(pb.User)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtUserKey(req.Id)).MustGet()

		err := u.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.User.Email != nil {
			u.Email = req.User.Email.Value
		}
		if req.User.Username != nil {
			u.Username = req.User.Username.Value
		}
		if req.User.Avatar != nil {
			u.Avatar = req.User.Avatar.Value
		}
		if req.User.Discriminator != nil {
			u.Discriminator = req.User.Discriminator.Value
		}
		if req.User.Token != nil {
			u.Token = req.User.Token.Value
		}
		if req.User.Verified != nil {
			u.Verified = req.User.Verified.Value
		}
		if req.User.MfaEnabled != nil {
			u.MfaEnabled = req.User.MfaEnabled.Value
		}
		if req.User.Bot != nil {
			u.Bot = req.User.Bot.Value
		}

		raw, err = req.User.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtUserKey(req.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtUserKey(req.Id))
		return nil, nil
	})

	return nil, err
}
