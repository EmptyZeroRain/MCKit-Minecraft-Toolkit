package coordinate

import (
	"math"
	"testing"
)

func TestToNether(t *testing.T) {
	// 需求 §5.3 验收示例：主世界 (800, 70, -240) 对应下界理论坐标 (100, 70, -30)。
	r, err := Convert(800, 70, -240, ToNether)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if r.Converted.X != 100 || r.Converted.Z != -30 {
		t.Fatalf("期望 (100,70,-30)，实际 (%v,%v,%v)", r.Converted.X, r.Converted.Y, r.Converted.Z)
	}
	// Y 坐标原样
	if r.Converted.Y != 70 {
		t.Fatalf("Y 应原样 70，实际 %v", r.Converted.Y)
	}
}

func TestToOverworld(t *testing.T) {
	r, err := Convert(100, 64, -30, ToOverworld)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if r.Converted.X != 800 || r.Converted.Z != -240 {
		t.Fatalf("期望 (800,64,-240)，实际 (%v,%v,%v)", r.Converted.X, r.Converted.Y, r.Converted.Z)
	}
}

func TestNegativeAndDecimal(t *testing.T) {
	// 负数与小数输入
	r, err := Convert(-320.5, 0, 21.25, ToNether)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	// -320.5 * 0.125 = -40.0625
	if math.Abs(r.Converted.X+40.0625) > 1e-9 {
		t.Fatalf("X 换算错误: %v", r.Converted.X)
	}
	if math.Abs(r.Converted.Z-2.65625) > 1e-9 {
		t.Fatalf("Z 换算错误: %v", r.Converted.Z)
	}
}

func TestInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		x    float64
		y    float64
		z    float64
	}{
		{"NaN", math.NaN(), 0, 0},
		{"Inf", math.Inf(1), 0, 0},
		{"NegInf", 0, math.Inf(-1), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Convert(tc.x, tc.y, tc.z, ToNether); err == nil {
				t.Fatal("非法值应报错")
			}
		})
	}
}

// Go 默认零值即可表达"空"，此处验证 0,0,0 是合法换算。
func TestZeroIsValid(t *testing.T) {
	r, err := Convert(0, 0, 0, ToNether)
	if err != nil {
		t.Fatalf("(0,0,0) 应合法: %v", err)
	}
	if r.Converted.X != 0 || r.Converted.Z != 0 {
		t.Fatalf("零值换算应仍为零")
	}
}

func TestRoundingDoesNotOverridePrecision(t *testing.T) {
	r, err := Convert(63, 70, -17, ToNether)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	// 精确值 63/8=7.875，取整 8
	if r.Converted.X != 7.875 {
		t.Fatalf("精确值应为 7.875，实际 %v", r.Converted.X)
	}
	if r.Rounding.X != 8 {
		t.Fatalf("取整建议应为 8，实际 %v", r.Rounding.X)
	}
}