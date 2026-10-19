// Package mappings 解析 Minecraft 生态常见的映射文件格式，
// 为「版本映射与类 / 方法查询」（TODO §21）提供统一导入模型。
//
// 支持的格式与来源：
//   - ProGuard（Mojang 官方映射）：client.txt / server.txt，
//     由版本清单的 downloads.client_mappings 指向；
//   - TSRG / TSRG2（MCPConfig，Forge / NeoForge 历史体系）；
//   - Tiny v1 / v2（Fabric Yarn，official → intermediary → named）。
//
// 设计原则：解析器只做「格式 → 结构化」的忠实转换，不猜测缺失的命名空间，
// 也不合成未提供的描述符；各名字列对应哪个命名空间由 Result.Namespaces 声明，
// 导入器据此写入 mapping_sets.note，避免使用者误读列含义。
package mappings

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Format 映射文件格式。
type Format string

// 支持的格式取值。
const (
	FormatProGuard Format = "proguard"
	FormatTSRG     Format = "tsrg"
	FormatTinyV1   Format = "tiny_v1"
	FormatTinyV2   Format = "tiny_v2"
)

// maxLineBytes 单行长度上限，防止异常文件占用过多内存。
const maxLineBytes = 4 * 1024 * 1024

// Member 一条成员映射（方法 / 字段 / 参数）。
type Member struct {
	// Kind method | field | param
	Kind string
	// ObfName 第一命名空间（混淆 / 官方名）下的成员名。
	ObfName string
	// MidName 中间命名空间（intermediary / SRG）下的成员名，未提供时为空。
	MidName string
	// NamedName 可读命名空间下的成员名，未提供时为空。
	NamedName string
	// DescKey 来源文件中用于区分重载的描述符原样值，仅作唯一性判别。
	DescKey string
	// ObfDesc 第一命名空间下的描述符（Tiny / TSRG 提供），ProGuard 为空。
	ObfDesc string
	// NamedDesc 可读类型描述符（ProGuard 提供），其余格式通常为空。
	NamedDesc string
}

// Class 一条类映射及其成员。
type Class struct {
	ObfName   string
	MidName   string
	NamedName string
	// Package 取自可读名（缺失时依次回退中间名、混淆名）的包路径。
	Package string
	Members []Member
}

// Result 一次解析的结果。
type Result struct {
	Format Format
	// Namespaces 各名字列对应的命名空间，顺序固定为 [第一, 中间, 可读]，
	// 未提供的命名空间以空串占位。
	Namespaces  []string
	Classes     []Class
	MemberCount int
}

// ErrUnknownFormat 无法识别映射格式。
var ErrUnknownFormat = errors.New("无法识别的映射格式（支持 ProGuard / TSRG / Tiny v1 / Tiny v2）")

// Parse 解析映射数据。formatHint 为空或 "auto" 时按内容自动识别。
func Parse(data []byte, formatHint string) (*Result, error) {
	raw, err := decompress(data)
	if err != nil {
		return nil, err
	}

	format := Format(strings.ToLower(strings.TrimSpace(formatHint)))
	if format == "" || format == "auto" {
		detected, err := Detect(raw)
		if err != nil {
			return nil, err
		}
		format = detected
	}

	switch format {
	case FormatProGuard, "mojang", "official":
		return parseProGuard(bytes.NewReader(raw))
	case FormatTSRG, "srg":
		return parseTSRG(bytes.NewReader(raw))
	case FormatTinyV1, "tiny":
		return parseTinyV1(bytes.NewReader(raw))
	case FormatTinyV2:
		return parseTinyV2(bytes.NewReader(raw))
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownFormat, format)
	}
}

// Detect 按内容识别映射格式。
func Detect(data []byte) (Format, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Tiny v2：首行 tiny <major> <minor> <ns...>
		if strings.HasPrefix(line, "tiny") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "2" {
				return FormatTinyV2, nil
			}
			return FormatTinyV1, nil
		}
		// Tiny v1：首行 v1 <ns...>
		if strings.HasPrefix(line, "v1") {
			return FormatTinyV1, nil
		}
		// ProGuard：类行形如 "com.example.Foo -> aaa:"
		if strings.Contains(line, " -> ") && strings.HasSuffix(line, ":") {
			return FormatProGuard, nil
		}
		// TSRG：类行为两到三列，无 "->"
		if len(strings.Fields(line)) >= 2 {
			return FormatTSRG, nil
		}
	}
	return "", ErrUnknownFormat
}

// decompress 识别 gzip 内容并解压（Mojang / 部分渠道会以 .gz 发布）。
func decompress(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip 解压失败: %w", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, 512*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("gzip 读取失败: %w", err)
	}
	return out, nil
}

// newScanner 构造带行缓冲上限的扫描器。
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return sc
}

// packageOf 取路径中最后一个分隔符之前的部分（同时兼容 / 与 . 分隔）。
func packageOf(name string) string {
	if name == "" {
		return ""
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[:i]
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return ""
}
