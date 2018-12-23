package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func main() {
	bod, err := ioutil.ReadFile("173184118492889089.GUILD_CREATE.etf.bin")
	if err != nil {
		panic(err)
	}

	start := time.Now()
	req, err := http.Post("http://localhost:8080/v1/events/guild_create", "application/etf", bytes.NewBuffer(bod))
	if err != nil {
		panic(err)
	}
	defer req.Body.Close()

	out, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}

	took := time.Since(start)
	fmt.Println(string(out), "took:", took)
}
