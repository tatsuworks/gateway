package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/tatsuworks/gateway/protos/gatewaypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	shards = 1024
	perPod = 64
)

func main() {
	_g := os.Args[1]
	guildID, err := strconv.ParseInt(_g, 10, 64)
	if err != nil {
		panic(err)
	}

	shard := (guildID >> 22) % shards
	podNum := int64(shard / perPod)
	fmt.Println(podNum, shard)

	conn, err := grpc.Dial("0.0.0.0:80", grpc.WithTransportCredentials(insecure.NewCredentials()))
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
