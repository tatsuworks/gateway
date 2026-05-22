package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/tatsuworks/gateway/gatewaypb"
	"google.golang.org/grpc"
)

const (
	shards = 1024
	perPod = 64
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <guildID> [addr]\n", os.Args[0])
		os.Exit(2)
	}
	_g := os.Args[1]
	guildID, err := strconv.ParseInt(_g, 10, 64)
	if err != nil {
		panic(err)
	}

	shard := (guildID >> 22) % shards
	podNum := int64(shard / perPod)
	fmt.Println(podNum, shard)

	addr := "0.0.0.0:80"
	if len(os.Args) >= 3 {
		addr = os.Args[2]
	}

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Fatal("failed to connect", err)
	}

	g := gatewaypb.NewGatewayClient(conn)
	_, err = g.RequestGuildMembers(context.Background(), &gatewaypb.RequestGuildMembersRequest{
		GuildId: guildID,
		Shard:   int32(shard),
	})
	if err != nil {
		panic(err)
	}
}
