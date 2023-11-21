package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
	"strings"
)

func (s *Server) getGuildMember(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	member, err := memberParam(p)
	if err != nil {
		return xerrors.Errorf("read member param: %w", err)
	}
	m, err := s.db.GetGuildMember(r.Context(), guild, member)
	if err != nil {
		return xerrors.Errorf("read member: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, m)
}

func (s *Server) getGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	if len(r.URL.Query()["id"]) > 0 {
		return s.getGuildMemberSlice(w, r, p)
	}

	if r.URL.Query().Get("query") != "" {
		return s.searchGuildMembers(w, r, p)
	}
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	ms, err := s.db.GetGuildMembers(r.Context(), guild)
	if err != nil {
		return xerrors.Errorf("read members: %w", err)
	}

	return s.writeTerms(w, ms)
}

func (s *Server) getGuildMembersWithRole(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	role, err := roleParam(p)
	if err != nil {
		return xerrors.Errorf("read role param: %w", err)
	}
	ms, err := s.db.GetGuildMembersWithRole(r.Context(), guild, role)
	if err != nil {
		return xerrors.Errorf("read members: %w", err)
	}

	return s.writeTerms(w, ms)
}

func (s *Server) getGuildMemberSlice(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	g, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}

	var (
		ms  = r.URL.Query()["id"]
		mrs = make([][]byte, 0, len(ms))
	)

	for _, e := range ms {
		mr, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return xerrors.Errorf("parse member id: %w", err)
		}

		mbmr, err := s.db.GetGuildMember(r.Context(), g, mr)
		if err != nil {
			if xerrors.Is(err, ErrNotFound) {
				mbmr, _ = json.Marshal(EmptyObj{Id: e, IsEmpty: true})
			} else {
				return xerrors.Errorf("get member: %w", err)
			}
		}

		mrs = append(mrs, mbmr)
	}

	return s.writeTerms(w, mrs)
}

func (s *Server) searchGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	g, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	var (
		ctx   = r.Context()
		query = r.URL.Query().Get("query")
	)
	var ms [][]byte
	if query != "" && len(query) > 2 {
		ms, err = s.db.SearchGuildMembers(ctx, g, query)
		if err != nil {
			return xerrors.Errorf("search members: %w", err)
		}
	}

	return s.writeTerms(w, ms)
}

func (s *Server) getUserPresence(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	member, err := memberParam(p)
	if err != nil {
		return xerrors.Errorf("read member param: %w", err)
	}
	presence, err := s.db.GetUserPresence(r.Context(), guild, member)
	if err != nil {
		return xerrors.Errorf("read user presence: %w", err)
	}

	if presence == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, presence)
}

func memberParam(p httprouter.Params) (int64, error) {
	m := p.ByName("member")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		return 0, ErrInvalidArgument
	}

	return mi, nil
}

func userParam(p httprouter.Params) (int64, error) {
	u := p.ByName("user")
	ui, err := strconv.ParseInt(u, 10, 64)
	if err != nil {
		return 0, ErrInvalidArgument
	}

	return ui, nil
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	user, err := userParam(p)
	if err != nil {
		return xerrors.Errorf("read user param: %w", err)
	}
	m, err := s.db.GetUser(r.Context(), user)
	if err != nil {
		return xerrors.Errorf("read user: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, m)
}

func (s *Server) getUsers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	// id1,id2,id3
	userIDs := r.URL.Query().Get("userIDs")
	
	if userIDs == "" {
		return xerrors.Errorf("empty userids array")
	}

	userIDsSlice := strings.Split(userIDs, ",")

	var convertedUserIDs []int64

	for _, userId := range userIDsSlice {
		value, err := strconv.ParseInt(userId,10,64);
		if err != nil {
			return xerrors.Errorf("parse int for userids array: %w", err)
		}
		convertedUserIDs = append(convertedUserIDs,value)
	}

	data, err := s.db.GetUsersDiscordIdAndUsername(r.Context(), convertedUserIDs)
	if err != nil {
		return xerrors.Errorf("read users: %w", err)
	}

	result := make([][]byte, len(data))

    for i, userAndData := range data {
        // Convert each UserAndData to []byte
        userAndDataBytes, err := json.Marshal(userAndData)
        if err != nil {
            return xerrors.Errorf("converting user ids and discord data to [][]byte: %w", err)
        }

        result[i] = userAndDataBytes
    }

	return s.writeTerms(w, result)
}

// Define a handler function for SetGuildMembers
func (s *Server) setGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
    // Parse guild ID from the URL parameter
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	
	var guildMemberData = make(map[string]interface{})
    if err := json.NewDecoder(r.Body).Decode(&guildMemberData); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return xerrors.Errorf("parse body json: %w", err)
    }

	var convertedGuildMemberData = make(map[int64][]byte)

	for key,value := range guildMemberData {
		num, err := strconv.ParseInt(key,10,64);
		if err != nil {
			return xerrors.Errorf("convert discord id from string to int64: %w", err)
		}

		memberJSON, err := json.Marshal(value);
		if err != nil {
			return xerrors.Errorf("convert guild member data to json format: %w", err)
		}

		convertedGuildMemberData[num] = memberJSON
	}
	
	err = s.db.SetGuildMembers(r.Context(),guild,convertedGuildMemberData)
	if err != nil {
		if err != nil {
			return xerrors.Errorf("update guild members cache: %w", err)
		}
	}

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Guild members set successfully"))
	return nil
}