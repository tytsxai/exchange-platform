module github.com/exchange/admin

go 1.25.5

require (
	github.com/exchange/common v0.0.0
	github.com/lib/pq v1.10.9
)

replace github.com/exchange/common => ../exchange-common
