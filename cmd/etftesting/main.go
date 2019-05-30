package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/tatsuworks/state/discord"
	"github.com/tatsuworks/state/etf"
	"github.com/tatsuworks/state/etf/discordetf"
	"github.com/klauspost/compress/zlib"
)

var _ = json.Unmarshal

func xd() {
	fi, err := ioutil.ReadFile("173184118492889089.GUILD_CREATE.etf.bin")
	if err != nil {
		panic(err)
	}

	start := time.Now()

	e, err := discordetf.DecodeT(fi)
	if err != nil {
		panic(err)
	}

	//end := time.Since(start)
	//fmt.Println("took       ", end)
	//fmt.Println(e.S)
	//fmt.Println(string(e.S))
	//fmt.Println(len(lul))

	start = time.Now()

	gc, err := discordetf.DecodeGuildCreate(e.D)
	if err != nil {
		panic(err.Error())
	}

	//fmt.Println("guild", gc.Guild)

	fmt.Println("took       ", time.Since(start))
	fmt.Println("")
	fmt.Println("id         ", gc.Id)
	fmt.Println("channels   ", len(gc.Channels))
	fmt.Println("emojis     ", len(gc.Emojis))
	fmt.Println("members    ", len(gc.Members))
	fmt.Println("presences  ", len(gc.Presences))
	fmt.Println("roles      ", len(gc.Roles))
	fmt.Println("voicestates", len(gc.VoiceStates))

	if true {
		return
	}

	var (
		rd  = bytes.NewReader(gc.Members[0])
		dec = new(etf.Context).NewDecoder(rd)
	)

	term, err := dec.NextTerm()
	if err != nil {
		panic(err)
	}

	spew.Dump(term)
}

func main() {
	xd()
	//asdf()
	//abc123()
	//mainabc()

	if true {
		return
	}

	fi, err := os.Open("173184118492889089.GUILD_CREATE.etf.bin")
	if err != nil {
		panic(err)
	}

	buf := make([]byte, 2)
	fi.Read(buf)

	fmt.Println(buf)

	buf = make([]byte, 4)
	fi.Read(buf)

	l := binary.BigEndian.Uint32(buf)
	fmt.Println(buf, l)

	buf = make([]byte, 1)
	fi.Read(buf)
	fmt.Println(buf)

	buf = make([]byte, 2)
	fi.Read(buf)

	ll := binary.BigEndian.Uint16(buf)
	fmt.Println(buf, ll)

	buf = make([]byte, ll)
	fi.Read(buf)
	fmt.Println(buf, string(buf))

	buf = make([]byte, 500)
	fi.Read(buf)
	spew.Dump(buf)
}

func mainabc() {
	fi, err := ioutil.ReadFile("173184118492889089.GUILD_CREATE.etf.zlib.bin")
	if err != nil {
		panic(err)
	}

	buf := bytes.NewReader(fi)

	start := time.Now()
	buf.Read(make([]byte, 2))

	etfLenRaw := make([]byte, 4)
	buf.Read(etfLenRaw)

	etfLen := binary.BigEndian.Uint32(etfLenRaw)
	fmt.Println(etfLen)

	zlibReader, err := zlib.NewReader(buf)
	if err != nil {
		panic(err)
	}
	defer zlibReader.Close()

	rawEtf := make([]byte, etfLen)
	wrote, err := io.ReadFull(zlibReader, rawEtf)
	if err != nil {
		panic(err)
	}
	fmt.Println(wrote)

	end := time.Since(start)
	spew.Dump(end, float64(etfLen)/float64(wrote))
	//spew.Dump(rawEtf)
}

func asdf() {
	fi, err := os.Open("173184118492889089.GUILD_CREATE.etf.bin")
	if err != nil {
		panic(err)
	}
	var (
		dec = new(etf.Context).NewDecoder(fi)
	)

	term, err := dec.NextTerm()
	if err != nil {
		panic(err)
	}

	gc := new(etf.Term)
	err = etf.TermIntoStruct(term, gc)
	if err != nil {
		panic(err)
	}

	spew.Dump(gc)
}

type defaultVals struct {
	Op int
	S  int
	T  string
}

type guildCreate struct {
	D discord.Guild
	defaultVals
}
