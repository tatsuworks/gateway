package state

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"git.abal.moe/tatsu/state/pb"
)

func (s *Server) fmtMessageKey(channel, message string) fdb.Key {
	return s.Subs.Messages.Pack(tuple.Tuple{channel, message})
}

func (s *Server) GetMessage(ctx context.Context, req *pb.GetMessageRequest) (*pb.GetMessageResponse, error) {
	msg := new(pb.Message)

	_, err := s.FDB.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
		raw := tx.Get(s.fmtMessageKey(req.ChannelId, req.Id)).MustGet()
		if raw == nil {
			msg = nil
			// abal wants this to be idempotent i guess
			return nil, nil
		}

		return nil, msg.Unmarshal(raw)
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetMessageResponse{
		Message: msg,
	}, nil
}

func (s *Server) SetMessage(ctx context.Context, req *pb.SetMessageRequest) (*pb.SetMessageResponse, error) {
	raw, err := req.Message.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Set(s.fmtMessageKey(req.Message.ChannelId, req.Message.Id), raw)
		return nil, nil
	})

	return nil, err
}

func (s *Server) UpdateMessage(ctx context.Context, req *pb.UpdateMessageRequest) (*pb.UpdateMessageResponse, error) {
	msg := new(pb.Message)

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		raw := tx.Get(s.fmtMessageKey(req.ChannelId, req.Id)).MustGet()

		err := msg.Unmarshal(raw)
		if err != nil {
			return nil, err
		}

		if req.Message.Content != nil {
			msg.Content = req.Message.Content.Value
		}
		if req.Message.EditedTimestamp != nil {
			msg.EditedTimestamp = req.Message.EditedTimestamp.Value
		}
		if req.Message.MentionRoles != nil {
			msg.MentionRoles = req.Message.MentionRoles
		}

		raw, err = msg.Marshal()
		if err != nil {
			return nil, err
		}

		tx.Set(s.fmtMessageKey(req.ChannelId, req.Id), raw)
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateMessageResponse{
		Message: msg,
	}, nil
}

func (s *Server) DeleteMessage(ctx context.Context, req *pb.DeleteMessageRequest) (*pb.DeleteMessageResponse, error) {
	if req.Id == "*" {
		if req.ChannelId == "*" {
			return nil, status.Error(codes.InvalidArgument, "must provide a channel id")
		}

		_, err := s.deleteMessagesFromChannel(ctx, req.ChannelId)
		if err != nil {
			return nil, liftPDB(err, "failed to delete messages by channel")
		}

		_, err = s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
			pre, _ := fdb.PrefixRange(s.fmtMessageKey(req.ChannelId, ""))
			tx.ClearRange(pre)
			return nil, nil
		})

		return &pb.DeleteMessageResponse{}, err
	}

	_, err := s.FDB.Transact(func(tx fdb.Transaction) (interface{}, error) {
		tx.Clear(s.fmtMessageKey(req.ChannelId, req.Id))
		return nil, nil
	})

	return &pb.DeleteMessageResponse{}, err
}

func (s *Server) channelGuildForMessage(ctx context.Context, message, channel, guild string) error {
	const sqlstr = `
		INSERT INTO public.messages (
			"id", "channel", "guild"
		) VALUES (
			$1, $2, $3
		) ON CONFLICT ("id") DO NOTHING
	`

	_, err := s.PDB.ExecContext(ctx, sqlstr, message, channel, guild)
	return errors.Wrap(err, "failed to set channel and guild for message")
}

func (s *Server) channelGuildFromMessage(ctx context.Context, message string) (c, g string, err error) {
	const sqlstr = `
		SELECT "channel", "guild" FROM public.messages WHERE "id" = $1
	`

	err = errors.Wrap(
		s.PDB.QueryRowContext(ctx, sqlstr, message).Scan(&c, &g),
		"failed to query channel and guild from message",
	)
	return
}

func (s *Server) deleteMessageFromID(ctx context.Context, id string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.messages where "id" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, id)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete message from id")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}

func (s *Server) deleteMessagesFromGuild(ctx context.Context, guild string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.messages where "guild" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, guild)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete messages from guild")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}

func (s *Server) deleteMessagesFromChannel(ctx context.Context, channel string) (int64, error) {
	const sqlstr = `
		DELETE FROM public.messages where "guild" = $1
	`

	q, err := s.PDB.ExecContext(ctx, sqlstr, channel)
	if err != nil {
		return 0, errors.Wrap(err, "failed to delete messages from guild")
	}

	aff, err := q.RowsAffected()
	if err != nil {
		s.log.Error("failed to get rows affected", zap.Error(err))
	}

	return aff, nil
}
