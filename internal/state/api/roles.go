package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuildRole(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	role, err := roleParam(p)
	if err != nil {
		return xerrors.Errorf("read role param: %w", err)
	}

	ro, err := s.db.GetGuildRole(r.Context(), guild, role)
	if err != nil {
		return xerrors.Errorf("read role: %w", err)
	}

	if ro == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, ro)
}

func (s *Server) getGuildRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	if len(r.URL.Query()["id"]) > 0 {
		return s.getGuildRoleSlice(w, r, p)
	}
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	ros, err := s.db.GetGuildRoles(r.Context(), guild)
	if err != nil {
		return xerrors.Errorf("read roles: %w", err)
	}

	return s.writeTerms(w, ros)
}

func (s *Server) getGuildRoleSlice(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	g, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	var (
		ctx = r.Context()
		rs  = r.URL.Query()["id"]
		ros = make([][]byte, 0, len(rs))
	)

	for _, e := range rs {
		rr, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return xerrors.Errorf("parse role id: %w", err)
		}

		rol, err := s.db.GetGuildRole(ctx, g, rr)
		if err != nil {
			if xerrors.Is(err, ErrNotFound) {
				rol, _ = json.Marshal(EmptyObj{Id: e, IsEmpty: true})
			} else {
				return xerrors.Errorf("get role: %w", err)
			}
		}

		ros = append(ros, rol)
	}

	return s.writeTerms(w, ros)
}

func (s *Server) setGuildRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}

	var rolesData = make(map[string]interface{})
    if err := json.NewDecoder(r.Body).Decode(&rolesData); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return xerrors.Errorf("parse body json: %w", err)
    }

	var convertedRolesData = make(map[int64][]byte)

	for key,value := range rolesData {
		num, err := strconv.ParseInt(key,10,64);
		if err != nil {
			return xerrors.Errorf("convert role id from string to int64: %w", err)
		}

		memberJSON, err := json.Marshal(value);
		if err != nil {
			return xerrors.Errorf("convert role data to json format: %w", err)
		}

		convertedRolesData[num] = memberJSON
	}

	err = s.db.SetGuildRoles(r.Context(),guild,convertedRolesData)
	if err != nil {
		return xerrors.Errorf("update guild roles cache: %w", err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Guild roles set successfully"))
	return nil
}

func (s *Server) deleteGuildRolesById(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}

	var rolesID []string
    if err := json.NewDecoder(r.Body).Decode(&rolesID); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return xerrors.Errorf("parse body json: %w", err)
    }
	
	var rolesToDelete []int64
	for _, roleIdString := range rolesID {
		num, err := strconv.ParseInt(roleIdString,10,64);
		if err != nil {
			return xerrors.Errorf("convert role id from string to int64: %w", err)
		}
		rolesToDelete = append(rolesToDelete,num);
	}

	err = s.db.DeleteGuildRolesById(r.Context(),guild,rolesToDelete)
	if err != nil {
		return xerrors.Errorf("update guild roles cache: %w", err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Guild roles deleted successfully"))
	return nil
}

func (s *Server) deleteGuildRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}

	err = s.db.DeleteGuildRoles(r.Context(),guild)
	if err != nil {
		return xerrors.Errorf("update guild roles cache: %w", err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Guild roles deleted successfully"))
	return nil
}

func roleParam(p httprouter.Params) (int64, error) {
	r := p.ByName("role")
	ri, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		return 0, ErrInvalidArgument
	}

	return ri, nil
}
