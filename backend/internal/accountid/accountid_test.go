package accountid

import (
	"strings"
	"testing"
)

// TestGenerateFormat 验证生成结果符合对外 ID 约定，且随机部分只使用安全字符集。
func TestGenerateFormat(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		id, err := Generate()
		if err != nil {
			t.Fatalf("Generate 失败: %v", err)
		}
		if !Valid(id) {
			t.Fatalf("生成的 ID 不合法: %q", id)
		}
		if len(id) != 1+BodyLength {
			t.Fatalf("ID 长度应为 %d，实际 %d: %q", 1+BodyLength, len(id), id)
		}
		// 30^10 空间下连续生成 64 个出现碰撞的概率可忽略；若发生说明随机源异常。
		if _, ok := seen[id]; ok {
			t.Fatalf("短时间内生成重复 ID: %q", id)
		}
		seen[id] = struct{}{}
	}
}

// TestValid 覆盖合法值、长度错误、前缀错误与易混淆字符。
func TestValid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "合法", in: "U23456789AB", want: true},
		{name: "空串", in: "", want: false},
		{name: "缺少前缀", in: "A23456789AB", want: false},
		{name: "长度过短", in: "U23456789A", want: false},
		{name: "长度过长", in: "U23456789ABC", want: false},
		{name: "包含易混淆 I", in: "U23456789AI", want: false},
		{name: "包含小写", in: "U23456789ab", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.in); got != tt.want {
				t.Fatalf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestAlphabetNoAmbiguous 保证字符集里没有 I / L / O / U，避免口头传播误读。
func TestAlphabetNoAmbiguous(t *testing.T) {
	for _, r := range "ILOU" {
		if strings.ContainsRune(alphabet, r) {
			t.Fatalf("字符集不应包含易混淆字符 %q", r)
		}
	}
}
