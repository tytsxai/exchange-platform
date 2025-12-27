module github.com/exchange/admin

go 1.25

require (
	github.com/exchange/common v0.0.0
	github.com/lib/pq v1.10.9
)

require github.com/prometheus/client_golang v1.19.0 // indirect

replace github.com/exchange/common => ../exchange-common
