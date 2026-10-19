// Package blockids 解析《物品id.md》方块/物品对照表（移植自 CreeperCode）。
// 文件格式：前半为方块表（中文名 \t 命名空间ID \t 物品形式），
// 第 1277 行附近为物品子表（中文名 \t 命名空间ID）。
package blockids

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Entry 对照表条目。
type Entry struct {
	// Name 中文名，可能含 [新增：JE x.x] 标记（保留原文）。
	Name string `json:"name"`
	// ID 命名空间 ID。
	ID string `json:"id"`
	// Form 物品形式原文，纯物品条目为空。
	Form string `json:"form"`
	// Kind 分类：same/different/variant/overlay/special/none/item/other。
	Kind string `json:"kind"`
	// RefID 物品形式括号中的关联 ID（如 ID不同（beetroot）的 beetroot）。
	RefID string `json:"refId,omitempty"`
}

// Kind 各分类常量。
const (
	KindSame      = "same"
	KindDifferent = "different"
	KindVariant   = "variant"
	KindOverlay   = "overlay"
	KindSpecial   = "special"
	KindNone      = "none"
	KindItem      = "item"
	KindOther     = "other"
)

// nsidRe 命名空间 ID 的合法形态，用于区分数据行与说明文字/表头。
var nsidRe = regexp.MustCompile(`^[a-z0-9_.-]+$`)

// refRe 提取物品形式中文括号内的关联 ID。
var refRe = regexp.MustCompile(`（([a-z0-9_.-]+)）`)

// Parse 解析 md 文本，忽略说明与表头，未知格式行跳过。
func Parse(data []byte) []Entry {
	entries := []Entry{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		parts := strings.Split(line, "\t")
		// 去掉每段首尾空白，空表头/说明行自然不满足 nsid 校验。
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) == 3 && nsidRe.MatchString(parts[1]) {
			e := Entry{
				Name: parts[0],
				ID:   parts[1],
				Form: parts[2],
				Kind: classify(parts[2]),
			}
			// 括号内关联 ID 仅在存在时填充，FindStringSubmatch 无匹配返回 nil。
			if m := refRe.FindStringSubmatch(parts[2]); m != nil {
				e.RefID = m[1]
			}
			entries = append(entries, e)
			continue
		}
		// 物品子表：两段（中文名、ID），物品形式即物品本身。
		if len(parts) == 2 && nsidRe.MatchString(parts[1]) {
			entries = append(entries, Entry{
				Name: parts[0],
				ID:   parts[1],
				Kind: KindItem,
			})
		}
	}
	return entries
}

// classify 依据物品形式文本判定分类。
func classify(form string) string {
	switch {
	case strings.Contains(form, "ID相同"):
		return KindSame
	case strings.HasPrefix(form, "ID不同"):
		return KindDifferent
	case strings.Contains(form, "没有物品形式"):
		return KindNone
	case strings.Contains(form, "等同于空物品槽"):
		return KindSpecial
	case strings.Contains(form, "叠加型方块"):
		return KindOverlay
	case strings.Contains(form, "变种（"):
		return KindVariant
	default:
		return KindOther
	}
}

// Load 从文件读取并解析，文件不存在时返回错误由调用方决定降级策略。
func Load(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取物品对照表失败: %w", err)
	}
	return Parse(data), nil
}

// itemJSON 是解析产物的落盘结构，供 cache/item.json 使用。
type itemJSON struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Source      string    `json:"source"`
	Total       int       `json:"total"`
	Entries     []Entry   `json:"entries"`
}

// SaveJSON 将解析结果写入 path（如 cache/item.json）。
// md 为唯一权威源，每次启动都会重新解析并覆盖此文件。
func SaveJSON(entries []Entry, path, source string) error {
	data, err := json.MarshalIndent(itemJSON{
		GeneratedAt: time.Now(),
		Source:      source,
		Total:       len(entries),
		Entries:     entries,
	}, "", "  ")
	if err != nil {
		return err
	}
	// 确保父目录存在（缓存目录可能尚未创建）。
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadJSON 从落盘的 item.json 恢复条目，用于 md 缺失时的降级。
func LoadJSON(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 item.json 失败: %w", err)
	}
	var j itemJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("解析 item.json 失败: %w", err)
	}
	return j.Entries, nil
}
