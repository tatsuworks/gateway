package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-redis/redis"
)

var redisURL = os.Getenv("REDIS")

func main() {
	rc := redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	err := rc.Ping().Err()
	if err != nil {
		panic(err)
	}

	// _start, _end := os.Args[1], os.Args[2]
	// start, end := parse(_start), parse(_end)
	//
	// for i := start; i < end; i++ {
	// 	_, err := rc.Del(
	// 		fmt.Sprintf("gateway:sess:%d", i),
	// 		fmt.Sprintf("gateway:seq:%d", i),
	// 	).Result()
	// 	if err != nil {
	// 		panic("delete key: " + err.Error())
	// 	}
	// }

	keys, err := rc.Keys("gateway:sess::*").Result()
	if err != nil {
		panic(err)
	}

	for _, e := range keys {
		val, err := rc.Get(e).Result()
		if err != nil {
			panic(err)
		}

		sp := strings.Split(e, "::")
		rc.Set(fmt.Sprintf("%s:gateway-state:%s", sp[0], sp[1]), val, 0).Err()
	}
}

func parse(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}

	return i
}
