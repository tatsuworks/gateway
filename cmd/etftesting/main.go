package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"time"

	"git.abal.moe/tatsu/state/discord"
	"git.abal.moe/tatsu/state/etf"
	"github.com/davecgh/go-spew/spew"
)

func main() {
	out, err := ioutil.ReadFile("173184118492889089.GUILD_CREATE.etf.bin")
	if err != nil {
		panic(err)
	}

	//fmt.Println(out[:10])
	//spew.Dump(out[:100])
	//
	//if true {
	//	return
	//}

	buf := bytes.NewBuffer(out)

	var (
		ctx = &etf.Context{}
		dec = ctx.NewDecoder(buf)
	)

	start := time.Now()
	term, err := dec.NextTerm()
	if err != nil {
		panic(err)
	}

	hmm := struct {
		D  discord.Guild
		Op int
		S  int
		T  string
	}{}

	err = etf.TermIntoStruct(term, &hmm)
	if err != nil {
		panic(err)
	}

	val := hmm.D.Channels[0]
	id := val[etf.Atom("id")]

	fmt.Println(time.Since(start))
	spew.Dump(id)

	//enc := new(bytes.Buffer)
	//ctx = &etf.Context{}
	//err = ctx.Write(enc, term)
	//if err != nil {
	//	panic(err)
	//}

	//spew.Dump(hmm)
}
