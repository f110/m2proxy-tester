package tester

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	"github.com/atpons/m2proxy/pkg/server"
	"github.com/atpons/m2proxy/pkg/storage"
	"github.com/dropbox/godropbox/errors"
	"github.com/dropbox/godropbox/memcache"
)

var (
	Port         int
	UseMemcached = false
)

func init() {
	flag.BoolVar(&UseMemcached, "memcached", false, "Use real memcached (port: 11211)")
}

func getClient(t *testing.T) memcache.ClientShard {
	if UseMemcached {
		Port = 11211
	}
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", Port))
	if err != nil {
		t.Fatal(err)
	}
	client := memcache.NewRawBinaryClient(0, conn)
	client.Flush(0)
	return client
}

func TestMain(m *testing.M) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port := addr.Port
	if err := l.Close(); err != nil {
		panic(err)
	}
	Port = port

	st := storage.NewLruStorage()
	s := server.NewServer(fmt.Sprintf(":%d", port), &st)
	go s.ListenAndServe()

	os.Exit(m.Run())
}

func TestGet(t *testing.T) {
	client := getClient(t)
	client.Set(&memcache.Item{Key: "get_ok", Value: []byte("atpons")})

	t.Run("KeyNotFound", func(t *testing.T) {
		res := client.Get("key_not_found")
		checkResponse(t, res, memcache.StatusKeyNotFound)
	})

	t.Run("KeyExists", func(t *testing.T) {
		res := client.Get("get_ok")
		checkResponse(t, res, memcache.StatusNoError)

		if !bytes.Equal([]byte("atpons"), res.Value()) {
			t.Errorf("value is unexpected result: %s", res.Value())
		}
	})

	t.Run("LargeValue", func(t *testing.T) {
		v := make([]byte, 512*1024) // 512 kB
		n, err := rand.Read(v)
		if err != nil {
			t.Fatal(err)
		}
		if n != 512*1024 {
			t.Fatal("Failed generate large value")
		}
		client.Set(&memcache.Item{Key: "get_large", Value: v})

		res := client.Get("get_large")
		checkResponse(t, res, memcache.StatusNoError)

		if len(res.Value()) != 512*1024 {
			t.Errorf("unexpected value size: %d", len(res.Value()))
		}
	})

	t.Run("CAS", func(t *testing.T) {
		res := client.Get("get_ok")
		checkResponse(t, res, memcache.StatusNoError)

		if res.DataVersionId() == 0 {
			t.Errorf("expect to have CAS but get response doesn't have")
		}
	})
}

func TestSet(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		res := client.Set(&memcache.Item{Key: "set_ok", Value: []byte("atpons")})
		checkResponse(t, res, memcache.StatusNoError)
		if res.Key() != "set_ok" {
			t.Errorf("Couldn't get key from set response: %s", res.Key())
		}
	})

	t.Run("Large", func(t *testing.T) {
		v := make([]byte, 2*1024) // 2 kB
		n, err := rand.Read(v)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2*1024 {
			t.Fatal("Failed generate value")
		}

		res := client.Set(&memcache.Item{Key: "set_large", Value: v})
		checkResponse(t, res, memcache.StatusNoError)
		if res.Key() != "set_large" {
			t.Errorf("Couldn't get key from set response: %s", res.Key())
		}
	})

	t.Run("Expiration", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "set_expiration", Value: []byte("atpons"), Expiration: 1})
		time.Sleep(2 * time.Second) // wait for expiration

		res := client.Get("set_expiration")
		checkResponse(t, res, memcache.StatusKeyNotFound)
	})

	t.Run("SetFlag", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "set_flag", Value: []byte("foobar"), Flags: 3})
		res := client.Get("set_flag")
		checkResponse(t, res, memcache.StatusNoError)

		if res.Flags() != 3 {
			t.Errorf("expect flags 3: %d", res.Flags())
		}
	})
}

func TestDelete(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "del_ok", Value: []byte("ok")})
		delRes := client.Delete("del_ok")
		checkResponse(t, delRes, memcache.StatusNoError)

		res := client.Get("del_ok")
		checkResponse(t, res, memcache.StatusKeyNotFound)
	})
}

func TestIncrement(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		res := client.Increment("incr_normal", 1, 1, 60)
		checkResponse(t, res, memcache.StatusNoError)
		if res.Key() != "incr_normal" {
			t.Errorf("key incr_normal: %s", res.Key())
		}
		if res.Count() != 1 {
			t.Errorf("count is expected 1: %d", res.Count())
		}
	})
}

func TestDecrement(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		res := client.Decrement("decr_normal", 1, 10, 60)
		checkResponse(t, res, memcache.StatusNoError)
		if res.Key() != "decr_normal" {
			t.Errorf("key decr_normal: %s", res.Key())
		}
		if res.Count() != 10 {
			t.Errorf("count is expected 10: %d", res.Count())
		}
	})
}

func TestCAS(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "cas_ok", Value: []byte("before")})
		res := client.Get("cas_ok")
		checkResponse(t, res, memcache.StatusNoError)

		casRes := client.Set(&memcache.Item{Key: "cas_ok", Value: []byte("after"), DataVersionId: res.DataVersionId()})
		checkResponse(t, casRes, memcache.StatusNoError)

		v := client.Get("cas_ok")
		if !bytes.Equal(v.Value(), []byte("after")) {
			t.Errorf("unexpected value: %v", v.Value())
		}
	})

	t.Run("InvalidCASKey", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "cas_ok", Value: []byte("before")})
		res := client.Get("cas_ok")
		checkResponse(t, res, memcache.StatusNoError)

		casRes := client.Set(&memcache.Item{Key: "cas_ok", Value: []byte("after"), DataVersionId: res.DataVersionId() + 1})
		err, ok := casRes.Error().(errors.DropboxError)
		if !ok {
			t.Fatal("unexpected error type")
		}
		if err.GetMessage() != "Key exists" {
			t.Errorf("expect key exists: %s", casRes.Error().Error())
		}
	})
}

func TestAdd(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		res := client.Add(&memcache.Item{Key: "add_normal", Value: []byte("before")})
		checkResponse(t, res, memcache.StatusNoError)
	})

	t.Run("KeyExists", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "add_keyexists", Value: []byte("ok")})
		res := client.Add(&memcache.Item{Key: "add_keyexists", Value: []byte("failure")})

		err, ok := res.Error().(errors.DropboxError)
		if !ok {
			t.Fatal("unexpected error type")
		}
		if err.GetMessage() != "Key exists" {
			t.Errorf("expect key exists: %s", err.GetMessage())
		}
	})
}

func TestReplace(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "replace_normal", Value: []byte("foobar")})
		res := client.Replace(&memcache.Item{Key: "replace_normal", Value: []byte("ok")})
		checkResponse(t, res, memcache.StatusNoError)
	})

	t.Run("KeyNotExist", func(t *testing.T) {
		res := client.Replace(&memcache.Item{Key: "replace_not", Value: []byte("foobar")})
		err, ok := res.Error().(errors.DropboxError)
		if !ok {
			t.Fatal("unexpected error type")
		}
		if err.GetMessage() != "Key not found" {
			t.Errorf("expect key not found: %s", err.GetMessage())
		}
	})
}

func TestAppend(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "append_normal", Value: []byte("at")})
		res := client.Append("append_normal", []byte("pons"))
		checkResponse(t, res, memcache.StatusNoError)

		get := client.Get("append_normal")
		if !bytes.Equal([]byte("atpons"), get.Value()) {
			t.Errorf("unexpected value: %s", get.Value())
		}
	})
}

func TestPrepend(t *testing.T) {
	client := getClient(t)

	t.Run("Normal", func(t *testing.T) {
		client.Set(&memcache.Item{Key: "prepend_normal", Value: []byte("pons")})
		res := client.Prepend("prepend_normal", []byte("at"))
		checkResponse(t, res, memcache.StatusNoError)

		get := client.Get("prepend_normal")
		if !bytes.Equal([]byte("atpons"), get.Value()) {
			t.Errorf("unexpected value: %s", get.Value())
		}
	})
}

func TestVersion(t *testing.T) {
	client := getClient(t)

	res := client.Version()
	checkResponse(t, res, memcache.StatusNoError)
	for _, v := range res.Versions() {
		if v == "" {
			t.Error("Couldn't not get version")
		}
	}
}

func TestStats(t *testing.T) {
	client := getClient(t)

	res := client.Stat("")
	checkResponse(t, res, memcache.StatusNoError)
}

func checkResponse(t *testing.T, res memcache.Response, expectStatus memcache.ResponseStatus) {
	if res.Status() != expectStatus {
		t.Errorf("expectStatus %v: %v", expectStatus, res.Status())
	}
	if res.Error() != nil {
		t.Fatal(res.Error())
	}
}
