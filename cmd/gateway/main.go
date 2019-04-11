package main

import (
	"context"

	"github.com/fngdevs/gateway/internal/gatewayws"
)

func main() {
	sess := gatewayws.NewSession()
	err := sess.Open(context.Background(), "")
	if err != nil {
		panic(err)
	}
}
