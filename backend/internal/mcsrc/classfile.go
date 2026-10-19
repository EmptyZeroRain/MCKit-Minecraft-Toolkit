package mcsrc

import (
	"errors"
	"fmt"
	"strings"
)

// JVM class 文件解析（JVMS §4）。
//
// 用途：在**不反编译、不依赖 JVM** 的前提下，从 class 字节码读出
// 「修饰符 / 种类 / 父类 / 接口 / 字段 / 方法」等结构信息，
// 供「类结构视图」与「继承关系」使用。
//
// 设计说明：
//   - 只解析到方法表为止，属性（Attribute）一律按长度跳过：结构视图不需要
//     Code / Signature / 注解等正文，跳过可显著降低解析成本与出错面；
//   - 所有读取都经 bounds 检查并累积错误，损坏 / 伪造的 class 字节不会 panic
//     （jar 内容来自上游，必须按不可信输入处理）；
//   - 常量池按 tag 精确推进（Long / Double 占两个槽位），避免后续错位。

const classMagic = 0xCAFEBABE

// access_flags 位（同一位置在类 / 字段 / 方法上语义不同，注释标注各自含义）。
const (
	accPublic       = 0x0001 // 类 / 字段 / 方法
	accPrivate      = 0x0002 // 字段 / 方法
	accProtected    = 0x0004 // 字段 / 方法
	accStatic       = 0x0008 // 字段 / 方法
	accFinal        = 0x0010 // 类 / 字段 / 方法
	accSynchronized = 0x0020 // 方法
	accSuper        = 0x0020 // 类（ACC_SUPER，与方法的 synchronized 同位）
	accVolatile     = 0x0040 // 字段
	accBridge       = 0x0040 // 方法
	accTransient    = 0x0080 // 字段
	accVarargs      = 0x0080 // 方法
	accNative       = 0x0100 // 方法
	accInterface    = 0x0200 // 类
	accAbstract     = 0x0400 // 类 / 方法
	accStrict       = 0x0800 // 方法
	accSynthetic    = 0x1000 // 类 / 字段 / 方法
	accAnnotation   = 0x2000 // 类
	accEnum         = 0x4000 // 类 / 字段
)

// ErrBadClassFile class 字节流损坏或不是 class 文件。
var ErrBadClassFile = errors.New("class 文件解析失败")

// MemberInfo 字段 / 方法的原始结构信息（描述符未渲染）。
type MemberInfo struct {
	AccessFlags uint16
	Name        string
	Descriptor  string
}

// ClassFile class 文件的解析结果（仅结构，不含字节码正文）。
type ClassFile struct {
	Minor       uint16
	Major       uint16
	AccessFlags uint16
	// ThisClass 本类全限定名（点分，如 net.minecraft.server.MinecraftServer）。
	ThisClass string
	// SuperClass 父类全限定名；解析不到时为 java.lang.Object。
	SuperClass string
	Interfaces []string
	Fields     []MemberInfo
	Methods    []MemberInfo
}

// Kind 返回类种类：class / interface / enum / annotation。
func (c *ClassFile) Kind() string { return kindFromFlags(c.AccessFlags) }

// kindFromFlags 由类访问标志推断种类。
func kindFromFlags(flags uint16) string {
	switch {
	case flags&accAnnotation != 0:
		return "annotation"
	case flags&accEnum != 0:
		return "enum"
	case flags&accInterface != 0:
		return "interface"
	default:
		return "class"
	}
}

// IsAbstract 是否为抽象类（含接口，接口本身即抽象）。
func (c *ClassFile) IsAbstract() bool { return c.AccessFlags&accAbstract != 0 }

// cpEntry 常量池条目。
type cpEntry struct {
	tag uint8
	// utf8 仅 tag == 1（Utf8）时有意义。
	utf8 string
	// idx1 / idx2 依 tag 含义不同：
	//   Class / String / MethodType / Module / Package → idx1 指向 Utf8
	//   Fieldref / Methodref / InterfaceMethodref / NameAndType / Dynamic / InvokeDynamic → idx1、idx2 指向其它条目
	//   MethodHandle → idx1 指向其它条目
	idx1 uint16
	idx2 uint16
	// num 数值常量的原始位模式：Integer / Float 取低 32 位，Long / Double 取 64 位。
	// 结构视图用不到它，但反汇编要渲染 LDC 的操作数，故在解析期一并保留。
	num int64
	// handleKind 仅 tag == 15（MethodHandle）时有意义，取值 1..9（JVMS §4.4.8）。
	handleKind uint8
}

// cpPool 常量池：命名切片类型（Go 不允许在切片字面量上定义方法）。
type cpPool []cpEntry

// classReader 带边界检查的顺序读取器：任一读取越界后置 err，后续读取返回零值。
type classReader struct {
	b   []byte
	pos int
	err error
}

func (r *classReader) fail() {
	if r.err == nil {
		r.err = ErrBadClassFile
	}
}

func (r *classReader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.pos+n > len(r.b) {
		r.fail()
		return false
	}
	return true
}

func (r *classReader) u1() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *classReader) u2() uint16 {
	if !r.need(2) {
		return 0
	}
	v := uint16(r.b[r.pos])<<8 | uint16(r.b[r.pos+1])
	r.pos += 2
	return v
}

func (r *classReader) u4() uint32 {
	if !r.need(4) {
		return 0
	}
	v := uint32(r.b[r.pos])<<24 | uint32(r.b[r.pos+1])<<16 |
		uint32(r.b[r.pos+2])<<8 | uint32(r.b[r.pos+3])
	r.pos += 4
	return v
}

func (r *classReader) bytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	v := r.b[r.pos : r.pos+n]
	r.pos += n
	return v
}

// skip 跳过 n 字节（属性正文）。
func (r *classReader) skip(n uint32) {
	if n > uint32(len(r.b)) { // 防御性上限：属性长度不可能超过文件本身
		r.fail()
		return
	}
	if !r.need(int(n)) {
		return
	}
	r.pos += int(n)
}

// ParseClassFile 解析 class 字节流。
// 输入损坏时返回 ErrBadClassFile，绝不 panic。
func ParseClassFile(data []byte) (*ClassFile, error) {
	r := &classReader{b: data}
	if r.u4() != classMagic {
		return nil, fmt.Errorf("%w: 魔数不匹配", ErrBadClassFile)
	}
	cf := &ClassFile{Minor: r.u2(), Major: r.u2()}

	pool, err := parseConstantPool(r)
	if err != nil {
		return nil, err
	}

	cf.AccessFlags = r.u2()
	thisIdx := r.u2()
	superIdx := r.u2()
	if r.err != nil {
		return nil, r.err
	}

	cf.ThisClass = pool.className(thisIdx)
	if cf.ThisClass == "" {
		return nil, fmt.Errorf("%w: 本类名缺失", ErrBadClassFile)
	}
	// super_class 为 0 时表示 java.lang.Object（自身）。
	cf.SuperClass = pool.className(superIdx)
	if cf.SuperClass == "" {
		cf.SuperClass = "java.lang.Object"
	}

	ifaceCount := r.u2()
	cf.Interfaces = make([]string, 0, ifaceCount)
	for i := 0; i < int(ifaceCount); i++ {
		idx := r.u2()
		if name := pool.className(idx); name != "" {
			cf.Interfaces = append(cf.Interfaces, name)
		}
	}

	fields, err := parseMembers(r, pool)
	if err != nil {
		return nil, err
	}
	cf.Fields = fields

	methods, err := parseMembers(r, pool)
	if err != nil {
		return nil, err
	}
	cf.Methods = methods
	if r.err != nil {
		return nil, r.err
	}
	return cf, nil
}

// parseConstantPool 解析常量池（索引 1..count-1）。
func parseConstantPool(r *classReader) (cpPool, error) {
	count := int(r.u2())
	if r.err != nil {
		return nil, r.err
	}
	if count < 1 {
		return nil, fmt.Errorf("%w: 常量池为空", ErrBadClassFile)
	}
	pool := make([]cpEntry, count)
	// Long / Double 占两个槽位，因此用显式下标推进。
	for i := 1; i < count; i++ {
		tag := r.u1()
		if r.err != nil {
			return nil, r.err
		}
		e := cpEntry{tag: tag}
		switch tag {
		case 1: // Utf8
			n := int(r.u2())
			b := r.bytes(n)
			if r.err != nil {
				return nil, r.err
			}
			e.utf8 = string(b)
		case 3, 4: // Integer / Float
			e.num = int64(r.u4())
		case 5, 6: // Long / Double：占两个槽位
			hi, lo := r.u4(), r.u4()
			e.num = int64(uint64(hi)<<32 | uint64(lo))
			i++
		case 7, 8, 16, 19, 20: // Class / String / MethodType / Module / Package
			e.idx1 = r.u2()
		case 9, 10, 11, 12, 17, 18: // Fieldref / Methodref / InterfaceMethodref / NameAndType / Dynamic / InvokeDynamic
			e.idx1 = r.u2()
			e.idx2 = r.u2()
		case 15: // MethodHandle
			e.handleKind = r.u1()
			e.idx1 = r.u2()
		default:
			return nil, fmt.Errorf("%w: 未知常量池 tag %d", ErrBadClassFile, tag)
		}
		if r.err != nil {
			return nil, r.err
		}
		pool[i] = e
	}
	return pool, nil
}

// utf8At 取索引指向的 Utf8 字符串；索引非法返回空串。
func (p cpPool) utf8At(idx uint16) string {
	if int(idx) >= len(p) || p[idx].tag != 1 {
		return ""
	}
	return p[idx].utf8
}

// className 把 Class 条目解析为点分全限定名。
func (p cpPool) className(idx uint16) string {
	if int(idx) >= len(p) || p[idx].tag != 7 {
		return ""
	}
	name := p.utf8At(p[idx].idx1)
	if name == "" {
		return ""
	}
	return strings.ReplaceAll(name, "/", ".")
}

// parseMembers 解析字段表或方法表（两者结构一致）。
func parseMembers(r *classReader, pool cpPool) ([]MemberInfo, error) {
	count := int(r.u2())
	if r.err != nil {
		return nil, r.err
	}
	out := make([]MemberInfo, 0, count)
	for i := 0; i < count; i++ {
		var m MemberInfo
		m.AccessFlags = r.u2()
		nameIdx := r.u2()
		descIdx := r.u2()
		if r.err != nil {
			return nil, r.err
		}
		m.Name = pool.utf8At(nameIdx)
		m.Descriptor = pool.utf8At(descIdx)
		// 属性表：逐个读长度并跳过正文。
		attrCount := int(r.u2())
		for j := 0; j < attrCount; j++ {
			r.u2() // attribute_name_index
			r.skip(r.u4())
			if r.err != nil {
				return nil, r.err
			}
		}
		out = append(out, m)
	}
	return out, nil
}
