package nbt

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// 构造 NBT 字节的工具函数（测试专用）。

func be16(n uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, n)
	return b
}

func be32(n int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}

func be64(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}

func str(s string) []byte {
	return append(be16(uint16(len(s))), []byte(s)...)
}

// namedTag 拼接 [type][name][payload]。
func namedTag(tagType byte, name string, payload []byte) []byte {
	out := []byte{tagType}
	out = append(out, str(name)...)
	out = append(out, payload...)
	return out
}

// sampleCompound 构造一个包含多种标签类型的顶层 Compound。
func sampleCompound() []byte {
	var buf bytes.Buffer
	buf.WriteByte(byte(TagCompound))
	buf.Write(str("")) // 顶层名称通常为空

	buf.Write(namedTag(byte(TagInt), "DataVersion", be32(3465)))
	buf.Write(namedTag(byte(TagString), "Status", str("full")))
	buf.Write(namedTag(byte(TagInt), "xPos", be32(10)))
	buf.Write(namedTag(byte(TagInt), "zPos", be32(-3)))
	buf.Write(namedTag(byte(TagByte), "isLightOn", []byte{1}))
	buf.Write(namedTag(byte(TagLong), "InhabitedTime", be64(123456789)))

	// 整数列表 [1,2,3]
	var list bytes.Buffer
	list.WriteByte(byte(TagInt)) // 元素类型
	list.Write(be32(3))          // 长度
	list.Write(be32(1))
	list.Write(be32(2))
	list.Write(be32(3))
	buf.Write(namedTag(byte(TagList), "nums", list.Bytes()))

	// 嵌套 Compound
	var sub bytes.Buffer
	sub.Write(namedTag(byte(TagString), "name", str("overworld")))
	sub.WriteByte(byte(TagEnd))
	buf.Write(namedTag(byte(TagCompound), "sub", sub.Bytes()))

	buf.WriteByte(byte(TagEnd))
	return buf.Bytes()
}

func TestParseCompound(t *testing.T) {
	top, err := Parse(sampleCompound())
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if dv, ok := top["DataVersion"].(int32); !ok || dv != 3465 {
		t.Fatalf("DataVersion 应为 int32(3465): %#v", top["DataVersion"])
	}
	if st, ok := top["Status"].(string); !ok || st != "full" {
		t.Fatalf("Status 应为 \"full\": %#v", top["Status"])
	}
	if z, ok := top["zPos"].(int32); !ok || z != -3 {
		t.Fatalf("zPos 应为 int32(-3): %#v", top["zPos"])
	}
	if b, ok := top["isLightOn"].(int8); !ok || b != 1 {
		t.Fatalf("isLightOn 应为 int8(1): %#v", top["isLightOn"])
	}
	if l, ok := top["InhabitedTime"].(int64); !ok || l != 123456789 {
		t.Fatalf("InhabitedTime 应为 int64: %#v", top["InhabitedTime"])
	}

	nums, ok := top["nums"].([]any)
	if !ok || len(nums) != 3 {
		t.Fatalf("nums 应为长度 3 的列表: %#v", top["nums"])
	}
	if nums[0].(int32) != 1 || nums[2].(int32) != 3 {
		t.Fatalf("nums 内容不符: %#v", nums)
	}

	sub, ok := top["sub"].(map[string]any)
	if !ok || sub["name"] != "overworld" {
		t.Fatalf("嵌套 Compound 解析失败: %#v", top["sub"])
	}
}

func TestParseTopLevelMustBeCompound(t *testing.T) {
	// 顶层为 Int 标签，应报错。
	bad := namedTag(byte(TagInt), "", be32(1))
	if _, err := Parse(bad); err == nil {
		t.Fatal("顶层非 Compound 应报错")
	}
}

func TestParseTruncated(t *testing.T) {
	data := sampleCompound()
	if _, err := Parse(data[:len(data)/2]); err == nil {
		t.Fatal("截断数据应报错")
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Fatal("空数据应报错")
	}
}

func TestParseArraysAndLists(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(byte(TagCompound))
	buf.Write(str(""))

	// IntArray 长度 2
	var ia bytes.Buffer
	ia.Write(be32(2))
	ia.Write(be32(7))
	ia.Write(be32(8))
	buf.Write(namedTag(byte(TagIntArray), "ia", ia.Bytes()))

	// LongArray 长度 1
	var la bytes.Buffer
	la.Write(be32(1))
	la.Write(be64(99))
	buf.Write(namedTag(byte(TagLongArray), "la", la.Bytes()))

	// ByteArray 长度 3
	var ba bytes.Buffer
	ba.Write(be32(3))
	ba.Write([]byte{1, 2, 3})
	buf.Write(namedTag(byte(TagByteArray), "ba", ba.Bytes()))

	buf.WriteByte(byte(TagEnd))

	top, err := Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if top["ia"] != "<int_array:2>" {
		t.Fatalf("IntArray 占位不符: %#v", top["ia"])
	}
	if top["la"] != "<long_array:1>" {
		t.Fatalf("LongArray 占位不符: %#v", top["la"])
	}
	if top["ba"] != "<byte_array:3>" {
		t.Fatalf("ByteArray 占位不符: %#v", top["ba"])
	}
}
