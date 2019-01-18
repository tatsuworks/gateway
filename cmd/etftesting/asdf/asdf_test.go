package asdf

import (
	"testing"
)

const N = 1024 * 1024

type T = int64

var xForMakeCopy = make([]T, N)
var xForAppend = make([]T, N)
var yForMakeCopy []T
var yForAppend []T

func Benchmark_MakeAndCopy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		yForMakeCopy = make([]T, N)
		copy(yForMakeCopy, xForMakeCopy)
	}
}

func Benchmark_Append(b *testing.B) {
	for i := 0; i < b.N; i++ {
		yForAppend = append(xForAppend[:0:0], xForAppend...)
	}
}
