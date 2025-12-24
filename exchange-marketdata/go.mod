module github.com/exchange/marketdata

go 1.21

require (
	github.com/exchange/common v0.0.0
	github.com/gorilla/websocket v1.5.3
	github.com/redis/go-redis/v9 v9.17.2
)

require (
	github.com/alicebob/miniredis/v2 v2.35.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
)

replace github.com/exchange/common => ../exchange-common
