package state

import (
	"context"
	"time"

	"git.abal.moe/tatsu/state/pb"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"go.uber.org/zap"
)

func (s *Server) fmtChannelKey(guild, channel string) fdb.Key {
	return s.Subs.Channels.Pack(tuple.Tuple{guild, channel})
}

func (s *Server) GetChannel(ctx context.Context, req *pb.GetChannelRequest) (*pb.GetChannelResponse, error) {
	ch := new(pb.Channel)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtChannelKey(req.GuildId, req.Id)).MustGet()
		if raw == nil {
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, ch.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetChannelResponse{
		Channel: ch,
	}, nil
}

func (s *Server) SetChannel(ctx context.Context, req *pb.SetChannelRequest) (*pb.SetChannelResponse, error) {
	raw, err := req.Channel.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtChannelKey(req.Channel.GuildId, req.Channel.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateChannel(ctx context.Context, req *pb.UpdateChannelRequest) (*pb.UpdateChannelResponse, error) {
	ch := new(pb.Channel)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtChannelKey(req.GuildId, req.Id)).MustGet()

		err := ch.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Channel.Name != nil {
			ch.Name = req.Channel.Name.Value
		}
		if req.Channel.Topic != nil {
			ch.Topic = req.Channel.Topic.Value
		}
		if req.Channel.Nsfw != nil {
			ch.Nsfw = req.Channel.Nsfw.Value
		}
		if req.Channel.Position != nil {
			ch.Position = req.Channel.Position.Value
		}
		if req.Channel.Bitrate != nil {
			ch.Bitrate = req.Channel.Bitrate.Value
		}
		if req.Channel.Overwrites != nil {
			ch.Overwrites = req.Channel.Overwrites
		}
		if req.Channel.ParentId != nil {
			ch.ParentId = req.Channel.ParentId.Value
		}

		raw, err = ch.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtChannelKey(ch.GuildId, ch.Id), raw)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateChannelResponse{
		Channel: ch,
	}, nil
}

func (s *Server) DeleteChannel(ctx context.Context, req *pb.DeleteChannelRequest) (*pb.DeleteChannelResponse, error) {
	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtChannelKey(req.GuildId, req.Id))

		// clear messages
		pre, _ := fdb.PrefixRange(s.fmtMessageKey(req.Id, "").FDBKey())
		tx.ClearRange(pre)

		return nil, nil
	})

	return nil, err
}

func (s *Server) SetChannelChunk(ctx context.Context, req *pb.SetChannelChunkRequest) (*pb.SetChannelChunkResponse, error) {
	start := time.Now()
	ops, err := s.AddPendingOp(req.GuildId)
	if err != nil {
		return nil, err
	}

	go func() {
		defer s.OpDone(req.GuildId)

		raws := make([][]byte, 0, len(req.Channels))
		for _, channel := range req.Channels {
			raw, err := channel.Marshal()
			if err != nil {
				s.log.Error("failed to marshal channel", zap.Error(err))
				return
			}

			raws = append(raws, raw)
		}

		_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
			for i, channel := range req.Channels {
				tx.Set(s.fmtChannelKey(channel.GuildId, channel.Id), raws[i])
			}
			return nil, nil
		})
		if err != nil {
			s.log.Error("failed to commit channel chunk transaction", zap.Error(err))
		}

		s.log.Info("processed channel chunk", zap.Duration("took", time.Since(start)))
	}()

	return &pb.SetChannelChunkResponse{
		Ops: ops,
	}, nil
}
