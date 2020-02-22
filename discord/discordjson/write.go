package discordjson

import jsoniter "github.com/json-iterator/go"

func (_ decoder) Write(obj interface{}) ([]byte, error) {
	return jsoniter.Marshal(obj)
}
