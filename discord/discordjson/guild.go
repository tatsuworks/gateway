package discordjson

import (
	"strconv"

	jsoniter "github.com/json-iterator/go"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func rawsToMapBySnowflake(raws []jsoniter.RawMessage, key string) (map[int64][]byte, error) {
	var ids = map[int64][]byte{}

	for _, raw := range raws {
		id, err := snowflakeFromObject(raw, key)
		if err != nil {
			return nil, xerrors.Errorf("get id from object: %w", err)
		}

		ids[id] = raw
	}

	return ids, nil
}

func snowflakeFromObject(raw []byte, key string) (int64, error) {
	idStr := jsoniter.Get(raw, key).ToString()
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse id: %w", err)
	}

	return id, nil
}

func snowflakeFromObjectOptional(raw []byte, key string) (int64, error) {
	idStr := jsoniter.Get(raw, key).ToString()
	if idStr == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, xerrors.Errorf("parse id: %w", err)
	}

	return id, nil
}

func nestedRawsToMapBySnowflake(raws []jsoniter.RawMessage, obj string) (map[int64][]byte, error) {
	var ids = map[int64][]byte{}

	for _, raw := range raws {
		id, err := idFromNestedObject(raw, obj)
		if err != nil {
			return nil, xerrors.Errorf("get id from object: %w", err)
		}

		ids[id] = raw
	}

	return ids, nil
}

func idFromNestedObject(raw []byte, obj string) (int64, error) {
	var objRaw jsoniter.RawMessage
	jsoniter.Get(raw, obj).ToVal(&objRaw)

	return snowflakeFromObject(objRaw, "id")
}

func (_ decoder) DecodeGuildCreate(buf []byte) (*discord.GuildCreate, error) {
	var (
		gc = &discord.GuildCreate{
			ID:          0,
			Raw:         nil,
			MemberCount: 0,
			Channels:    map[int64][]byte{},
			Emojis:      map[int64][]byte{},
			Members:     map[int64][]byte{},
			Presences:   map[int64][]byte{},
			Roles:       map[int64][]byte{},
			VoiceStates: map[int64][]byte{},
		}
		gStream    = jsoniter.NewStream(jsoniter.ConfigFastest, nil, 0)
		firstField = true
		err        error
	)

	writeComma := func() {
		if firstField {
			firstField = false
			return
		}

		gStream.WriteMore()
	}

	gStream.WriteObjectStart()

	iter := jsoniter.ParseBytes(jsoniter.ConfigFastest, buf)
	if ok := iter.ReadMapCB(func(iter *jsoniter.Iterator, key string) bool {
		switch key {
		case "id":
			idStr := iter.ReadString()
			gc.ID, err = strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				err = xerrors.Errorf("parse guild id: %w", err)
				return false
			}

			writeComma()
			gStream.WriteObjectField(key)
			gStream.WriteString(idStr)

		case "member_count":
			gc.MemberCount = int64(iter.ReadUint64())

			writeComma()
			gStream.WriteObjectField(key)
			gStream.WriteInt64(gc.MemberCount)

		case "channels":
			var channels []jsoniter.RawMessage
			iter.ReadVal(&channels)
			gc.Channels, err = rawsToMapBySnowflake(channels, "id")
			if err != nil {
				err = xerrors.Errorf("map channels by id: %w", err)
				return false
			}

		case "emojis":
			var emojis []jsoniter.RawMessage
			iter.ReadVal(&emojis)
			gc.Emojis, err = rawsToMapBySnowflake(emojis, "id")
			if err != nil {
				err = xerrors.Errorf("map emojis by id: %w", err)
				return false
			}

		case "members":
			var members []jsoniter.RawMessage
			iter.ReadVal(&members)
			gc.Members, err = nestedRawsToMapBySnowflake(members, "user")
			if err != nil {
				err = xerrors.Errorf("map members by id: %w", err)
				return false
			}

		case "presences":
			var presences []jsoniter.RawMessage
			iter.ReadVal(&presences)
			gc.Presences, err = nestedRawsToMapBySnowflake(presences, "user")
			if err != nil {
				err = xerrors.Errorf("map presences by id: %w", err)
				return false
			}

		case "roles":
			var roles []jsoniter.RawMessage
			iter.ReadVal(&roles)
			gc.Roles, err = rawsToMapBySnowflake(roles, "id")
			if err != nil {
				err = xerrors.Errorf("map roles by id: %w", err)
				return false
			}

		case "voice_states":
			var voiceStates []jsoniter.RawMessage
			iter.ReadVal(&voiceStates)
			gc.VoiceStates, err = rawsToMapBySnowflake(voiceStates, "user_id")
			if err != nil {
				err = xerrors.Errorf("map voiceStates by id: %w", err)
				return false
			}

		default:
			writeComma()
			gStream.WriteObjectField(key)
			iter.ReadAny().WriteTo(gStream)
		}

		return true
	}); !ok {
		if iter.Error != nil {
			return nil, iter.Error
		}

		return nil, err
	}

	gStream.WriteObjectEnd()
	gc.Raw = gStream.Buffer()

	return gc, nil
}

func (_ decoder) DecodeGuildBan(buf []byte) (*discord.GuildBan, error) {
	var (
		gb  discord.GuildBan
		err error
	)

	gb.UserID, err = idFromNestedObject(buf, "user")
	if err != nil {
		return nil, xerrors.Errorf("extract user id: %w", err)
	}

	gb.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	return &gb, nil
}
