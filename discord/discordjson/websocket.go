package discordjson

import (
	jsoniter "github.com/json-iterator/go"
)

func (_ decoder) DecodeHello(buf []byte) (hbInterval int, trace string, _ error) {
	hbInterval = jsoniter.Get(buf, "d", "heartbeat_interval").ToInt()

	return hbInterval, "", nil
}

func (_ decoder) DecodeReady(buf []byte) (version int, sessionID string, _ error) {
	version = jsoniter.Get(buf, "v").ToInt()
	sessionID = jsoniter.Get(buf, "session_id").ToString()

	return
}
