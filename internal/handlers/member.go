package state

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"

	"git.friday.cafe/fndevs/state/pb"
)

func (s *Server) fmtMemberKey(guild, user string) fdb.Key {
	return s.Subs.Members.Pack(tuple.Tuple{guild, user})
}

func (s *Server) GetMember(ctx context.Context, req *pb.GetMemberRequest) (*pb.GetMemberResponse, error) {
	m := new(pb.Member)

	_, err := s.DB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtMemberKey(req.GuildId, req.Id)).MustGet()

		err := m.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetMemberResponse{
		Member: m,
	}, nil
}

func (s *Server) SetMember(ctx context.Context, req *pb.SetMemberRequest) (*pb.SetMemberResponse, error) {
	raw, err := req.Member.Marshal()
	if err != nil {
		return nil, err
	}

	s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtMemberKey(req.Member.GuildId, req.Member.Id), raw)
		return nil, nil
	})

	return nil, nil
}

func (s *Server) UpdateMember(ctx context.Context, req *pb.UpdateMemberRequest) (*pb.UpdateMemberResponse, error) {
	m := new(pb.Member)

	_, err := s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtMemberKey(req.GuildId, req.Id)).MustGet()

		err := m.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Member.JoinedAt != nil {
			m.JoinedAt = req.Member.JoinedAt.Value
		}
		if req.Member.Nick != nil {
			m.Nick = req.Member.Nick.Value
		}
		if req.Member.Deaf != nil {
			m.Deaf = req.Member.Deaf.Value
		}
		if req.Member.Mute != nil {
			m.Mute = req.Member.Mute.Value
		}
		if req.Member.Roles != nil {
			m.Roles = req.Member.Roles.Value
		}

		raw, err = req.Member.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtMemberKey(req.GuildId, req.Id), raw)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (s *Server) DeleteMember(ctx context.Context, req *pb.DeleteMemberRequest) (*pb.DeleteMemberResponse, error) {
	_, err := s.DB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtMemberKey(req.GuildId, req.Id))
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return nil, nil
}
