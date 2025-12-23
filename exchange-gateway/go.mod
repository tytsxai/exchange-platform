module github.com/exchange/gateway

go 1.25.5

require (
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/gorilla/websocket v1.5.1
	github.com/redis/go-redis/v9 v9.17.2
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/exchange/common v0.0.0
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/net v0.20.0 // indirect
)

replace github.com/exchange/common => ../exchange-common
