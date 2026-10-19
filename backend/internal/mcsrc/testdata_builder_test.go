package mcsrc

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// 本文件提供测试用「合成 class 文件」构造器。
// 真实 jar 不可提交进仓库，单元测试需要能精确控制字节码内容，
// 因此按 JVMS §4 手工拼装最小合法 class 文件（常量池 + 头 + 字段 / 方法表）。

// refSpec 常量池成员 / 类引用（构造查找引用索引所需的最小指令）。
type refSpec struct {
	// Kind 取值 field | method | iface | class。
	Kind string
	// Owner 点分全限定名（class 时即被引用的类名）。
	Owner string
	Name  string
	Desc  string
}

// memberSpec 字段 / 方法定义。
type memberSpec struct {
	Access uint16
	Name   string
	Desc   string
	// Refs 非空时该方法带一个 Code 属性，正文依次引用 Refs 中的常量池条目。
	Refs []refSpec
}

// classSpec class 定义。
type classSpec struct {
	Name       string // 点分全限定名
	Super      string // 点分；空表示 java.lang.Object
	Interfaces []string
	Access     uint16
	Major      uint16
	Minor      uint16
	Fields     []memberSpec
	Methods    []memberSpec
}

// cpWriter 常量池写入器（只写结构视图需要的 tag：Utf8 / Class / NameAndType）。
type cpWriter struct {
	buf   bytes.Buffer
	slots uint16
	seen  map[string]uint16
}

func newCPWriter() *cpWriter { return &cpWriter{seen: map[string]uint16{}} }

// utf8 写入（或复用）Utf8 条目，返回其索引。
func (w *cpWriter) utf8(s string) uint16 {
	if i, ok := w.seen["u:"+s]; ok {
		return i
	}
	w.buf.WriteByte(1)
	_ = binary.Write(&w.buf, binary.BigEndian, uint16(len(s)))
	w.buf.WriteString(s)
	w.slots++
	w.seen["u:"+s] = w.slots
	return w.slots
}

// class 写入 Class 条目（name 为内部形式 a/b/C），返回其索引。
func (w *cpWriter) class(name string) uint16 {
	if i, ok := w.seen["c:"+name]; ok {
		return i
	}
	nameIdx := w.utf8(name)
	w.buf.WriteByte(7)
	_ = binary.Write(&w.buf, binary.BigEndian, nameIdx)
	w.slots++
	w.seen["c:"+name] = w.slots
	return w.slots
}

// nameAndType 写入 NameAndType 条目，返回其索引。
func (w *cpWriter) nameAndType(name, desc string) uint16 {
	key := "n:" + name + ":" + desc
	if i, ok := w.seen[key]; ok {
		return i
	}
	nameIdx := w.utf8(name)
	descIdx := w.utf8(desc)
	w.buf.WriteByte(12)
	_ = binary.Write(&w.buf, binary.BigEndian, nameIdx)
	_ = binary.Write(&w.buf, binary.BigEndian, descIdx)
	w.slots++
	w.seen[key] = w.slots
	return w.slots
}

// memberRef 写入 Fieldref / Methodref / InterfaceMethodref 条目，返回其索引。
// tag 取 9（Fieldref）/ 10（Methodref）/ 11（InterfaceMethodref）。
func (w *cpWriter) memberRef(tag byte, owner, name, desc string) uint16 {
	key := fmt.Sprintf("r:%d:%s:%s:%s", tag, owner, name, desc)
	if i, ok := w.seen[key]; ok {
		return i
	}
	ownerIdx := w.class(owner)
	ntIdx := w.nameAndType(name, desc)
	w.buf.WriteByte(tag)
	_ = binary.Write(&w.buf, binary.BigEndian, ownerIdx)
	_ = binary.Write(&w.buf, binary.BigEndian, ntIdx)
	w.slots++
	w.seen[key] = w.slots
	return w.slots
}

// registerRef 把一条引用登记进常量池，返回其操作码与常量池索引。
func (w *cpWriter) registerRef(r refSpec) (opcode byte, index uint16) {
	switch r.Kind {
	case "field":
		return 0xb4, w.memberRef(9, internal(r.Owner), r.Name, r.Desc) // GETFIELD
	case "iface":
		return 0xb9, w.memberRef(11, internal(r.Owner), r.Name, r.Desc) // INVOKEINTERFACE
	case "class":
		return 0xbb, w.class(internal(r.Owner)) // NEW
	default: // method
		return 0xb6, w.memberRef(10, internal(r.Owner), r.Name, r.Desc) // INVOKEVIRTUAL
	}
}

// internal 点分名 → 内部形式。
func internal(name string) string {
	out := make([]byte, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			out[i] = '/'
			continue
		}
		out[i] = name[i]
	}
	return string(out)
}

// buildClass 生成一个最小合法 class 文件字节流。
// 字段 / 方法各带一个空属性表（属性数量 0），足以覆盖解析路径。
func buildClass(t *testing.T, spec classSpec) []byte {
	t.Helper()
	if spec.Major == 0 {
		spec.Major = 52 // Java 8
	}
	cp := newCPWriter()
	thisIdx := cp.class(internal(spec.Name))
	superIdx := uint16(0)
	if spec.Super != "" {
		superIdx = cp.class(internal(spec.Super))
	}
	ifaceIdx := make([]uint16, 0, len(spec.Interfaces))
	for _, i := range spec.Interfaces {
		ifaceIdx = append(ifaceIdx, cp.class(internal(i)))
	}
	// 成员名与描述符必须先进常量池（引用顺序无关，这里显式登记）。
	all := append(append([]memberSpec{}, spec.Fields...), spec.Methods...)
	for _, f := range all {
		cp.utf8(f.Name)
		cp.utf8(f.Desc)
	}
	// 方法体（引用）也必须在写出常量池之前登记：Code 属性名同样先登记。
	codeNameIdx := cp.utf8("Code")
	methodCodes := make([][]byte, len(spec.Methods))
	for i, m := range spec.Methods {
		if len(m.Refs) == 0 {
			continue
		}
		code := make([]byte, 0, len(m.Refs)*5+1)
		for _, r := range m.Refs {
			op, idx := cp.registerRef(r)
			code = append(code, op, byte(idx>>8), byte(idx))
			if op == 0xb9 { // INVOKEINTERFACE：u1 参数槽数 + u1 保留位
				code = append(code, 1, 0)
			}
		}
		code = append(code, 0xb1) // RETURN
		methodCodes[i] = code
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, uint32(classMagic))
	_ = binary.Write(&out, binary.BigEndian, spec.Minor)
	_ = binary.Write(&out, binary.BigEndian, spec.Major)
	_ = binary.Write(&out, binary.BigEndian, cp.slots+1) // count = 槽位数 + 1
	out.Write(cp.buf.Bytes())

	_ = binary.Write(&out, binary.BigEndian, spec.Access)
	_ = binary.Write(&out, binary.BigEndian, thisIdx)
	_ = binary.Write(&out, binary.BigEndian, superIdx)
	_ = binary.Write(&out, binary.BigEndian, uint16(len(ifaceIdx)))
	for _, i := range ifaceIdx {
		_ = binary.Write(&out, binary.BigEndian, i)
	}

	// writeCodeAttr 写出一个 Code 属性（max_stack / max_locals 取 1，异常表与子属性为空）。
	writeCodeAttr := func(code []byte) {
		_ = binary.Write(&out, binary.BigEndian, codeNameIdx)
		body := make([]byte, 0, 8+len(code)+4)
		var hdr bytes.Buffer
		_ = binary.Write(&hdr, binary.BigEndian, uint16(1))
		_ = binary.Write(&hdr, binary.BigEndian, uint16(1))
		_ = binary.Write(&hdr, binary.BigEndian, uint32(len(code)))
		body = append(body, hdr.Bytes()...)
		body = append(body, code...)
		body = append(body, 0, 0, 0, 0)
		_ = binary.Write(&out, binary.BigEndian, uint32(len(body)))
		out.Write(body)
	}

	writeMembers := func(list []memberSpec, codes [][]byte) {
		_ = binary.Write(&out, binary.BigEndian, uint16(len(list)))
		for i, m := range list {
			_ = binary.Write(&out, binary.BigEndian, m.Access)
			_ = binary.Write(&out, binary.BigEndian, cp.utf8(m.Name))
			_ = binary.Write(&out, binary.BigEndian, cp.utf8(m.Desc))
			if codes != nil && codes[i] != nil {
				_ = binary.Write(&out, binary.BigEndian, uint16(1)) // 属性数量 = 1（Code）
				writeCodeAttr(codes[i])
			} else {
				_ = binary.Write(&out, binary.BigEndian, uint16(0)) // 属性表为空
			}
		}
	}
	writeMembers(spec.Fields, nil)
	writeMembers(spec.Methods, methodCodes)
	_ = binary.Write(&out, binary.BigEndian, uint16(0)) // 类属性表为空
	return out.Bytes()
}

// seedJar 在 Store 的缓存目录写入 jar（含 meta），可选是否建索引。
// 返回 jar 摘要，供断言使用。
func seedJar(t *testing.T, s *Store, version, side string, classes map[string][]byte, withIndex bool) string {
	t.Helper()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		t.Fatalf("创建缓存目录失败: %v", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range classes {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("写入 zip 条目失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	raw := buf.Bytes()
	sum := sha1.Sum(raw)
	if err := os.WriteFile(s.jarPath(version, side), raw, 0o644); err != nil {
		t.Fatalf("写入 jar 失败: %v", err)
	}
	meta := &entryMeta{
		Version: version,
		Side:    side,
		SHA1:    hex.EncodeToString(sum[:]),
		Size:    int64(len(raw)),
		URL:     "https://example.com/fake.jar",
	}
	if err := s.writeMeta(version, side, meta); err != nil {
		t.Fatalf("写入 meta 失败: %v", err)
	}
	if withIndex {
		if err := s.buildIndexLocked(t.Context(), version, side); err != nil {
			t.Fatalf("建索引失败: %v", err)
		}
	}
	return meta.SHA1
}

// classPath 类名 → zip 条目路径。
func classPath(name string) string { return filepath.ToSlash(internal(name)) + ".class" }
