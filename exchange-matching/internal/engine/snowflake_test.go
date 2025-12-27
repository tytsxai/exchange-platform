package engine

import "github.com/exchange/common/pkg/snowflake"

func init() {
	// Ensure trade ID generation works in unit tests.
	_ = snowflake.Init(0)
}
