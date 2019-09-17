package tester

import (
	"fmt"
	"net"
	"testing"

	"github.com/dropbox/godropbox/memcache"
)

func BenchmarkGet(b *testing.B) {
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", Port))
	if err != nil {
		b.Fatal(err)
	}
	client := memcache.NewRawBinaryClient(0, conn)
	client.Flush(0)
	client.Set(&memcache.Item{Key: "benchmark_1", Value: []byte("benchmark_1")})

	for n := 0; n < b.N; n++ {
		client.Get("benchmark_1")
	}
}
