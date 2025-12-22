module github.com/exchange/order

go 1.25.5

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/exchange/common v0.0.0
	github.com/go-redis/redismock/v9 v9.2.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.19.0
	github.com/redis/go-redis/v9 v9.17.2
)

replace github.com/exchange/common => ../exchange-common

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/sys v0.16.0 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
)
