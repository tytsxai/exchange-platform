module github.com/exchange/user

go 1.25.5

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/exchange/common v0.0.0
	github.com/lib/pq v1.10.9
	github.com/pquerna/otp v1.4.0
	github.com/redis/go-redis/v9 v9.17.2
	golang.org/x/crypto v0.46.0
)

replace github.com/exchange/common => ../exchange-common

require (
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
)
