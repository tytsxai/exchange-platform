// Package errors 定义统一错误码
package errors

import (
	"fmt"
	"net/http"
)

// Code 错误码
type Code string

// 错误码定义
const (
	// 通用错误 (1xxx)
	CodeOK              Code = "OK"
	CodeUnknown         Code = "UNKNOWN"
	CodeInvalidParam    Code = "INVALID_PARAM"
	CodeInvalidRequest  Code = "INVALID_REQUEST"
	CodeNotFound        Code = "NOT_FOUND"
	CodeAlreadyExists   Code = "ALREADY_EXISTS"
	CodePermissionDenied Code = "PERMISSION_DENIED"
	CodeUnauthenticated Code = "UNAUTHENTICATED"
	CodeInternal        Code = "INTERNAL"
	CodeUnavailable     Code = "UNAVAILABLE"
	CodeTimeout         Code = "TIMEOUT"

	// 签名与鉴权 (2xxx)
	CodeInvalidSignature  Code = "INVALID_SIGNATURE"
	CodeInvalidTimestamp  Code = "INVALID_TIMESTAMP"
	CodeInvalidNonce      Code = "INVALID_NONCE"
	CodeInvalidApiKey     Code = "INVALID_API_KEY"
	CodeApiKeyDisabled    Code = "API_KEY_DISABLED"
	CodeApiKeyNoPermission Code = "API_KEY_NO_PERMISSION"
	CodeIpNotWhitelisted  Code = "IP_NOT_WHITELISTED"
	Code2FARequired       Code = "2FA_REQUIRED"
	CodeInvalid2FACode    Code = "INVALID_2FA_CODE"

	// 限流 (3xxx)
	CodeRateLimited       Code = "RATE_LIMITED"
	CodeTooManyRequests   Code = "TOO_MANY_REQUESTS"
	CodeOrderRateLimited  Code = "ORDER_RATE_LIMITED"
	CodeCancelRateLimited Code = "CANCEL_RATE_LIMITED"

	// 交易 (4xxx)
	CodeSymbolNotFound      Code = "SYMBOL_NOT_FOUND"
	CodeSymbolNotTrading    Code = "SYMBOL_NOT_TRADING"
	CodeInvalidSide         Code = "INVALID_SIDE"
	CodeInvalidOrderType    Code = "INVALID_ORDER_TYPE"
	CodeInvalidTimeInForce  Code = "INVALID_TIME_IN_FORCE"
	CodeInvalidPrice        Code = "INVALID_PRICE"
	CodeInvalidQuantity     Code = "INVALID_QUANTITY"
	CodePriceOutOfRange     Code = "PRICE_OUT_OF_RANGE"
	CodeQtyTooSmall         Code = "QTY_TOO_SMALL"
	CodeQtyTooLarge         Code = "QTY_TOO_LARGE"
	CodeNotionalTooSmall    Code = "NOTIONAL_TOO_SMALL"
	CodeOrderNotFound       Code = "ORDER_NOT_FOUND"
	CodeOrderAlreadyCanceled Code = "ORDER_ALREADY_CANCELED"
	CodeOrderAlreadyFilled  Code = "ORDER_ALREADY_FILLED"
	CodeDuplicateClientOrderId Code = "DUPLICATE_CLIENT_ORDER_ID"
	CodeSelfTradeBlocked    Code = "SELF_TRADE_BLOCKED"
	CodeMarketOrderNotAllowed Code = "MARKET_ORDER_NOT_ALLOWED"
	CodePostOnlyRejected    Code = "POST_ONLY_REJECTED"

	// 资金 (5xxx)
	CodeInsufficientBalance Code = "INSUFFICIENT_BALANCE"
	CodeInsufficientFrozen  Code = "INSUFFICIENT_FROZEN"
	CodeAssetNotFound       Code = "ASSET_NOT_FOUND"
	CodeFreezeFailure       Code = "FREEZE_FAILURE"
	CodeUnfreezeFailure     Code = "UNFREEZE_FAILURE"
	CodeSettleFailure       Code = "SETTLE_FAILURE"
	CodeIdempotencyConflict Code = "IDEMPOTENCY_CONFLICT"

	// 出入金 (6xxx)
	CodeDepositDisabled     Code = "DEPOSIT_DISABLED"
	CodeWithdrawDisabled    Code = "WITHDRAW_DISABLED"
	CodeWithdrawAmountTooSmall Code = "WITHDRAW_AMOUNT_TOO_SMALL"
	CodeWithdrawAmountTooLarge Code = "WITHDRAW_AMOUNT_TOO_LARGE"
	CodeInvalidAddress      Code = "INVALID_ADDRESS"
	CodeAddressNotWhitelisted Code = "ADDRESS_NOT_WHITELISTED"
	CodeWithdrawPending     Code = "WITHDRAW_PENDING"
	CodeWithdrawRejected    Code = "WITHDRAW_REJECTED"

	// 用户 (7xxx)
	CodeUserNotFound     Code = "USER_NOT_FOUND"
	CodeUserFrozen       Code = "USER_FROZEN"
	CodeUserDisabled     Code = "USER_DISABLED"
	CodeEmailExists      Code = "EMAIL_EXISTS"
	CodeInvalidPassword  Code = "INVALID_PASSWORD"
	CodeKycRequired      Code = "KYC_REQUIRED"

	// 系统 (9xxx)
	CodeSystemBusy       Code = "SYSTEM_BUSY"
	CodeServiceDegraded  Code = "SERVICE_DEGRADED"
	CodeMaintenanceMode  Code = "MAINTENANCE_MODE"
)

// Error 业务错误
type Error struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	RequestID string `json:"requestId,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// New 创建错误
func New(code Code, message string) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: isRetryable(code),
	}
}

// Newf 创建格式化错误
func Newf(code Code, format string, args ...interface{}) *Error {
	return New(code, fmt.Sprintf(format, args...))
}

// WithRequestID 添加请求 ID
func (e *Error) WithRequestID(requestID string) *Error {
	e.RequestID = requestID
	return e
}

// HTTPStatus 返回对应的 HTTP 状态码
func (e *Error) HTTPStatus() int {
	return httpStatus(e.Code)
}

// isRetryable 判断是否可重试
func isRetryable(code Code) bool {
	switch code {
	case CodeRateLimited, CodeTooManyRequests, CodeSystemBusy,
		CodeTimeout, CodeUnavailable, CodeInvalidTimestamp:
		return true
	default:
		return false
	}
}

// httpStatus 错误码对应的 HTTP 状态码
func httpStatus(code Code) int {
	switch code {
	case CodeOK:
		return http.StatusOK
	case CodeInvalidParam, CodeInvalidRequest, CodeInvalidPrice,
		CodeInvalidQuantity, CodeInvalidSide, CodeInvalidOrderType:
		return http.StatusBadRequest
	case CodeUnauthenticated, CodeInvalidSignature, CodeInvalidApiKey,
		CodeInvalidTimestamp, CodeInvalidNonce, CodeInvalid2FACode:
		return http.StatusUnauthorized
	case CodePermissionDenied, CodeApiKeyNoPermission, CodeIpNotWhitelisted,
		Code2FARequired, CodeUserFrozen, CodeKycRequired:
		return http.StatusForbidden
	case CodeNotFound, CodeOrderNotFound, CodeUserNotFound,
		CodeSymbolNotFound, CodeAssetNotFound:
		return http.StatusNotFound
	case CodeAlreadyExists, CodeDuplicateClientOrderId, CodeIdempotencyConflict:
		return http.StatusConflict
	case CodeRateLimited, CodeTooManyRequests, CodeOrderRateLimited:
		return http.StatusTooManyRequests
	case CodeInternal, CodeUnknown:
		return http.StatusInternalServerError
	case CodeUnavailable, CodeSystemBusy, CodeMaintenanceMode:
		return http.StatusServiceUnavailable
	case CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// 预定义错误
var (
	ErrInvalidParam       = New(CodeInvalidParam, "invalid parameter")
	ErrNotFound           = New(CodeNotFound, "not found")
	ErrUnauthenticated    = New(CodeUnauthenticated, "unauthenticated")
	ErrPermissionDenied   = New(CodePermissionDenied, "permission denied")
	ErrInsufficientBalance = New(CodeInsufficientBalance, "insufficient balance")
	ErrOrderNotFound      = New(CodeOrderNotFound, "order not found")
	ErrSymbolNotFound     = New(CodeSymbolNotFound, "symbol not found")
	ErrRateLimited        = New(CodeRateLimited, "rate limited")
	ErrSystemBusy         = New(CodeSystemBusy, "system busy, please retry")
)
