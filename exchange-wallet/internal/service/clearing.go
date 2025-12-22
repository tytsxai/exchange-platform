package service

import (
	"context"

	"github.com/exchange/wallet/internal/client"
)

// ClearingClient 清算客户端接口
type ClearingClient interface {
	Freeze(ctx context.Context, req *client.FreezeRequest) error
	Unfreeze(ctx context.Context, req *client.UnfreezeRequest) error
	Deduct(ctx context.Context, req *client.DeductRequest) error
}
