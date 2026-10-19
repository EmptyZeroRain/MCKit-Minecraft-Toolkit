// Package coordinate 提供主世界/下界坐标换算逻辑（需求 §5.3）。
package coordinate

import (
	"errors"
	"math"
)

// ErrInvalidValue 输入不是有效数值（空、NaN、无穷）。
var ErrInvalidValue = errors.New("坐标必须是有效数值")

// Coordinate 三维坐标。
type Coordinate struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Result 坐标换算结果。
// Y 坐标原样显示不参与 8 倍换算；Rounding 为可选取整建议值。
type Result struct {
	Original  Coordinate `json:"original"`
	Converted Coordinate `json:"converted"`
	Rounding  Coordinate `json:"rounding"`
}

// Factor 描述换算方向。主世界→下界 X/Z 除以 8（倍率 0.125），反之乘以 8。
type Factor struct {
	Multiplier float64
}

var (
	// ToNether 主世界 → 下界。
	ToNether = Factor{Multiplier: 0.125}
	// ToOverworld 下界 → 主世界。
	ToOverworld = Factor{Multiplier: 8}
)

// Convert 执行换算。任一坐标分量必须是有理数，NaN / 无穷 / 空均被拒绝。
func Convert(x, y, z float64, factor Factor) (Result, error) {
	if !isValid(x) || !isValid(y) || !isValid(z) {
		return Result{}, ErrInvalidValue
	}

	converted := Coordinate{
		X: x * factor.Multiplier,
		Y: y,
		Z: z * factor.Multiplier,
	}
	rounding := Coordinate{
		X: math.Round(converted.X),
		Y: y,
		Z: math.Round(converted.Z),
	}
	return Result{
		Original:  Coordinate{X: x, Y: y, Z: z},
		Converted: converted,
		Rounding:  rounding,
	}, nil
}

func isValid(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}