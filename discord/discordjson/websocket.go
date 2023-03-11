package discordjson

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

func (_ decoder) DecodeHello(buf []byte) (hbInterval int, trace string, _ error) {
	hbInterval = jsoniter.Get(buf, "d", "heartbeat_interval").ToInt()

	return hbInterval, "", nil
}

func (_ decoder) DecodeReady(buf []byte) (guilds map[int64][]byte, version int, sessionID string,
	resumeGatewayURL string, _ error) {
	defer func() {
		err := recover()
		if err != nil {
			fmt.Println("bad ready guilds:", err)
		}
	}()

	version = jsoniter.Get(buf, "v").ToInt()
	sessionID = jsoniter.Get(buf, "session_id").ToString()
	resumeGatewayURL = jsoniter.Get(buf, "resume_gateway_url").ToString()

	var _guilds []jsoniter.RawMessage
	jsoniter.Get(buf, "guilds").ToVal(&_guilds)
	guilds, err := rawsToMapBySnowflake(_guilds, "id")
	if err != nil {
		return nil, 0, "", "", err
	}

	return guilds, version, sessionID, resumeGatewayURL, nil
}
