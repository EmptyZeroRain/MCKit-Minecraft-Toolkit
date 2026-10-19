// Package nbt 实现 Minecraft NBT（Named Binary Tag）二进制格式解析。
// 格式参考：Minecraft Wiki "NBT format"。
// 用于读取区块数据中的顶层字段（如 DataVersion、xPos、zPos、Status 等）。
package nbt

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// TagType NBT 标签类型。
type TagType byte

// 标签类型常量。
const (
	TagEnd       TagType = 0
	TagByte      TagType = 1
	TagShort     TagType = 2
	TagInt       TagType = 3
	TagLong      TagType = 4
	TagFloat     TagType = 5
	TagDouble    TagType = 6
	TagByteArray TagType = 7
	TagString    TagType = 8
	TagList      TagType = 9
	TagCompound  TagType = 10
	TagIntArray  TagType = 11
	TagLongArray TagType = 12
)

// maxDepth 限制递归深度，防止恶意数据导致栈溢出。
const maxDepth = 64

// Parse 解析 NBT 数据，返回顶层 compound 的 map。
// 顶层必须是 TAG_Compound。
func Parse(data []byte) (map[string]any, error) {
	r := &reader{data: data}
	tagType, err := r.readByte()
	if err != nil {
		return nil, err
	}
	if TagType(tagType) != TagCompound {
		return nil, fmt.Errorf("NBT 顶层必须是 Compound，实际类型 %d", tagType)
	}
	// 顶层名称（通常为空）
	if _, err := r.readString(); err != nil {
		return nil, err
	}
	v, err := r.readCompound(0)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// MaxDecompressedSize 单次解压输出上限（16 MiB），防止压缩炸弹造成内存放大。
const MaxDecompressedSize = 16 << 20

// DecompressGzip 解压 gzip 数据，输出超过 MaxDecompressedSize 时报错。
// 用于 level.dat 等以 gzip 压缩的 NBT 文件。
func DecompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, MaxDecompressedSize+1))
	if err != nil {
		return nil, err
	}
	if len(out) > MaxDecompressedSize {
		return nil, fmt.Errorf("解压输出超过上限 %d 字节", MaxDecompressedSize)
	}
	return out, nil
}

// ParseFile 解析可能是 gzip 压缩、也可能是未压缩的 NBT 数据。
// level.dat 通常为 gzip；先尝试解压，失败则按未压缩解析。
// 解压成功但 NBT 结构非法时返回解析错误，不再回退（回退也无意义）。
func ParseFile(data []byte) (map[string]any, error) {
	if raw, err := DecompressGzip(data); err == nil {
		return Parse(raw)
	}
	return Parse(data)
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("NBT 数据意外结束")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *reader) readN(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.data) {
		return nil, fmt.Errorf("NBT 读取 %d 字节越界", n)
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *reader) readString() (string, error) {
	lenBytes, err := r.readN(2)
	if err != nil {
		return "", err
	}
	n := int(binary.BigEndian.Uint16(lenBytes))
	if n == 0 {
		return "", nil
	}
	b, err := r.readN(n)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *reader) readCompound(depth int) (map[string]any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("NBT 嵌套过深")
	}
	out := map[string]any{}
	for {
		t, err := r.readByte()
		if err != nil {
			return nil, err
		}
		tagType := TagType(t)
		if tagType == TagEnd {
			return out, nil
		}
		name, err := r.readString()
		if err != nil {
			return nil, err
		}
		val, err := r.readPayload(tagType, depth)
		if err != nil {
			return nil, err
		}
		out[name] = val
	}
}

func (r *reader) readPayload(t TagType, depth int) (any, error) {
	switch t {
	case TagByte:
		b, err := r.readByte()
		return int8(b), err
	case TagShort:
		bs, err := r.readN(2)
		if err != nil {
			return nil, err
		}
		return int16(binary.BigEndian.Uint16(bs)), nil
	case TagInt:
		bs, err := r.readN(4)
		if err != nil {
			return nil, err
		}
		return int32(binary.BigEndian.Uint32(bs)), nil
	case TagLong:
		bs, err := r.readN(8)
		if err != nil {
			return nil, err
		}
		return int64(binary.BigEndian.Uint64(bs)), nil
	case TagFloat:
		bs, err := r.readN(4)
		if err != nil {
			return nil, err
		}
		return math.Float32frombits(binary.BigEndian.Uint32(bs)), nil
	case TagDouble:
		bs, err := r.readN(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(bs)), nil
	case TagString:
		return r.readString()
	case TagByteArray:
		bs, err := r.readN(4)
		if err != nil {
			return nil, err
		}
		n := int(int32(binary.BigEndian.Uint32(bs)))
		_, err = r.readN(n)
		return fmt.Sprintf("<byte_array:%d>", n), err
	case TagIntArray:
		bs, err := r.readN(4)
		if err != nil {
			return nil, err
		}
		n := int(int32(binary.BigEndian.Uint32(bs)))
		_, err = r.readN(n * 4)
		return fmt.Sprintf("<int_array:%d>", n), err
	case TagLongArray:
		bs, err := r.readN(4)
		if err != nil {
			return nil, err
		}
		n := int(int32(binary.BigEndian.Uint32(bs)))
		_, err = r.readN(n * 8)
		return fmt.Sprintf("<long_array:%d>", n), err
	case TagList:
		return r.readList(depth)
	case TagCompound:
		return r.readCompound(depth + 1)
	default:
		return nil, fmt.Errorf("未知 NBT 标签类型: %d", t)
	}
}

func (r *reader) readList(depth int) ([]any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("NBT 嵌套过深")
	}
	typeByte, err := r.readByte()
	if err != nil {
		return nil, err
	}
	elemType := TagType(typeByte)
	lenBytes, err := r.readN(4)
	if err != nil {
		return nil, err
	}
	n := int(int32(binary.BigEndian.Uint32(lenBytes)))
	if n < 0 || n > 1<<24 {
		return nil, fmt.Errorf("NBT 列表长度非法: %d", n)
	}
	out := make([]any, 0, min(n, 1024))
	for i := 0; i < n; i++ {
		v, err := r.readPayload(elemType, depth+1)
		if err != nil {
			return nil, err
		}
		if len(out) < 1024 {
			out = append(out, v)
		}
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
