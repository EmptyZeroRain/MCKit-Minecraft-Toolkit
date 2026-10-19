package mcsrc

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// JVM 字节码反汇编（ASM Textifier 风格文本）。
//
// 用途：在**不反编译、不依赖 JVM** 的前提下，把 class 字节码渲染成
// 「类头 + 字段 + 方法（含 Code 属性内的指令 / 标签 / 行号 / 局部变量表 / 异常表）」
// 的文本反汇编，供前端「字节码」标签页展示。
//
// 文本形态对齐上游 mcsrc 使用的 ASM Textifier 输出：上游由浏览器内的 Java indexer
// 产出该文本，本项目改由 Go 侧产出（部署侧无需 JVM），因此上游的 bytecode
// 语法高亮规则（Monarch）可原样复用。
//
// 覆盖范围（v1）：
//   - class：版本 / 访问标志 / 修饰符 / SOURCEFILE / SIGNATURE / INNERCLASS /
//     NESTHOST / NESTMEMBERS / ENCLOSINGMETHOD / PERMITTEDSUBCLASSES / BOOTSTRAPMETHODS
//   - field：访问标志 / 修饰符 / 描述符 / 名称 / SIGNATURE / 常量值（行尾注释）
//   - method：访问标志 / 修饰符 / 名称 / 描述符 / SIGNATURE / THROWS /
//     Code（TRYCATCHBLOCK / 标签 / LINENUMBER / 指令 / LOCALVARIABLE / MAXSTACK / MAXLOCALS）
//
// 与结构视图（classfile.go）的分工：结构视图**按长度跳过全部属性**（层级扫描上万个类
// 时省内存），本文件是**带属性的完整解析**，只在单类反汇编时使用，两者互不影响。
//
// 有意未覆盖（v1 显式标注，不静默丢弃）：
//   - StackMapTable 的帧：属校验器元数据，不影响指令阅读，仅输出一行提示；
//   - 注解 / 类型注解明细、Record / Module 属性：不参与指令阅读，整体略过。
const (
	// maxBytecodeTextBytes 单类反汇编文本上限（防御畸形 / 超大方法撑爆响应）。
	maxBytecodeTextBytes = 1 << 20
	// maxSwitchTargets 单个 switch 渲染的跳转表条目上限。
	maxSwitchTargets = 4096
	// maxCodeAttributeBytes Code 属性正文上限（防御畸形属性长度）。
	maxCodeAttributeBytes = 1 << 20
	// maxLocalVars 单个方法渲染的局部变量表条目上限。
	maxLocalVars = 4096
)

// 反汇编文本缩进（对齐 ASM Textifier：类成员 2 空格、指令 4 空格、行号等 5 空格）。
const (
	indentMember    = 2
	indentCode      = 4
	indentDirective = 5
)

// opKind 指令操作数形态。
type opKind uint8

const (
	opNone         opKind = iota // 无操作数
	opLocal                      // 局部变量索引（u1）
	opByteConst                  // BIPUSH（s1）
	opShortConst                 // SIPUSH（s2）
	opCP1                        // LDC（u1 常量池索引）
	opCP2                        // LDC_W / 字段与方法引用 / 类型指令（u2）
	opCP2Pad                     // LDC2_W / INVOKEDYNAMIC（u2 + 2 字节保留位）
	opBranch2                    // 2 字节相对偏移
	opBranch4                    // 4 字节相对偏移（GOTO_W / JSR_W）
	opIInc                       // IINC（局部变量索引 u1 + 常量 s1）
	opNewArray                   // NEWARRAY（u1 数组类型）
	opMultiArray                 // MULTIANEWARRAY（u2 类索引 + u1 维数）
	opInvokeIface                // INVOKEINTERFACE（u2 + u1 参数槽数 + 1 字节保留位）
	opTableSwitch                // TABLESWITCH
	opLookupSwitch               // LOOKUPSWITCH
	opWide                       // WIDE
)

// opcodeInfo 单条指令的静态信息。
type opcodeInfo struct {
	name string
	kind opKind
}

// opcodeTable 操作码表（JVMS §6.5 与 §6.2 的保留码）。
var opcodeTable = buildOpcodeTable()

// buildOpcodeTable 构造操作码表。
// 连续编号的同类指令用 seq 批量登记，减少手工罗列时的笔误。
func buildOpcodeTable() [256]opcodeInfo {
	var t [256]opcodeInfo
	seq := func(start byte, names ...string) {
		for i, n := range names {
			t[int(start)+i] = opcodeInfo{name: n}
		}
	}
	one := func(code byte, name string, kind opKind) {
		t[code] = opcodeInfo{name: name, kind: kind}
	}

	one(0x00, "NOP", opNone)
	one(0x01, "ACONST_NULL", opNone)
	seq(0x02, "ICONST_M1", "ICONST_0", "ICONST_1", "ICONST_2", "ICONST_3", "ICONST_4", "ICONST_5")
	seq(0x09, "LCONST_0", "LCONST_1")
	seq(0x0b, "FCONST_0", "FCONST_1", "FCONST_2")
	seq(0x0e, "DCONST_0", "DCONST_1")
	one(0x10, "BIPUSH", opByteConst)
	one(0x11, "SIPUSH", opShortConst)
	one(0x12, "LDC", opCP1)
	one(0x13, "LDC_W", opCP2)
	one(0x14, "LDC2_W", opCP2Pad)
	seq(0x15, "ILOAD", "LLOAD", "FLOAD", "DLOAD", "ALOAD")
	for _, c := range []byte{0x15, 0x16, 0x17, 0x18, 0x19} {
		t[c].kind = opLocal
	}
	seq(0x1a, "ILOAD_0", "ILOAD_1", "ILOAD_2", "ILOAD_3",
		"LLOAD_0", "LLOAD_1", "LLOAD_2", "LLOAD_3",
		"FLOAD_0", "FLOAD_1", "FLOAD_2", "FLOAD_3",
		"DLOAD_0", "DLOAD_1", "DLOAD_2", "DLOAD_3",
		"ALOAD_0", "ALOAD_1", "ALOAD_2", "ALOAD_3")
	seq(0x2e, "IALOAD", "LALOAD", "FALOAD", "DALOAD", "AALOAD", "BALOAD", "CALOAD", "SALOAD")
	seq(0x36, "ISTORE", "LSTORE", "FSTORE", "DSTORE", "ASTORE")
	for _, c := range []byte{0x36, 0x37, 0x38, 0x39, 0x3a} {
		t[c].kind = opLocal
	}
	seq(0x3b, "ISTORE_0", "ISTORE_1", "ISTORE_2", "ISTORE_3",
		"LSTORE_0", "LSTORE_1", "LSTORE_2", "LSTORE_3",
		"FSTORE_0", "FSTORE_1", "FSTORE_2", "FSTORE_3",
		"DSTORE_0", "DSTORE_1", "DSTORE_2", "DSTORE_3",
		"ASTORE_0", "ASTORE_1", "ASTORE_2", "ASTORE_3")
	seq(0x4f, "IASTORE", "LASTORE", "FASTORE", "DASTORE", "AASTORE", "BASTORE", "CASTORE", "SASTORE")
	seq(0x57, "POP", "POP2", "DUP", "DUP_X1", "DUP_X2", "DUP2", "DUP2_X1", "DUP2_X2", "SWAP")
	seq(0x60, "IADD", "LADD", "FADD", "DADD", "ISUB", "LSUB", "FSUB", "DSUB",
		"IMUL", "LMUL", "FMUL", "DMUL", "IDIV", "LDIV", "FDIV", "DDIV",
		"IREM", "LREM", "FREM", "DREM", "INEG", "LNEG", "FNEG", "DNEG",
		"ISHL", "LSHL", "ISHR", "LSHR", "IUSHR", "LUSHR", "IAND", "LAND",
		"IOR", "LOR", "IXOR", "LXOR")
	one(0x84, "IINC", opIInc)
	seq(0x85, "I2L", "I2F", "I2D", "L2I", "L2F", "L2D", "F2I", "F2L", "F2D", "D2I", "D2L", "D2F", "I2B", "I2C", "I2S")
	seq(0x94, "LCMP", "FCMPL", "FCMPG", "DCMPL", "DCMPG")
	seq(0x99, "IFEQ", "IFNE", "IFLT", "IFGE", "IFGT", "IFLE",
		"IF_ICMPEQ", "IF_ICMPNE", "IF_ICMPLT", "IF_ICMPGE", "IF_ICMPGT", "IF_ICMPLE",
		"IF_ACMPEQ", "IF_ACMPNE")
	for _, c := range []byte{0x99, 0x9a, 0x9b, 0x9c, 0x9d, 0x9e, 0x9f, 0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6} {
		t[c].kind = opBranch2
	}
	one(0xa7, "GOTO", opBranch2)
	one(0xa8, "JSR", opBranch2)
	one(0xa9, "RET", opLocal)
	one(0xaa, "TABLESWITCH", opTableSwitch)
	one(0xab, "LOOKUPSWITCH", opLookupSwitch)
	seq(0xac, "IRETURN", "LRETURN", "FRETURN", "DRETURN", "ARETURN", "RETURN")
	seq(0xb2, "GETSTATIC", "PUTSTATIC", "GETFIELD", "PUTFIELD")
	for _, c := range []byte{0xb2, 0xb3, 0xb4, 0xb5} {
		t[c].kind = opCP2
	}
	seq(0xb6, "INVOKEVIRTUAL", "INVOKESPECIAL", "INVOKESTATIC")
	for _, c := range []byte{0xb6, 0xb7, 0xb8} {
		t[c].kind = opCP2
	}
	one(0xb9, "INVOKEINTERFACE", opInvokeIface)
	one(0xba, "INVOKEDYNAMIC", opCP2Pad)
	one(0xbb, "NEW", opCP2)
	one(0xbc, "NEWARRAY", opNewArray)
	one(0xbd, "ANEWARRAY", opCP2)
	one(0xbe, "ARRAYLENGTH", opNone)
	one(0xbf, "ATHROW", opNone)
	one(0xc0, "CHECKCAST", opCP2)
	one(0xc1, "INSTANCEOF", opCP2)
	one(0xc2, "MONITORENTER", opNone)
	one(0xc3, "MONITOREXIT", opNone)
	one(0xc4, "WIDE", opWide)
	one(0xc5, "MULTIANEWARRAY", opMultiArray)
	one(0xc6, "IFNULL", opBranch2)
	one(0xc7, "IFNONNULL", opBranch2)
	one(0xc8, "GOTO_W", opBranch4)
	one(0xc9, "JSR_W", opBranch4)
	one(0xca, "BREAKPOINT", opNone)
	one(0xfe, "IMPDEP1", opNone)
	one(0xff, "IMPDEP2", opNone)
	return t
}

// arrayTypeNames NEWARRAY 的数组元素类型（JVMS §6.5 newarray）。
var arrayTypeNames = map[uint8]string{
	4: "T_BOOLEAN", 5: "T_CHAR", 6: "T_FLOAT", 7: "T_DOUBLE",
	8: "T_BYTE", 9: "T_SHORT", 10: "T_INT", 11: "T_LONG",
}

// methodHandleKindNames MethodHandle 引用类型名（JVMS §5.4.3.5）。
var methodHandleKindNames = map[uint8]string{
	1: "GETFIELD", 2: "GETSTATIC", 3: "PUTFIELD", 4: "PUTSTATIC",
	5: "INVOKEVIRTUAL", 6: "INVOKESTATIC", 7: "INVOKESPECIAL",
	8: "NEWINVOKESPECIAL", 9: "INVOKEINTERFACE",
}

// attribute 原始属性（名称 + 正文）。
type attribute struct {
	name string
	body []byte
}

// fullMember 字段 / 方法（含属性正文）。
type fullMember struct {
	AccessFlags uint16
	NameIdx     uint16
	DescIdx     uint16
	Attrs       []attribute
}

// fullClass 反汇编所需的完整 class 结构（与 ClassFile 的区别：保留属性正文）。
type fullClass struct {
	Minor       uint16
	Major       uint16
	AccessFlags uint16
	ThisIdx     uint16
	SuperIdx    uint16
	Interfaces  []uint16
	Fields      []fullMember
	Methods     []fullMember
	Attrs       []attribute
	pool        cpPool
}

// ClassBytecode 单个类的字节码反汇编结果。
type ClassBytecode struct {
	Name   string `json:"name"`
	Simple string `json:"simple"`
	Pkg    string `json:"pkg"`
	Kind   string `json:"kind"`
	Major  int    `json:"major"`
	Minor  int    `json:"minor"`
	// Text ASM Textifier 风格的反汇编全文（前端按 bytecode 语言高亮）。
	Text string `json:"text"`
	// MethodCount 方法总数（含无 Code 的抽象 / native 方法）。
	MethodCount int `json:"method_count"`
	// InstructionCount 指令总数（不含标签与伪指令），供状态栏展示。
	InstructionCount int `json:"instruction_count"`
	// Truncated 文本达到上限被截断。
	Truncated bool `json:"truncated"`
}

// Disassembly 反汇编结果。
type Disassembly struct {
	// Text ASM Textifier 风格的反汇编全文。
	Text string
	// AccessFlags / Major / Minor 类头信息（供调用方渲染徽章）。
	AccessFlags uint16
	Major       int
	Minor       int
	// MethodCount 方法总数（含无 Code 的抽象 / native 方法）。
	MethodCount int
	// InstructionCount 指令总数（不含标签与伪指令）。
	InstructionCount int
	// Truncated 文本达到上限被截断。
	Truncated bool
}

// Disassemble 反汇编 class 字节流，返回 ASM Textifier 风格文本。
// 输入损坏时返回 ErrBadClassFile，绝不 panic。
func Disassemble(data []byte) (*Disassembly, error) {
	fc, err := parseClassFull(data)
	if err != nil {
		return nil, err
	}
	d := &disassembler{pool: fc.pool}
	d.renderClass(fc)
	return &Disassembly{
		Text:             d.out.String(),
		AccessFlags:      fc.AccessFlags,
		Major:            int(fc.Major),
		Minor:            int(fc.Minor),
		MethodCount:      len(fc.Methods),
		InstructionCount: d.instructions,
		Truncated:        d.truncated,
	}, nil
}

// ClassBytecode 反汇编单个类（要求该版本的 jar 已缓存；与 jar 体积无关）。
func (s *Store) ClassBytecode(version, side, className string) (*ClassBytecode, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return nil, err
	}
	name, err := normalizeClassName(className)
	if err != nil {
		return nil, err
	}
	// 与结构视图一致：jar 未缓存即视为「尚未准备」。
	if st := s.statusLocked(v, sd); !st.Cached {
		return nil, ErrNotFound
	}

	zr, err := s.openJar(v, sd)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()

	data, err := readClassEntry(zr, name)
	if err != nil {
		return nil, err
	}
	dis, err := Disassemble(data)
	if err != nil {
		return nil, err
	}
	pkg, simple := splitClassName(name)
	return &ClassBytecode{
		Name:             name,
		Simple:           simple,
		Pkg:              pkg,
		Kind:             kindFromFlags(dis.AccessFlags),
		Major:            dis.Major,
		Minor:            dis.Minor,
		Text:             dis.Text,
		MethodCount:      dis.MethodCount,
		InstructionCount: dis.InstructionCount,
		Truncated:        dis.Truncated,
	}, nil
}

// parseClassFull 完整解析 class（含属性正文），供反汇编使用。
func parseClassFull(data []byte) (*fullClass, error) {
	r := &classReader{b: data}
	if r.u4() != classMagic {
		return nil, fmt.Errorf("%w: 魔数不匹配", ErrBadClassFile)
	}
	fc := &fullClass{Minor: r.u2(), Major: r.u2()}

	pool, err := parseConstantPool(r)
	if err != nil {
		return nil, err
	}
	fc.pool = pool

	fc.AccessFlags = r.u2()
	fc.ThisIdx = r.u2()
	fc.SuperIdx = r.u2()
	if r.err != nil {
		return nil, r.err
	}
	if pool.className(fc.ThisIdx) == "" {
		return nil, fmt.Errorf("%w: 本类名缺失", ErrBadClassFile)
	}

	ifaceCount := int(r.u2())
	fc.Interfaces = make([]uint16, 0, ifaceCount)
	for i := 0; i < ifaceCount; i++ {
		fc.Interfaces = append(fc.Interfaces, r.u2())
	}

	if fc.Fields, err = parseFullMembers(r, pool); err != nil {
		return nil, err
	}
	if fc.Methods, err = parseFullMembers(r, pool); err != nil {
		return nil, err
	}
	fc.Attrs = parseAttributes(r, pool)
	if r.err != nil {
		return nil, r.err
	}
	return fc, nil
}

// parseFullMembers 解析字段表或方法表，保留属性正文。
func parseFullMembers(r *classReader, pool cpPool) ([]fullMember, error) {
	count := int(r.u2())
	if r.err != nil {
		return nil, r.err
	}
	out := make([]fullMember, 0, count)
	for i := 0; i < count; i++ {
		var m fullMember
		m.AccessFlags = r.u2()
		m.NameIdx = r.u2()
		m.DescIdx = r.u2()
		if r.err != nil {
			return nil, r.err
		}
		m.Attrs = parseAttributes(r, pool)
		out = append(out, m)
	}
	return out, nil
}

// parseAttributes 解析属性表，读取名称与正文（不递归解释正文）。
func parseAttributes(r *classReader, pool cpPool) []attribute {
	count := int(r.u2())
	if r.err != nil {
		return nil
	}
	out := make([]attribute, 0, count)
	for i := 0; i < count; i++ {
		nameIdx := r.u2()
		length := r.u4()
		if r.err != nil {
			return out
		}
		body := r.bytes(int(length))
		if r.err != nil {
			return out
		}
		out = append(out, attribute{name: pool.utf8At(nameIdx), body: body})
	}
	return out
}

// attr 取指定名称的属性；不存在返回 nil。
func attrOf(attrs []attribute, name string) *attribute {
	for i := range attrs {
		if attrs[i].name == name {
			return &attrs[i]
		}
	}
	return nil
}

// hasAttr 是否存在指定属性。
func hasAttr(attrs []attribute, name string) bool { return attrOf(attrs, name) != nil }

// disassembler 反汇编状态。
type disassembler struct {
	pool cpPool
	out  strings.Builder
	// instructions 已渲染的指令条数（不含标签 / 伪指令）。
	instructions int
	// truncated 文本达到上限，后续内容不再输出。
	truncated bool
	// bootstrapAttr BootstrapMethods 属性正文（INVOKEDYNAMIC 需要）。
	bootstrapAttr []byte
	// bootstrapMethods 解析后的引导方法表（惰性解析）。
	bootstrapMethods []bootstrapMethod
}

// line 输出一行（带缩进）；超出上限后置 truncated 并停止输出。
func (d *disassembler) line(indent int, s string) {
	if d.truncated {
		return
	}
	if d.out.Len()+len(s) > maxBytecodeTextBytes {
		d.truncated = true
		return
	}
	d.out.WriteString(strings.Repeat(" ", indent))
	d.out.WriteString(s)
	d.out.WriteByte('\n')
}

// blank 输出空行（超限时静默忽略）。
func (d *disassembler) blank() {
	if d.truncated {
		return
	}
	d.out.WriteByte('\n')
}

// renderClass 渲染整个类。
func (d *disassembler) renderClass(fc *fullClass) {
	d.line(0, fmt.Sprintf("// class version %d.%d (%d)", fc.Major, fc.Minor, fc.Major))
	d.line(0, fmt.Sprintf("// access flags 0x%x", fc.AccessFlags))
	d.line(0, fmt.Sprintf("%s %s {", strings.Join(classAccessWords(fc.AccessFlags), " "), d.internalName(fc.ThisIdx)))

	// BootstrapMethods 在类属性表中（顺序不保证），需先取到正文，方法内的
	// INVOKEDYNAMIC 才能渲染出引导方法。
	if bm := attrOf(fc.Attrs, "BootstrapMethods"); bm != nil {
		d.bootstrapAttr = bm.body
	}

	for i := range fc.Attrs {
		d.renderClassAttr(&fc.Attrs[i])
	}
	for i := range fc.Fields {
		d.blank()
		d.renderField(&fc.Fields[i])
	}
	for i := range fc.Methods {
		d.blank()
		d.renderMethod(&fc.Methods[i])
	}
	d.line(0, "}")
}

// classAccessWords 类级修饰词（含 super / interface / annotation / enum 的 Textifier 习惯）。
func classAccessWords(flags uint16) []string {
	out := make([]string, 0, 6)
	out = appendAccess(out, flags)
	if flags&accFinal != 0 {
		out = append(out, "final")
	}
	if flags&accSuper != 0 {
		out = append(out, "super")
	}
	switch {
	case flags&accAnnotation != 0:
		out = append(out, "abstract", "interface", "annotation")
	case flags&accInterface != 0:
		out = append(out, "abstract", "interface")
	case flags&accEnum != 0:
		out = append(out, "enum")
	default:
		out = append(out, "class")
	}
	if flags&accSynthetic != 0 {
		out = append(out, "synthetic")
	}
	return out
}

// renderClassAttr 渲染类级属性（只渲染与阅读指令相关的部分）。
func (d *disassembler) renderClassAttr(a *attribute) {
	r := &classReader{b: a.body}
	switch a.name {
	case "SourceFile":
		d.line(indentMember, "SOURCEFILE "+d.pool.utf8At(r.u2()))
	case "Signature":
		d.line(indentMember, "SIGNATURE "+d.pool.utf8At(r.u2()))
	case "NestHost":
		d.line(indentMember, "NESTHOST "+d.classRefText(r.u2()))
	case "NestMembers":
		n := int(r.u2())
		for i := 0; i < n && r.err == nil; i++ {
			d.line(indentMember, "NESTMEMBER "+d.classRefText(r.u2()))
		}
	case "PermittedSubclasses":
		n := int(r.u2())
		for i := 0; i < n && r.err == nil; i++ {
			d.line(indentMember, "PERMITTEDSUBCLASS "+d.classRefText(r.u2()))
		}
	case "EnclosingMethod":
		owner := d.classRefText(r.u2())
		natIdx := r.u2()
		if name, desc, ok := d.pool.nameAndType(natIdx); ok {
			d.line(indentMember, fmt.Sprintf("ENCLOSINGMETHOD %s.%s (%s)", owner, name, desc))
			return
		}
		d.line(indentMember, "ENCLOSINGMETHOD "+owner)
	case "InnerClasses":
		n := int(r.u2())
		for i := 0; i < n && r.err == nil; i++ {
			innerIdx := r.u2()
			outerIdx := r.u2()
			nameIdx := r.u2()
			access := r.u2()
			inner := d.internalName(innerIdx)
			outer := "-"
			if outerIdx != 0 {
				outer = d.internalName(outerIdx)
			}
			simple := d.pool.utf8At(nameIdx)
			if simple == "" {
				simple = "-"
			}
			d.line(indentMember, fmt.Sprintf("INNERCLASS %s %s %s 0x%x", inner, outer, simple, access))
		}
	case "BootstrapMethods":
		for _, bsm := range parseBootstrapMethods(a.body) {
			d.line(indentMember, fmt.Sprintf("BOOTSTRAPMETHOD %s", d.constantText(bsm.handleIdx)))
		}
	}
}

// renderField 渲染字段。
func (d *disassembler) renderField(f *fullMember) {
	d.line(indentMember, fmt.Sprintf("// access flags 0x%x", f.AccessFlags))
	mods := strings.Join(FieldModifiers(f.AccessFlags), " ")
	desc := d.pool.utf8At(f.DescIdx)
	name := d.pool.utf8At(f.NameIdx)
	line := fmt.Sprintf("%s %s", strings.TrimSpace(mods+" "+desc), name)
	if cv := attrOf(f.Attrs, "ConstantValue"); cv != nil {
		r := &classReader{b: cv.body}
		line += " // constant " + d.constantText(r.u2())
	}
	d.line(indentMember, line)
	if sig := attrOf(f.Attrs, "Signature"); sig != nil {
		r := &classReader{b: sig.body}
		d.line(indentMember, "SIGNATURE "+d.pool.utf8At(r.u2()))
	}
}

// renderMethod 渲染方法（含 Code 属性）。
func (d *disassembler) renderMethod(m *fullMember) {
	d.line(indentMember, fmt.Sprintf("// access flags 0x%x", m.AccessFlags))
	mods := strings.Join(MethodModifiers(m.AccessFlags), " ")
	name := d.pool.utf8At(m.NameIdx)
	desc := d.pool.utf8At(m.DescIdx)
	d.line(indentMember, strings.TrimSpace(fmt.Sprintf("%s %s%s", mods, name, desc)))

	if ex := attrOf(m.Attrs, "Exceptions"); ex != nil {
		r := &classReader{b: ex.body}
		n := int(r.u2())
		for i := 0; i < n && r.err == nil; i++ {
			d.line(indentCode, "THROWS "+d.classRefText(r.u2()))
		}
	}
	if sig := attrOf(m.Attrs, "Signature"); sig != nil {
		r := &classReader{b: sig.body}
		d.line(indentCode, "SIGNATURE "+d.pool.utf8At(r.u2()))
	}

	if code := attrOf(m.Attrs, "Code"); code != nil {
		d.renderCode(code.body)
	}
}

// lineNumber LineNumberTable 条目。
type lineNumber struct {
	pc   int
	line int
}

// localVar LocalVariableTable 条目。
type localVar struct {
	name, desc string
	start, end int
	index      int
}

// exceptionHandler Code 属性异常表条目。
type exceptionHandler struct {
	start, end, handler int
	// typ 捕获类型（内部名）；空串表示 finally（catch_type = 0）。
	typ string
}

// renderCode 渲染 Code 属性：异常表 → 标签 / 行号 / 指令 → 局部变量表 → MAXSTACK / MAXLOCALS。
func (d *disassembler) renderCode(body []byte) {
	r := &classReader{b: body}
	maxStack := r.u2()
	maxLocals := r.u2()
	codeLen := int(r.u4())
	if r.err != nil || codeLen < 0 || codeLen > maxCodeAttributeBytes {
		d.line(indentCode, "// Code 属性解析失败（class 损坏）")
		return
	}
	code := r.bytes(codeLen)
	if r.err != nil {
		d.line(indentCode, "// Code 属性解析失败（class 损坏）")
		return
	}

	handlers := parseExceptionTable(r, d.pool)
	framesOmitted := false
	var lines []lineNumber
	var locals []localVar
	attrCount := int(r.u2())
	for i := 0; i < attrCount && r.err == nil; i++ {
		nameIdx := r.u2()
		length := r.u4()
		if r.err != nil {
			break
		}
		attrBody := r.bytes(int(length))
		if r.err != nil {
			break
		}
		r2 := &classReader{b: attrBody}
		switch d.pool.utf8At(nameIdx) {
		case "LineNumberTable":
			n := int(r2.u2())
			for j := 0; j < n && r2.err == nil; j++ {
				lines = append(lines, lineNumber{pc: int(r2.u2()), line: int(r2.u2())})
			}
		case "LocalVariableTable":
			n := int(r2.u2())
			for j := 0; j < n && r2.err == nil && len(locals) < maxLocalVars; j++ {
				start := int(r2.u2())
				length := int(r2.u2())
				nameI := r2.u2()
				descI := r2.u2()
				locals = append(locals, localVar{
					name:  d.pool.utf8At(nameI),
					desc:  d.pool.utf8At(descI),
					start: start,
					end:   start + length,
					index: int(r2.u2()),
				})
			}
		case "StackMapTable":
			framesOmitted = true
		}
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].pc < lines[j].pc })

	// 异常表：Textifier 在指令之前输出 TRYCATCHBLOCK。
	for _, h := range handlers {
		typ := h.typ
		if typ == "" {
			typ = "finally" // catch_type = 0
		}
		d.line(indentCode, fmt.Sprintf("TRYCATCHBLOCK L%d L%d L%d %s", h.start, h.end, h.handler, typ))
	}

	insns := d.decode(code)
	d.instructions += len(insns)

	// 需要输出标签的偏移：分支 / switch 目标 + 行号 + 异常表 + 局部变量范围。
	labels := make(map[int]bool, len(insns)/2+8)
	for _, in := range insns {
		for _, t := range in.targets {
			labels[t] = true
		}
	}
	for _, ln := range lines {
		labels[ln.pc] = true
	}
	for _, h := range handlers {
		labels[h.start], labels[h.end], labels[h.handler] = true, true, true
	}
	for _, lv := range locals {
		labels[lv.start], labels[lv.end] = true, true
	}

	lineIdx := 0
	for _, in := range insns {
		if labels[in.pos] {
			d.line(indentCode, fmt.Sprintf("L%d", in.pos))
		}
		for lineIdx < len(lines) && lines[lineIdx].pc <= in.pos {
			if lines[lineIdx].pc == in.pos {
				d.line(indentDirective, fmt.Sprintf("LINENUMBER %d L%d", lines[lineIdx].line, in.pos))
			}
			lineIdx++
		}
		for _, text := range strings.Split(in.text, "\n") {
			d.line(indentCode, text)
		}
	}
	// 行号指向最后一条指令之后的偏移（如方法结尾的收尾行号）：补标签后输出。
	for ; lineIdx < len(lines); lineIdx++ {
		d.line(indentCode, fmt.Sprintf("L%d", lines[lineIdx].pc))
		d.line(indentDirective, fmt.Sprintf("LINENUMBER %d L%d", lines[lineIdx].line, lines[lineIdx].pc))
	}

	for _, lv := range locals {
		d.line(indentCode, fmt.Sprintf("LOCALVARIABLE %s %s L%d L%d %d", lv.name, lv.desc, lv.start, lv.end, lv.index))
	}
	if framesOmitted {
		d.line(indentCode, "// StackMapTable 帧已省略（校验器元数据，不影响指令阅读）")
	}
	d.line(indentCode, fmt.Sprintf("MAXSTACK = %d", maxStack))
	d.line(indentCode, fmt.Sprintf("MAXLOCALS = %d", maxLocals))
}

// parseExceptionTable 解析 Code 属性中的异常表（catch_type 经常量池解析为内部名）。
func parseExceptionTable(r *classReader, pool cpPool) []exceptionHandler {
	count := int(r.u2())
	if r.err != nil {
		return nil
	}
	out := make([]exceptionHandler, 0, count)
	for i := 0; i < count; i++ {
		h := exceptionHandler{
			start:   int(r.u2()),
			end:     int(r.u2()),
			handler: int(r.u2()),
		}
		catchIdx := r.u2()
		if r.err != nil {
			return out
		}
		// catch_type = 0 表示 finally（捕获所有异常）。
		if catchIdx != 0 {
			h.typ = pool.internalNameAt(catchIdx)
		}
		out = append(out, h)
	}
	return out
}

// insn 一条已渲染的指令。
type insn struct {
	// pos 指令在 code 数组中的起始偏移。
	pos int
	// text 渲染文本；switch 等多行内容以 \n 分隔（各行已含相对缩进）。
	text string
	// targets 该指令引用的跳转目标偏移（分支 / switch）。
	targets []int
}

// decode 解码 code 数组为指令序列。
// 遇到截断 / 未知操作码时输出一条注释并停止（不 panic、不误读后续字节）。
func (d *disassembler) decode(code []byte) []insn {
	out := make([]insn, 0, len(code)/2)
	for pos := 0; pos < len(code); {
		info := opcodeTable[code[pos]]
		if info.name == "" {
			out = append(out, insn{pos: pos, text: fmt.Sprintf("// 未知操作码 0x%02x", code[pos])})
			return out
		}
		start := pos
		pos++
		have := func(n int) bool { return pos+n <= len(code) }
		truncated := func() []insn {
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s // 操作数不完整（class 损坏）", info.name)})
			return out
		}

		switch info.kind {
		case opNone:
			out = append(out, insn{pos: start, text: info.name})
		case opLocal:
			if !have(1) {
				return truncated()
			}
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s %d", info.name, code[pos])})
			pos++
		case opByteConst:
			if !have(1) {
				return truncated()
			}
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s %d", info.name, int8(code[pos]))})
			pos++
		case opShortConst:
			if !have(2) {
				return truncated()
			}
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s %d", info.name, int16(binary.BigEndian.Uint16(code[pos:])))})
			pos += 2
		case opCP1:
			if !have(1) {
				return truncated()
			}
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s %s", info.name, d.constantText(uint16(code[pos])))})
			pos++
		case opCP2:
			if !have(2) {
				return truncated()
			}
			idx := binary.BigEndian.Uint16(code[pos:])
			pos += 2
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s %s", info.name, d.operandText(code[start], idx))})
		case opCP2Pad:
			if !have(4) {
				return truncated()
			}
			idx := binary.BigEndian.Uint16(code[pos:])
			pos += 4
			if code[start] == 0xba { // INVOKEDYNAMIC：附带引导方法，便于阅读 lambda
				out = append(out, insn{pos: start, text: d.invokeDynamicText(idx)})
				break
			}
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s %s", info.name, d.constantText(idx))})
		case opBranch2:
			if !have(2) {
				return truncated()
			}
			target := start + int(int16(binary.BigEndian.Uint16(code[pos:])))
			pos += 2
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s L%d", info.name, target), targets: []int{target}})
		case opBranch4:
			if !have(4) {
				return truncated()
			}
			target := start + int(int32(binary.BigEndian.Uint32(code[pos:])))
			pos += 4
			out = append(out, insn{pos: start, text: fmt.Sprintf("%s L%d", info.name, target), targets: []int{target}})
		case opIInc:
			if !have(2) {
				return truncated()
			}
			out = append(out, insn{pos: start, text: fmt.Sprintf("IINC %d %d", code[pos], int8(code[pos+1]))})
			pos += 2
		case opNewArray:
			if !have(1) {
				return truncated()
			}
			typ := arrayTypeNames[code[pos]]
			if typ == "" {
				typ = fmt.Sprintf("0x%02x", code[pos])
			}
			out = append(out, insn{pos: start, text: "NEWARRAY " + typ})
			pos++
		case opMultiArray:
			if !have(3) {
				return truncated()
			}
			idx := binary.BigEndian.Uint16(code[pos:])
			dims := code[pos+2]
			pos += 3
			out = append(out, insn{pos: start, text: fmt.Sprintf("MULTIANEWARRAY %s %d", d.internalName(idx), dims)})
		case opInvokeIface:
			if !have(4) {
				return truncated()
			}
			idx := binary.BigEndian.Uint16(code[pos:])
			count := code[pos+2]
			pos += 4
			out = append(out, insn{pos: start, text: fmt.Sprintf("INVOKEINTERFACE %s %d", d.operandText(0xb9, idx), count)})
		case opTableSwitch:
			pad := (4 - pos%4) % 4
			if !have(pad + 12) {
				return truncated()
			}
			pos += pad
			def := int32(binary.BigEndian.Uint32(code[pos:]))
			low := int32(binary.BigEndian.Uint32(code[pos+4:]))
			high := int32(binary.BigEndian.Uint32(code[pos+8:]))
			pos += 12
			n := int(high-low) + 1
			if n < 0 || n > maxSwitchTargets {
				return truncated()
			}
			if !have(n * 4) {
				return truncated()
			}
			lines := make([]string, 0, n+1)
			targets := make([]int, 0, n+1)
			for i := 0; i < n; i++ {
				off := int32(binary.BigEndian.Uint32(code[pos:]))
				pos += 4
				t := start + int(off)
				targets = append(targets, t)
				lines = append(lines, fmt.Sprintf("  %d: L%d", low+int32(i), t))
			}
			defTarget := start + int(def)
			targets = append(targets, defTarget)
			lines = append(lines, fmt.Sprintf("  default: L%d", defTarget))
			out = append(out, insn{pos: start, text: "TABLESWITCH\n" + strings.Join(lines, "\n"), targets: targets})
		case opLookupSwitch:
			pad := (4 - pos%4) % 4
			if !have(pad + 8) {
				return truncated()
			}
			pos += pad
			def := int32(binary.BigEndian.Uint32(code[pos:]))
			n := int(int32(binary.BigEndian.Uint32(code[pos+4:])))
			pos += 8
			if n < 0 || n > maxSwitchTargets || !have(n*8) {
				return truncated()
			}
			lines := make([]string, 0, n+1)
			targets := make([]int, 0, n+1)
			for i := 0; i < n; i++ {
				key := int32(binary.BigEndian.Uint32(code[pos:]))
				off := int32(binary.BigEndian.Uint32(code[pos+4:]))
				pos += 8
				t := start + int(off)
				targets = append(targets, t)
				lines = append(lines, fmt.Sprintf("  %d: L%d", key, t))
			}
			defTarget := start + int(def)
			targets = append(targets, defTarget)
			lines = append(lines, fmt.Sprintf("  default: L%d", defTarget))
			out = append(out, insn{pos: start, text: "LOOKUPSWITCH\n" + strings.Join(lines, "\n"), targets: targets})
		case opWide:
			if !have(1) {
				return truncated()
			}
			inner := opcodeTable[code[pos]]
			pos++
			if !have(2) {
				return truncated()
			}
			idx := binary.BigEndian.Uint16(code[pos:])
			pos += 2
			text := fmt.Sprintf("WIDE %s %d", inner.name, idx)
			if code[start+1] == 0x84 { // WIDE IINC：再跟一个 s2 常量
				if !have(2) {
					return truncated()
				}
				text += fmt.Sprintf(" %d", int16(binary.BigEndian.Uint16(code[pos:])))
				pos += 2
			}
			out = append(out, insn{pos: start, text: text})
		default:
			out = append(out, insn{pos: start, text: info.name})
		}
	}
	return out
}

// operandText 渲染字段 / 方法 / 类型类指令的操作数。
// 字段引用用 `owner.name : desc`、方法与类型指令用 `owner.name (desc)` / `owner`，
// 与 ASM Textifier 的可读形态一致。
func (d *disassembler) operandText(opcode byte, idx uint16) string {
	if int(idx) >= len(d.pool) {
		return "#" + strconv.Itoa(int(idx))
	}
	switch opcode {
	case 0xbb, 0xbd, 0xc0, 0xc1: // NEW / ANEWARRAY / CHECKCAST / INSTANCEOF
		return d.internalName(idx)
	}
	owner, name, desc, ok := d.pool.refParts(idx)
	if !ok {
		return d.constantText(idx)
	}
	switch d.pool[idx].tag {
	case 10, 11: // Methodref / InterfaceMethodref
		return fmt.Sprintf("%s.%s (%s)", owner, name, desc)
	default: // Fieldref
		return fmt.Sprintf("%s.%s : %s", owner, name, desc)
	}
}

// invokeDynamicText 渲染 INVOKEDYNAMIC（含引导方法，便于阅读 lambda）。
func (d *disassembler) invokeDynamicText(idx uint16) string {
	name, desc, ok := d.pool.nameAndType(idx2Of(d.pool, idx))
	if !ok {
		return "INVOKEDYNAMIC " + d.constantText(idx)
	}
	head := fmt.Sprintf("INVOKEDYNAMIC %s (%s)", name, desc)
	bsmIdx := idx1Of(d.pool, idx)
	bsm := d.bootstrapMethod(bsmIdx)
	if bsm == nil {
		return head
	}
	parts := []string{head + " [", "  " + d.constantText(bsm.handleIdx)}
	for _, a := range bsm.argIdx {
		parts = append(parts, "  "+d.constantText(a))
	}
	parts = append(parts, "]")
	return strings.Join(parts, "\n")
}

// constantText 渲染常量池条目为可读文本（LDC / 引导方法参数等）。
func (d *disassembler) constantText(idx uint16) string {
	if int(idx) >= len(d.pool) || idx == 0 {
		return "#" + strconv.Itoa(int(idx))
	}
	e := d.pool[idx]
	switch e.tag {
	case 1: // Utf8
		return strconv.Quote(e.utf8)
	case 3: // Integer
		return strconv.Itoa(int(int32(e.num)))
	case 4: // Float
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(e.num))), 'g', -1, 32)
	case 5: // Long
		return strconv.FormatInt(e.num, 10) + "L"
	case 6: // Double
		return strconv.FormatFloat(math.Float64frombits(uint64(e.num)), 'g', -1, 64)
	case 7: // Class
		return "L" + d.internalName(idx) + ";"
	case 8: // String
		return strconv.Quote(d.pool.utf8At(e.idx1))
	case 12: // NameAndType
		name, desc, _ := d.pool.nameAndType(idx)
		return fmt.Sprintf("%s %s", name, desc)
	case 15: // MethodHandle
		kind := methodHandleKindNames[e.handleKind]
		if kind == "" {
			kind = fmt.Sprintf("kind 0x%x", e.handleKind)
		}
		return kind + " " + d.operandText(0xb6, e.idx1)
	case 16: // MethodType
		return d.pool.utf8At(e.idx1)
	case 17, 18: // Dynamic / InvokeDynamic
		name, desc, _ := d.pool.nameAndType(e.idx2)
		return fmt.Sprintf("%s (%s)", name, desc)
	default:
		return "#" + strconv.Itoa(int(idx))
	}
}

// internalName 取 Class 条目的内部名（a/b/C）；非法索引返回空串。
func (d *disassembler) internalName(idx uint16) string {
	if int(idx) >= len(d.pool) || d.pool[idx].tag != 7 {
		return ""
	}
	return d.pool.utf8At(d.pool[idx].idx1)
}

// classRefText 取 Class 条目的内部名，非法时回退为 `#索引`。
func (d *disassembler) classRefText(idx uint16) string {
	if name := d.internalName(idx); name != "" {
		return name
	}
	return "#" + strconv.Itoa(int(idx))
}

// bootstrapMethod BootstrapMethods 条目。
type bootstrapMethod struct {
	handleIdx uint16
	argIdx    []uint16
}

// bootstrapMethod 取第 idx 个引导方法（BootstrapMethods 属性内）。
func (d *disassembler) bootstrapMethod(idx uint16) *bootstrapMethod {
	if d.bootstrapMethods == nil {
		d.bootstrapMethods = parseBootstrapMethods(d.bootstrapAttr)
	}
	if int(idx) >= len(d.bootstrapMethods) {
		return nil
	}
	return &d.bootstrapMethods[idx]
}

// parseBootstrapMethods 解析 BootstrapMethods 属性正文。
func parseBootstrapMethods(body []byte) []bootstrapMethod {
	r := &classReader{b: body}
	n := int(r.u2())
	if r.err != nil {
		return nil
	}
	out := make([]bootstrapMethod, 0, n)
	for i := 0; i < n; i++ {
		var bm bootstrapMethod
		bm.handleIdx = r.u2()
		argCount := int(r.u2())
		bm.argIdx = make([]uint16, 0, argCount)
		for j := 0; j < argCount; j++ {
			bm.argIdx = append(bm.argIdx, r.u2())
		}
		if r.err != nil {
			return out
		}
		out = append(out, bm)
	}
	return out
}

// idx1Of / idx2Of 取常量池条目的 idx1 / idx2（越界返回 0）。
func idx1Of(p cpPool, idx uint16) uint16 {
	if int(idx) >= len(p) {
		return 0
	}
	return p[idx].idx1
}

func idx2Of(p cpPool, idx uint16) uint16 {
	if int(idx) >= len(p) {
		return 0
	}
	return p[idx].idx2
}

// refParts 解析 Fieldref / Methodref / InterfaceMethodref 为 owner(内部名) / name / desc。
func (p cpPool) refParts(idx uint16) (owner, name, desc string, ok bool) {
	if int(idx) >= len(p) {
		return "", "", "", false
	}
	e := p[idx]
	if e.tag != 9 && e.tag != 10 && e.tag != 11 {
		return "", "", "", false
	}
	owner = p.internalNameAt(e.idx1)
	name, desc, ok = p.nameAndType(e.idx2)
	if owner == "" || !ok {
		return "", "", "", false
	}
	return owner, name, desc, true
}

// nameAndType 解析 NameAndType 条目。
func (p cpPool) nameAndType(idx uint16) (name, desc string, ok bool) {
	if int(idx) >= len(p) || p[idx].tag != 12 {
		return "", "", false
	}
	return p.utf8At(p[idx].idx1), p.utf8At(p[idx].idx2), true
}

// internalNameAt 取 Class 条目的内部名。
func (p cpPool) internalNameAt(idx uint16) string {
	if int(idx) >= len(p) || p[idx].tag != 7 {
		return ""
	}
	return p.utf8At(p[idx].idx1)
}
