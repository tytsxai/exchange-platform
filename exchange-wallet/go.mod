module github.com/exchange/wallet

go 1.21

require (
	github.com/exchange/common v0.0.0
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.19.0
)

replace github.com/exchange/common => ../exchange-common
