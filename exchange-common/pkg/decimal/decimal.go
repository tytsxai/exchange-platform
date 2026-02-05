// Package decimal 精度计算工具
package decimal

import (
	"fmt"
	"math/big"
	"strings"
)

// Decimal 高精度十进制数
type Decimal struct {
	value *big.Int // 内部值（最小单位整数）
	scale int      // 小数位数
}

// Zero 零值
var Zero = &Decimal{value: big.NewInt(0), scale: 0}

// New 从字符串创建
func New(s string) (*Decimal, error) {
	if s == "" {
		return Zero, nil
	}

	// 处理负号
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}

	// 分离整数和小数部分
	parts := strings.Split(s, ".")
	intPart := parts[0]
	fracPart := ""
	if len(parts) > 1 {
		fracPart = parts[1]
	}

	// 去除前导零
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}

	// 合并为整数
	combined := intPart + fracPart
	value := new(big.Int)
	_, ok := value.SetString(combined, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal: %s", s)
	}

	if negative {
		value.Neg(value)
	}

	return &Decimal{
		value: value,
		scale: len(fracPart),
	}, nil
}

// MustNew 从字符串创建，panic on error
func MustNew(s string) *Decimal {
	d, err := New(s)
	if err != nil {
		panic(err)
	}
	return d
}

// FromInt 从整数创建
func FromInt(v int64) *Decimal {
	return &Decimal{
		value: big.NewInt(v),
		scale: 0,
	}
}

// FromIntWithScale 从最小单位整数创建
func FromIntWithScale(v int64, scale int) *Decimal {
	return &Decimal{
		value: big.NewInt(v),
		scale: scale,
	}
}

// String 转字符串
func (d *Decimal) String() string {
	if d == nil || d.value == nil {
		return "0"
	}

	s := d.value.String()
	negative := strings.HasPrefix(s, "-")
	if negative {
		s = s[1:]
	}

	if d.scale == 0 {
		if negative {
			return "-" + s
		}
		return s
	}

	// 补零
	for len(s) <= d.scale {
		s = "0" + s
	}

	// 插入小数点
	pos := len(s) - d.scale
	result := s[:pos] + "." + s[pos:]

	// 去除尾部零
	result = strings.TrimRight(result, "0")
	result = strings.TrimRight(result, ".")

	if negative {
		return "-" + result
	}
	return result
}

// Cmp 比较：-1 (d < other), 0 (d == other), 1 (d > other)
func (d *Decimal) Cmp(other *Decimal) int {
	d1, d2 := d.alignScale(other)
	return d1.value.Cmp(d2.value)
}

// Add 加法
func (d *Decimal) Add(other *Decimal) *Decimal {
	d1, d2 := d.alignScale(other)
	result := new(big.Int).Add(d1.value, d2.value)
	return &Decimal{value: result, scale: d1.scale}
}

// Sub 减法
func (d *Decimal) Sub(other *Decimal) *Decimal {
	d1, d2 := d.alignScale(other)
	result := new(big.Int).Sub(d1.value, d2.value)
	return &Decimal{value: result, scale: d1.scale}
}

// Mul 乘法
func (d *Decimal) Mul(other *Decimal) *Decimal {
	result := new(big.Int).Mul(d.value, other.value)
	return &Decimal{value: result, scale: d.scale + other.scale}
}

// Div 除法（指定精度，向下截断）
func (d *Decimal) Div(other *Decimal, scale int) *Decimal {
	if other.value.Sign() == 0 {
		// 避免生产环境 panic：返回 0（指定精度），由调用方自行处理业务含义
		return &Decimal{value: big.NewInt(0), scale: scale}
	}

	// 扩展被除数精度
	targetScale := scale + other.scale
	scaleDiff := targetScale - d.scale

	dividend := new(big.Int).Set(d.value)
	if scaleDiff > 0 {
		multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scaleDiff)), nil)
		dividend.Mul(dividend, multiplier)
	} else if scaleDiff < 0 {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scaleDiff)), nil)
		dividend.Div(dividend, divisor)
	}

	result := new(big.Int).Div(dividend, other.value)
	return &Decimal{value: result, scale: scale}
}

// Neg 取负
func (d *Decimal) Neg() *Decimal {
	result := new(big.Int).Neg(d.value)
	return &Decimal{value: result, scale: d.scale}
}

// Abs 绝对值
func (d *Decimal) Abs() *Decimal {
	result := new(big.Int).Abs(d.value)
	return &Decimal{value: result, scale: d.scale}
}

// IsZero 是否为零
func (d *Decimal) IsZero() bool {
	return d.value.Sign() == 0
}

// IsPositive 是否为正
func (d *Decimal) IsPositive() bool {
	return d.value.Sign() > 0
}

// IsNegative 是否为负
func (d *Decimal) IsNegative() bool {
	return d.value.Sign() < 0
}

// Truncate 截断到指定精度（向下）
func (d *Decimal) Truncate(scale int) *Decimal {
	if scale >= d.scale {
		return d
	}

	diff := d.scale - scale
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(diff)), nil)
	result := new(big.Int).Div(d.value, divisor)
	return &Decimal{value: result, scale: scale}
}

// ToInt 转为最小单位整数
func (d *Decimal) ToInt(scale int) int64 {
	aligned := d.setScale(scale)
	return aligned.value.Int64()
}

// alignScale 对齐精度
func (d *Decimal) alignScale(other *Decimal) (*Decimal, *Decimal) {
	if d.scale == other.scale {
		return d, other
	}
	if d.scale > other.scale {
		return d, other.setScale(d.scale)
	}
	return d.setScale(other.scale), other
}

// setScale 设置精度
func (d *Decimal) setScale(scale int) *Decimal {
	if scale == d.scale {
		return d
	}

	diff := scale - d.scale
	result := new(big.Int).Set(d.value)

	if diff > 0 {
		multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(diff)), nil)
		result.Mul(result, multiplier)
	} else {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-diff)), nil)
		result.Div(result, divisor)
	}

	return &Decimal{value: result, scale: scale}
}

// Min 返回较小值
func Min(a, b *Decimal) *Decimal {
	if a.Cmp(b) <= 0 {
		return a
	}
	return b
}

// Max 返回较大值
func Max(a, b *Decimal) *Decimal {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}
