package nbt

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// gzipBytes 对输入做 gzip 压缩（测试专用）。
func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("gzip 写入失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip 关闭失败: %v", err)
	}
	return buf.Bytes()
}

func TestParseFileGzip(t *testing.T) {
	top, err := ParseFile(gzipBytes(t, sampleCompound()))
	if err != nil {
		t.Fatalf("ParseFile(gzip) 失败: %v", err)
	}
	if dv, ok := top["DataVersion"].(int32); !ok || dv != 3465 {
		t.Fatalf("DataVersion 应为 int32(3465): %#v", top["DataVersion"])
	}
}

func TestParseFileUncompressed(t *testing.T) {
	// 非 gzip 数据应回退为未压缩 NBT 解析。
	top, err := ParseFile(sampleCompound())
	if err != nil {
		t.Fatalf("ParseFile(未压缩) 失败: %v", err)
	}
	if _, ok := top["Status"]; !ok {
		t.Fatalf("未压缩解析结果缺少 Status: %#v", top)
	}
}

func TestParseFileEmpty(t *testing.T) {
	if _, err := ParseFile(nil); err == nil {
		t.Fatal("空数据应报错")
	}
}

func TestDecompressGzipLimit(t *testing.T) {
	// 构造超过 MaxDecompressedSize 的高压缩比数据（全 0），应被上限拦截。
	huge := make([]byte, MaxDecompressedSize+1024)
	if _, err := DecompressGzip(gzipBytes(t, huge)); err == nil {
		t.Fatal("解压输出超过上限应报错")
	}
}

func TestDecompressGzipInvalid(t *testing.T) {
	if _, err := DecompressGzip([]byte("not-gzip")); err == nil {
		t.Fatal("非 gzip 数据应报错")
	}
}
