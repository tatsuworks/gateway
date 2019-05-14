package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"golang.org/x/net/http2"
	"io/ioutil"
	"net/http"
	"time"
)

var _ = http2.Transport{}

func main() {
	bod, err := ioutil.ReadFile("173184118492889089.GUILD_CREATE.etf.bin")
	if err != nil {
		panic(err)
	}

	c := http.Client{
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	start := time.Now()
	req, err := http.NewRequest("POST", "https://localhost:8080/v1/events/guild_create", bytes.NewBuffer(bod))
	if err != nil {
		panic(err)
	}

	res, err := c.Do(req)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	out, err := ioutil.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}

	took := time.Since(start)
	fmt.Println(string(out), "took:", took)
}
