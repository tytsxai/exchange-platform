package validate

import (
	stderrors "errors"
	"net/mail"
	"regexp"
	"strings"

	commonerrors "github.com/exchange/common/pkg/errors"
)

const defaultPrecision = 8

var (
	symbolAllowedRe        = regexp.MustCompile(`^[A-Z_]{3,20}$`)
	symbolPartRe           = regexp.MustCompile(`^[A-Z]{1,19}$`)
	clientOrderIDRe        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,36}$`)
	ethAddressRe           = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	trxAddressRe           = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)
	btcBase58AddressRe     = regexp.MustCompile(`^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$`)
	btcBech32AddressLower  = regexp.MustCompile(`^bc1[0-9ac-hj-np-z]{11,71}$`)
	btcBech32AddressUpper  = regexp.MustCompile(`^BC1[0-9AC-HJ-NP-Z]{11,71}$`)
	emailSimpleStrictRe    = regexp.MustCompile(`^[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
)

// Symbol 校验交易对格式（如 BTC_USDT）
func Symbol(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return commonerrors.New(commonerrors.CodeInvalidParam, "symbol is required")
	}
	if len(s) < 3 || len(s) > 20 || !symbolAllowedRe.MatchString(s) {
		return commonerrors.Newf(commonerrors.CodeInvalidParam, "invalid symbol: %q (expected BASE_QUOTE, uppercase letters and underscore, length 3-20)", s)
	}
	parts := strings.Split(s, "_")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return commonerrors.Newf(commonerrors.CodeInvalidParam, "invalid symbol: %q (expected BASE_QUOTE)", s)
	}
	if !symbolPartRe.MatchString(parts[0]) || !symbolPartRe.MatchString(parts[1]) {
		return commonerrors.Newf(commonerrors.CodeInvalidParam, "invalid symbol: %q (BASE/QUOTE must be uppercase letters)", s)
	}
	return nil
}

// Side 校验订单方向
func Side(s string) error {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "BUY", "SELL":
		return nil
	default:
		return commonerrors.Newf(commonerrors.CodeInvalidSide, "invalid side: %q (expected BUY or SELL)", s)
	}
}

// OrderType 校验订单类型
func OrderType(s string) error {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "LIMIT", "MARKET":
		return nil
	default:
		return commonerrors.Newf(commonerrors.CodeInvalidOrderType, "invalid order type: %q (expected LIMIT or MARKET)", s)
	}
}

// TimeInForce 校验有效期类型
func TimeInForce(s string) error {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "GTC", "IOC", "FOK", "POST_ONLY":
		return nil
	default:
		return commonerrors.Newf(commonerrors.CodeInvalidTimeInForce, "invalid timeInForce: %q (expected GTC/IOC/FOK/POST_ONLY)", s)
	}
}

// Price 校验价格（必须 > 0，精度校验）
//
// 约定：price 为按 10^defaultPrecision 缩放后的整数（如 defaultPrecision=8）。
// precision 表示允许的小数位数（0..defaultPrecision）。
func Price(price int64, precision int) error {
	if price <= 0 {
		return commonerrors.Newf(commonerrors.CodeInvalidPrice, "invalid price: %d (must be > 0)", price)
	}
	if precision < 0 || precision > defaultPrecision {
		return commonerrors.Newf(commonerrors.CodeInvalidPrice, "invalid price precision: %d (expected 0..%d)", precision, defaultPrecision)
	}
	if precision == defaultPrecision {
		return nil
	}
	// precision=2 => 允许 2 位小数；在 defaultPrecision=8 的缩放下，必须能被 10^(8-2) 整除。
	factor := pow10i(defaultPrecision - precision)
	if factor <= 0 {
		return commonerrors.Newf(commonerrors.CodeInvalidPrice, "invalid price precision: %d", precision)
	}
	if price%factor != 0 {
		return commonerrors.Newf(commonerrors.CodeInvalidPrice, "invalid price: %d (precision=%d, expected multiple of %d)", price, precision, factor)
	}
	return nil
}

func pow10i(n int) int64 {
	if n < 0 {
		return 0
	}
	factor := int64(1)
	for i := 0; i < n; i++ {
		factor *= 10
	}
	return factor
}

// Quantity 校验数量（必须 > 0，范围校验）
func Quantity(qty int64, min, max int64) error {
	if qty <= 0 {
		return commonerrors.Newf(commonerrors.CodeInvalidQuantity, "invalid quantity: %d (must be > 0)", qty)
	}
	if min > 0 && qty < min {
		return commonerrors.Newf(commonerrors.CodeInvalidQuantity, "invalid quantity: %d (min=%d)", qty, min)
	}
	if max > 0 && qty > max {
		return commonerrors.Newf(commonerrors.CodeInvalidQuantity, "invalid quantity: %d (max=%d)", qty, max)
	}
	if min > 0 && max > 0 && min > max {
		return commonerrors.Newf(commonerrors.CodeInvalidQuantity, "invalid quantity range: min=%d > max=%d", min, max)
	}
	return nil
}

// ClientOrderID 校验客户端订单ID（长度、字符）
func ClientOrderID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return commonerrors.New(commonerrors.CodeInvalidParam, "clientOrderID is required")
	}
	if !clientOrderIDRe.MatchString(id) {
		return commonerrors.Newf(commonerrors.CodeInvalidParam, "invalid clientOrderID: %q (expected 1-36 chars, [A-Za-z0-9_-])", id)
	}
	return nil
}

// Address 校验区块链地址格式
func Address(addr, network string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return commonerrors.New(commonerrors.CodeInvalidAddress, "address is required")
	}
	switch strings.ToUpper(strings.TrimSpace(network)) {
	case "ETH":
		if !ethAddressRe.MatchString(addr) {
			return commonerrors.Newf(commonerrors.CodeInvalidAddress, "invalid ETH address: %q", addr)
		}
		return nil
	case "TRX":
		if !trxAddressRe.MatchString(addr) {
			return commonerrors.Newf(commonerrors.CodeInvalidAddress, "invalid TRX address: %q", addr)
		}
		return nil
	case "BTC":
		if btcBase58AddressRe.MatchString(addr) || btcBech32AddressLower.MatchString(addr) || btcBech32AddressUpper.MatchString(addr) {
			return nil
		}
		return commonerrors.Newf(commonerrors.CodeInvalidAddress, "invalid BTC address: %q", addr)
	default:
		return commonerrors.Newf(commonerrors.CodeInvalidAddress, "invalid network: %q (expected ETH/TRX/BTC)", network)
	}
}

// Email 校验邮箱格式
func Email(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return commonerrors.New(commonerrors.CodeInvalidParam, "email is required")
	}
	if len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
		return commonerrors.Newf(commonerrors.CodeInvalidParam, "invalid email: %q", email)
	}
	if _, err := mail.ParseAddress(email); err != nil || !emailSimpleStrictRe.MatchString(email) {
		return commonerrors.Newf(commonerrors.CodeInvalidParam, "invalid email: %q", email)
	}
	return nil
}

type ValidationError struct {
	Field   string
	Code    commonerrors.Code
	Message string
}

type Validator struct {
	errors []ValidationError
}

func New() *Validator {
	return &Validator{}
}

func (v *Validator) add(field string, err error) *Validator {
	if err == nil {
		return v
	}
	var ce *commonerrors.Error
	if ok := stderrors.As(err, &ce); ok && ce != nil {
		v.errors = append(v.errors, ValidationError{Field: field, Code: ce.Code, Message: ce.Message})
		return v
	}
	v.errors = append(v.errors, ValidationError{Field: field, Code: commonerrors.CodeInvalidParam, Message: err.Error()})
	return v
}

func (v *Validator) Symbol(field, value string) *Validator {
	return v.add(field, Symbol(value))
}

func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		return v.add(field, commonerrors.Newf(commonerrors.CodeInvalidParam, "%s is required", field))
	}
	return v
}

func (v *Validator) Errors() []ValidationError {
	out := make([]ValidationError, len(v.errors))
	copy(out, v.errors)
	return out
}

func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

func (v *Validator) FirstError() *ValidationError {
	if len(v.errors) == 0 {
		return nil
	}
	return &v.errors[0]
}
