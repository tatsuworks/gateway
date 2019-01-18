package grpcstate

import (
	"context"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/fngdevs/state/pb"
	"go.uber.org/zap"
)

func (s *Server) fmtRoleKey(guild, id string) fdb.Key {
	return s.Subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (s *Server) GetRole(ctx context.Context, req *pb.GetRoleRequest) (*pb.GetRoleResponse, error) {
	r := new(pb.Role)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtRoleKey(req.GuildId, req.Id)).MustGet()
		if raw == nil {
			r = nil
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, r.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetRoleResponse{
		Role: r,
	}, nil
}

func (s *Server) SetRole(ctx context.Context, req *pb.SetRoleRequest) (*pb.SetRoleResponse, error) {
	raw, err := req.Role.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtRoleKey(req.Role.GuildId, req.Role.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateRole(ctx context.Context, req *pb.UpdateRoleRequest) (*pb.UpdateRoleResponse, error) {
	r := new(pb.Role)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtRoleKey(req.GuildId, req.Id)).MustGet()

		err := r.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Role.Name != nil {
			r.Name = req.Role.Name.Value
		}
		if req.Role.Managed != nil {
			r.Managed = req.Role.Managed.Value
		}
		if req.Role.Mentionable != nil {
			r.Mentionable = req.Role.Mentionable.Value
		}
		if req.Role.Hoist != nil {
			r.Hoist = req.Role.Hoist.Value
		}
		if req.Role.Color != nil {
			r.Color = req.Role.Color.Value
		}
		if req.Role.Position != nil {
			r.Position = req.Role.Position.Value
		}
		if req.Role.Permissions != nil {
			r.Permissions = req.Role.Permissions.Value
		}

		raw, err = r.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtRoleKey(req.GuildId, req.Id), raw)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateRoleResponse{
		Role: r,
	}, nil
}

func (s *Server) DeleteRole(ctx context.Context, req *pb.DeleteRoleRequest) (*pb.DeleteRoleResponse, error) {
	// _, err := s.deleteRoleFromID(ctx, req.Id)
	// if err != nil {
	// 	return nil, liftPDB(err, "failed to delete role by id")
	// }

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtRoleKey(req.GuildId, req.Id))
		return nil, nil
	})

	return nil, err
}

func (s *Server) SetRoleChunk(ctx context.Context, req *pb.SetRoleChunkRequest) (*pb.SetRoleChunkResponse, error) {
	start := time.Now()
	ops, err := s.AddPendingOp(req.GuildId)
	if err != nil {
		return nil, err
	}

	go func() {
		defer s.OpDone(req.GuildId)

		// because the fdb transaction will be retried if there conflicts, it's best to marshal
		// everything outside of the txn. also, we want to keep txn lengths low.
		raws := make([][]byte, 0, len(req.Roles))
		for _, role := range req.Roles {
			raw, err := role.Marshal()
			if err != nil {
				s.log.Error("failed to marshal role", zap.Error(err))
				return
			}

			raws = append(raws, raw)
		}

		_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
			for i, role := range req.Roles {
				tx.Set(s.fmtRoleKey(role.GuildId, role.Id), raws[i])
			}

			return nil, nil
		})
		if err != nil {
			s.log.Error("failed to commit role chunk transaction", zap.Error(err))
		}

		s.log.Info("processed role chunk", zap.Duration("took", time.Since(start)))
	}()

	return &pb.SetRoleChunkResponse{
		Ops: ops,
	}, nil
}
