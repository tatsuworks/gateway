package gatewayws

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestIntents(t *testing.T) {
	ints := DefaultIntents.Collect()
	spew.Dump(DefaultIntents)
	fmt.Println(ints)
}
