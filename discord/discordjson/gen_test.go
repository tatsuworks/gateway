package discordjson

import (
	"fmt"
	"testing"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/minio/simdjson-go"
)

var testDataSimd = []byte(`{"op": 10, "d": {"foo": "bar", "bar": "foo"}, "s": 42, "t": "GATEWAY_EVENT_NAME"}`)

func TestSimd(t *testing.T) {
	logger := slogtest.Make(t, nil)

	parsed, err := simdjson.Parse(testDataSimd, nil)
	if err != nil {
		logger.Error(nil, "failed to parse", slog.Error(err))
	}

	iter := parsed.Iter()
	for {
		typ := iter.Advance()
		if typ == simdjson.TypeNone {
			fmt.Println(typ.String())
			break
		}

		typ, iter, err := iter.Root(nil)
		if err != nil {
			logger.Error(nil, "failed to parse", slog.Error(err))
			return
		}

		obj, err := iter.Object(nil)
		if err != nil {
			logger.Error(nil, "failed to parse", slog.Error(err))
			return
		}

		if obj == nil {
			logger.Error(nil, "obj is nil?")
			return
		}

		ele := simdjson.Element{}
		e := obj.FindKey("op", &ele)
		if e == nil {
			logger.Error(nil, "op is nil")
			return
		}
		opInt, _ := ele.Iter.Int()
		fmt.Println(ele.Type.String(), opInt)
		e = obj.FindKey("t", &ele)
		if e == nil {
			logger.Error(nil, "t is nil")
			return
		}
		tStr, _ := ele.Iter.String()
		fmt.Println(ele.Type.String(), tStr)

		e = obj.FindKey("d", &ele)
		if e == nil {
			fmt.Println("test")
			logger.Error(nil, "t is nil")
			return
		}

		fmt.Println("test")
		testDataSimd = testDataSimd[:0]
		test2, err := ele.Iter.MarshalJSONBuffer(testDataSimd)
		if err != nil {
			fmt.Println("test")
			logger.Error(nil, "failed to parse", slog.Error(err))
			return
		}

		fmt.Println("test")
		fmt.Println(string(testDataSimd), test2)
	}
}
