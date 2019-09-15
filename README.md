m2proxy-tester
---

m2proxy-tester is a test suite for [m2proxy](https://github.com/atpons/m2proxy)

# How to run

use `go test`

```console
$ go test ./...
```

Automatically run the m2proxy inside test suite.

If you want to run a test suite under real memcached server,
Start memcached and pass `-memcached` flag to go test like below.

```console
$ go test ./... -memcached
```

# LICENSE

MIT

# Author

Fumihiro Ito