// Package jsonb 提供 JSONB 字段的解析辅助。
// GORM 下 JSONB 字段以字符串存储，service 层通过本工具完成序列化/反序列化。
package jsonb

import "encoding/json"

// ParseStringArray 将 JSONB 字符串解析为字符串数组。
// 非法或为空时返回 nil，不报错（数据库约束保证写入时合法）。
func ParseStringArray(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// MarshalStringArray 将字符串数组序列化为 JSONB 字符串（默认 []）。
func MarshalStringArray(items []string) string {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// ParseMap 将 JSONB 字符串解析为 map[string]any。
func ParseMap(s string) map[string]any {
	if s == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}

// MarshalMap 将 map 序列化为 JSONB 字符串（默认 {}）。
func MarshalMap(m map[string]any) string {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ParseAny 将 JSONB 字符串解析为任意值（用于泛型内容字段）。
func ParseAny(s string) any {
	if s == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

// MarshalRaw 序列化任意值为 JSONB 字符串；失败时返回传入的默认回退值。
func MarshalRaw(v any, fallback string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(b)
}