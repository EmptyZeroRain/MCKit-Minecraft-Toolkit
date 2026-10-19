// Package types 提供 GORM JSONB 字段的自定义类型，
// 使存储为 jsonb 的数组/对象在读写和 JSON 序列化时保持原生结构。
package types

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONSlice 对应 jsonb 数组字段。
type JSONSlice []string

// Value 实现 driver.Valuer。
func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		j = JSONSlice{}
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (j *JSONSlice) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("无法扫描类型 %T 到 JSONSlice", value)
	}
	if raw == "" {
		*j = nil
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return err
	}
	*j = out
	return nil
}

// MarshalJSON 直接输出原生数组。
func (j JSONSlice) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(j))
}

// JSONMap 对应 jsonb 对象字段。
type JSONMap map[string]any

// Value 实现 driver.Valuer。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		m = JSONMap{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (m *JSONMap) Scan(value any) error {
	if value == nil {
		*m = nil
		return nil
	}
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("无法扫描类型 %T 到 JSONMap", value)
	}
	if raw == "" {
		*m = nil
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return err
	}
	*m = out
	return nil
}

// MarshalJSON 直接输出原生对象。
func (m JSONMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]any(m))
}

// JSONB 通用 JSONB 字段类型：读取/写入任意 JSON 结构。
type JSONB json.RawMessage

// Value 实现 driver.Valuer。
func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	return string(j), nil
}

// Scan 实现 sql.Scanner，保留原始 JSON 解析。
func (j *JSONB) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("无法扫描类型 %T 到 JSONB", value)
	}
	if len(raw) == 0 {
		*j = nil
		return nil
	}
	// 校验合法性但保留原文。
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*j = append((*j)[:0], raw...)
	return nil
}

// MarshalJSON 输出原始内容。
func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	return j, nil
}

// Unmarshal 解析 JSONB 为任意目标结构。
func (j JSONB) Unmarshal(dst any) error {
	return json.Unmarshal(j, dst)
}

// JSONBArray 将原始 JSON 归一化为 JSONB 数组字段值：
// 空或 null 时回退为 "[]"，保证数组列始终为合法 JSON。
func JSONBArray(raw []byte) JSONB {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || string(t) == "null" {
		return JSONB("[]")
	}
	return JSONB(t)
}