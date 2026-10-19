package region

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"testing"
)

// buildNBT 构造一个仅含 DataVersion / Status / xPos / zPos 的最小 Compound。
func buildNBT() []byte {
	be16 := func(n uint16) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, n); return b }
	be32 := func(n int32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, uint32(n)); return b }
	str := func(s string) []byte { return append(be16(uint16(len(s))), []byte(s)...) }

	var buf bytes.Buffer
	buf.WriteByte(10) // TagCompound
	buf.Write(str(""))
	buf.WriteByte(3) // TagInt
	buf.Write(str("DataVersion"))
	buf.Write(be32(3465))
	buf.WriteByte(8) // TagString
	buf.Write(str("Status"))
	buf.Write(str("full"))
	buf.WriteByte(3) // TagInt
	buf.Write(str("xPos"))
	buf.Write(be32(10))
	buf.WriteByte(3) // TagInt
	buf.Write(str("zPos"))
	buf.Write(be32(-3))
	buf.WriteByte(0) // TagEnd
	return buf.Bytes()
}

// buildRegion 构造一个含单个区块（本地坐标 0,0）的 .mca 字节流。
func buildRegion(t *testing.T, compression int) []byte {
	t.Helper()

	nbt := buildNBT()
	var compressed bytes.Buffer
	switch compression {
	case CompressionZlib:
		w := zlib.NewWriter(&compressed)
		if _, err := w.Write(nbt); err != nil {
			t.Fatal(err)
		}
		w.Close()
	case CompressionGzip:
		w := gzip.NewWriter(&compressed)
		if _, err := w.Write(nbt); err != nil {
			t.Fatal(err)
		}
		w.Close()
	default:
		compressed.Write(nbt)
	}

	const sectorSize = 4096
	const dataSector = 2
	dataStart := dataSector * sectorSize

	// 头部 8192 字节：位置表 + 时间戳表。
	header := make([]byte, headerSize)
	// 位置项 i=0：offset(3) + sectors(1)
	header[0] = byte(dataSector >> 16)
	header[1] = byte(dataSector >> 8)
	header[2] = byte(dataSector)
	header[3] = 1 // 1 个扇区
	// 时间戳项 i=0
	binary.BigEndian.PutUint32(header[4096:], 1700000000)

	out := make([]byte, dataStart)
	copy(out, header)

	// 区块数据：u32 长度（含压缩类型字节）+ 压缩类型 + 压缩数据。
	length := uint32(1 + compressed.Len())
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], length)
	out = append(out, lenBuf[:]...)
	out = append(out, byte(compression))
	out = append(out, compressed.Bytes()...)
	return out
}

func TestParseRegionName(t *testing.T) {
	x, z, err := ParseRegionName("r.3.-7.mca")
	if err != nil {
		t.Fatalf("解析文件名失败: %v", err)
	}
	if x != 3 || z != -7 {
		t.Fatalf("坐标不符: (%d,%d)", x, z)
	}
	if _, _, err := ParseRegionName("bad-name.mca"); err == nil {
		t.Fatal("非法文件名应报错")
	}
}

func TestParseRegion(t *testing.T) {
	data := buildRegion(t, CompressionZlib)
	rf, err := Parse(data, 0, 0)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if len(rf.Chunks) != 1 {
		t.Fatalf("应有 1 个区块，实际 %d", len(rf.Chunks))
	}
	c := rf.Chunks[0]
	if c.LocalX != 0 || c.LocalZ != 0 {
		t.Fatalf("本地区块坐标应为 (0,0): %+v", c)
	}
	if c.GlobalChunkX != 0 || c.GlobalChunkZ != 0 {
		t.Fatalf("全局区块坐标应为 (0,0): %+v", c)
	}
	if c.Compression != CompressionZlib {
		t.Fatalf("压缩类型应为 zlib: %d", c.Compression)
	}
	if c.Timestamp != 1700000000 {
		t.Fatalf("时间戳不符: %d", c.Timestamp)
	}
}

func TestParseRegionTooSmall(t *testing.T) {
	if _, err := Parse(make([]byte, 100), 0, 0); err == nil {
		t.Fatal("过小文件应报错")
	}
}

func TestExtractChunkNBT(t *testing.T) {
	data := buildRegion(t, CompressionZlib)
	nbtBytes, compression, err := ExtractChunkNBT(data, 0, 0)
	if err != nil {
		t.Fatalf("ExtractChunkNBT 失败: %v", err)
	}
	if compression != CompressionZlib {
		t.Fatalf("压缩类型应为 zlib: %d", compression)
	}
	if len(nbtBytes) == 0 {
		t.Fatal("解压结果不应为空")
	}
	// 解压结果应能以 NBT 形态出现关键字段文本。
	if !bytes.Contains(nbtBytes, []byte("DataVersion")) {
		t.Fatalf("解压结果应含 DataVersion: %q", nbtBytes)
	}
}

func TestExtractChunkNBTGzip(t *testing.T) {
	data := buildRegion(t, CompressionGzip)
	if _, compression, err := ExtractChunkNBT(data, 0, 0); err != nil || compression != CompressionGzip {
		t.Fatalf("gzip 解压失败: comp=%d err=%v", compression, err)
	}
}

func TestExtractChunkNBTEmptyAndOutOfRange(t *testing.T) {
	data := buildRegion(t, CompressionZlib)
	// 空区块（1,1）未写入。
	if _, _, err := ExtractChunkNBT(data, 1, 1); err == nil {
		t.Fatal("空区块应报错")
	}
	// 越界坐标。
	if _, _, err := ExtractChunkNBT(data, 32, 0); err == nil {
		t.Fatal("越界坐标应报错")
	}
}
