package types

// OpenOrder 启动恢复用的挂单快照（来自数据库）
type OpenOrder struct {
	OrderID       int64
	ClientOrderID string
	UserID        int64
	Symbol        string
	Side          string // BUY/SELL
	OrderType     string // LIMIT/MARKET
	TimeInForce   string // GTC/IOC/FOK/POST_ONLY
	Price         int64
	LeavesQty     int64 // 剩余数量
	CreatedAt     int64 // 纳秒时间戳
}
