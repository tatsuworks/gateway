package api

import (
	"fmt"
	"net/http"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
	"github.com/tatsuworks/state/internal/mwerr"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

var ErrNotFound = xerrors.New("resource not found")

// Transact is a helper around (fdb.Database).Transact which accepts a function that doesn't require a return value.
func (s *Server) Transact(fn func(t fdb.Transaction) error) error {
	_, err := s.fdb.Transact(func(t fdb.Transaction) (ret interface{}, err error) {
		return nil, fn(t)
	})

	return errors.Wrap(err, "failed to commit fdb txn")
}

// ReadTransact is a helper around (fdb.Database).ReadTransact which accepts a function that doesn't require a return value.
func (s *Server) ReadTransact(fn func(t fdb.ReadTransaction) error) error {
	_, err := s.fdb.ReadTransact(func(t fdb.ReadTransaction) (ret interface{}, err error) {
		return nil, fn(t)
	})

	return errors.Wrap(err, "failed to commit fdb read txn")
}

func wrapHandler(fn func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		err := fn(w, r, p)
		if err != nil {
			var (
				msg  = err.Error()
				code = http.StatusInternalServerError
			)

			if perr, ok := err.(mwerr.Public); ok {
				msg, code = perr.Public()
			}

			fmt.Println(msg)
			http.Error(w, msg, code)
		}
	})
}

func (s *Server) setETFs(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) error {
	eg := new(errgroup.Group)

	send := func(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) {
		eg.Go(func() error {
			return s.Transact(func(t fdb.Transaction) error {
				for id, e := range etfs {
					t.Set(key(guild, id), e)
				}

				return nil
			})
		})

	}

	bufMap := etfs
	if len(etfs) > 1000 {
		bufMap = make(map[int64][]byte, 1000)

		for i, e := range etfs {
			bufMap[i] = e

			if len(bufMap) >= 1000 {
				send(guild, bufMap, key)
				bufMap = make(map[int64][]byte, 1000)
			}
		}
	}

	send(guild, bufMap, key)
	return eg.Wait()
}

func (s *Server) fmtChannelKey(id int64) fdb.Key {
	return s.subs.Channels.Pack(tuple.Tuple{id})
}

func (s *Server) fmtChannelPrefix() fdb.Key {
	return s.subs.Channels.FDBKey()
}

func (s *Server) fmtGuildChannelKey(guild, id int64) fdb.Key {
	return s.subs.Channels.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtGuildChannelPrefix(guild int64) fdb.Key {
	return s.subs.Channels.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtGuildKey(guild int64) fdb.Key {
	return s.subs.Guilds.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtGuildPrefix() fdb.Key {
	return s.subs.Guilds.FDBKey()
}

func (s *Server) fmtGuildBanKey(guild, user int64) fdb.Key {
	return s.subs.Guilds.Pack(tuple.Tuple{guild, "bans", user})
}

func (s *Server) fmtGuildMemberKey(guild, id int64) fdb.Key {
	return s.subs.Members.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtGuildMemberPrefix(guild int64) fdb.Key {
	return s.subs.Members.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtMemberPrefix() fdb.Key {
	return s.subs.Members.FDBKey()
}

func (s *Server) fmtChannelMessageKey(channel, id int64) fdb.Key {
	return s.subs.Messages.Pack(tuple.Tuple{channel, id})
}

func (s *Server) fmtChannelMessagePrefix(channel int64) fdb.Key {
	return s.subs.Messages.Pack(tuple.Tuple{channel})
}

func (s *Server) fmtMessagePrefix() fdb.Key {
	return s.subs.Messages.FDBKey()
}

func (s *Server) fmtMessageReactionKey(channel, id, user int64, name interface{}) fdb.Key {
	return s.subs.Messages.Pack(tuple.Tuple{channel, id, "rxns", user, name})
}

func (s *Server) fmtGuildPresenceKey(guild, id int64) fdb.Key {
	return s.subs.Presences.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtGuildPresencePrefix(guild int64) fdb.Key {
	return s.subs.Presences.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtPresencePrefix() fdb.Key {
	return s.subs.Presences.FDBKey()
}

func (s *Server) fmtGuildRoleKey(guild, id int64) fdb.Key {
	return s.subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtGuildRolePrefix(guild int64) fdb.Key {
	return s.subs.Roles.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtRolePrefix() fdb.Key {
	return s.subs.Roles.FDBKey()
}

func (s *Server) fmtGuildVoiceStateKey(guild, user int64) fdb.Key {
	return s.subs.VoiceStates.Pack(tuple.Tuple{guild, user})
}

func (s *Server) fmtGuildVoiceStatePrefix(guild int64) fdb.Key {
	return s.subs.VoiceStates.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtVoiceStatePrefix() fdb.Key {
	return s.subs.VoiceStates.FDBKey()
}
