package state

import (
	"context"
	"fmt"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"git.abal.moe/tatsu/state/pb"
)

func (s *Server) fmtMemberKey(guild, user string) fdb.Key {
	return s.Subs.Members.Pack(tuple.Tuple{guild, user})
}

func (s *Server) GetMember(ctx context.Context, req *pb.GetMemberRequest) (*pb.GetMemberResponse, error) {
	m := new(pb.Member)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtMemberKey(req.GuildId, req.Id)).MustGet()
		if raw == nil {
			m = nil
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, m.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetMemberResponse{
		Member: m,
	}, nil
}

func fmtMembersIndex(guild, user string) string {
	return fmt.Sprintf("%s:%s", guild, user)
}

func liftEDB(err error, msg string) error {
	if err == nil {
		return nil
	}

	if elastic.IsNotFound(err) {
		// errors.Wrap(err, "document not found")
		return status.Error(codes.NotFound, errors.Wrap(err, msg).Error())
	}

	if elastic.IsTimeout(err) {
		// errors.Wrap(err, "query timed out")
		return status.Error(codes.Internal, errors.Wrap(err, msg).Error())
	}

	if elastic.IsConnErr(err) {
		// errors.Wrap(err, "query timed out")
		return status.Error(codes.Internal, errors.Wrap(err, msg).Error())
	}

	return status.Error(codes.Internal, errors.Wrap(err, msg).Error())
}

func (s *Server) SetMember(ctx context.Context, req *pb.SetMemberRequest) (*pb.SetMemberResponse, error) {
	go func() {
		s.indexMember <- req.Member
	}()

	raw, err := req.Member.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtMemberKey(req.Member.GuildId, req.Member.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateMember(ctx context.Context, req *pb.UpdateMemberRequest) (*pb.UpdateMemberResponse, error) {
	m := new(pb.Member)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
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
			m.Roles = req.Member.Roles
		}

		raw, err = m.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtMemberKey(req.GuildId, req.Id), raw)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateMemberResponse{
		Member: m,
	}, nil
}

func (s *Server) DeleteMember(ctx context.Context, req *pb.DeleteMemberRequest) (*pb.DeleteMemberResponse, error) {
	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtMemberKey(req.GuildId, req.Id))
		return nil, nil
	})

	return nil, err
}

func (s *Server) SetMemberChunk(ctx context.Context, req *pb.SetMemberChunkRequest) (*pb.SetMemberChunkResponse, error) {
	eg := errgroup.Group{}

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		for _, member := range req.Members {
			member := member
			eg.Go(func() error {
				go func() {
					s.indexMember <- member
				}()

				rawUser, err := member.User.Marshal()
				if err != nil {
					return err
				}
				tx.Set(s.fmtUserKey(member.Id), rawUser)

				rawMember, err := member.Marshal()
				if err != nil {
					return err
				}

				tx.Set(s.fmtMemberKey(req.GuildId, member.User.Id), rawMember)
				return nil
			})
		}

		return nil, eg.Wait()
	})

	return nil, err
}
