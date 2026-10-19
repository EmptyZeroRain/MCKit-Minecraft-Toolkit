package structure

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// GenerateSeeds 生成 n 个随机 Java 版世界种子。
// Java 种子为 64 位有符号整数，范围为 [-2^63, 2^63)。
func GenerateSeeds(n int) ([]int64, error) {
	if n <= 0 || n > 100 {
		n = 1
	}
	out := make([]int64, 0, n)
	buf := make([]byte, 8)
	for i := 0; i < n; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("生成随机种子失败: %w", err)
		}
		out = append(out, int64(binary.BigEndian.Uint64(buf)))
	}
	return out, nil
}

// ParseSeed 解析用户输入的种子。
// 支持纯数字（含负数）以及 Java 的 String.hashCode() 形式文本种子。
func ParseSeed(input string) (int64, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, fmt.Errorf("种子不能为空")
	}

	// 尝试纯数字（严格解析，避免 Sscanf 前缀匹配误判）
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}

	// 文本种子：复刻 Java String.hashCode()
	var h int32
	for _, c := range s {
		h = 31*h + int32(c)
	}
	return int64(h), nil
}

// FormatSeed 将种子格式化为字符串。
//
// 说明：Java 种子为 64 位有符号整数，范围 [-2^63, 2^63)，远超 JS Number 的
// 安全整数上限 2^53-1。直接以 JSON number 返回会被前端静默改写，因此对外
// 统一以字符串形式返回。
func FormatSeed(seed int64) string {
	return fmt.Sprintf("%d", seed)
}

// FormatSeeds 批量将种子格式化为字符串（保持顺序）。
func FormatSeeds(seeds []int64) []string {
	out := make([]string, 0, len(seeds))
	for _, s := range seeds {
		out = append(out, FormatSeed(s))
	}
	return out
}