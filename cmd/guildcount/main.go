package main

import (
	"fmt"

	"github.com/tatsuworks/gateway/internal/state"
)

func main() {
	db, err := state.NewDB()
	must(err, "failed to init db")

	gCount, err := db.GetGuildCount()
	must(err, "failed to get guild count")

	fmt.Println("Guild count:", gCount)

	cCount, err := db.GetChannelCount()
	must(err, "failed to get channel count")

	fmt.Println("Channel count:", cCount)
}

func must(err error, msg string) {
	if err != nil {
		panic(msg + ": " + err.Error())
	}
}
