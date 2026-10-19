package mcsrc

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// 反向引用索引（「查找引用」的数据层）。
//
// 目标：在**不反编译、不依赖 JVM** 的前提下，为「查找引用」提供数据——
// 扫描 jar 内每个 class 的常量池与指令，记录「哪个成员引用了哪个目标」。
//
// 引用键（目标，**点分全限定名**，与 mcsrc 其余接口一致）：
//
//	c:<owner>                       类引用
//	m:<owner>:<name>:<descriptor>   方法引用
//	f:<owner>:<name>:<descriptor>   字段引用
//
// 只索引**目标类位于本 jar 内**的引用：JDK / 第三方库类不在 jar 中，记录它们既无法
// 跳转也没有意义（与上游 mcsrc 仅索引 net/minecraft、com/mojang 的意图一致，但这里
// 用「是否在本 jar 内」判定，对任意 MC 版本 / 侧别都成立）。
//
// 覆盖来源（对齐上游 ASM ClassIndexVisitor 的语义）：
//   - 指令操作数：GETSTATIC/PUTSTATIC/GETFIELD/PUTFIELD → 字段引用；
//     INVOKEVIRTUAL/SPECIAL/STATIC/INTERFACE → 方法引用；
//     NEW/ANEWARRAY/CHECKCAST/INSTANCEOF/MULTIANEWARRAY 与 LDC 类常量 → 类引用；
//   - INVOKEDYNAMIC 引导方法的方法句柄与参数（lambda 目标）→ 成员 / 类引用；
//   - 字段类型与方法的参数 / 返回类型描述符 → 类引用。
//
// 有意未覆盖（v1 显式标注，不静默丢弃）：注解与泛型签名（Signature）中的类型、
// Record / Module 属性——它们不参与「调用 / 引用」语义。
const (
	// maxRefSitesPerKey 单个目标的引用点上限（防御超高频成员撑爆响应）。
	maxRefSitesPerKey = 2000
	// maxRefSitesTotal 单次建索引的引用点总数上限（防御超大 jar 耗尽内存）。
	maxRefSitesTotal = 400000
	// maxRefCacheEntries 引用索引内存缓存保留的版本数（每个需全量扫描，代价高）。
	maxRefCacheEntries = 2
)

// ErrBadReference 引用目标非法（kind / 类名 / 成员名不合法）。
var ErrBadReference = errors.New("引用目标非法")

// ReferenceSite 一个引用点：由某个成员引用了目标。
type ReferenceSite struct {
	// FromClass 引用点所在类（点分全限定名）。
	FromClass string `json:"from_class"`
	// FromKind 引用点所在成员类型：method / field。
	FromKind string `json:"from_kind"`
	// FromName 引用点所在成员名。
	FromName string `json:"from_name"`
	// FromDescriptor 引用点所在成员描述符。
	FromDescriptor string `json:"from_descriptor"`
}

// ReferenceResult 查找引用结果。
type ReferenceResult struct {
	// Key 归一化后的引用键（c: / m: / f:）。
	Key string `json:"key"`
	// Kind 目标类型：class / method / field。
	Kind string `json:"kind"`
	// Owner 目标类（点分全限定名）。
	Owner string `json:"owner"`
	// Name 目标成员名（类引用为空）。
	Name string `json:"name,omitempty"`
	// Descriptor 目标成员描述符（类引用为空）。
	Descriptor string `json:"descriptor,omitempty"`
	// Sites 引用点（按类名、成员类型、成员名、描述符排序，已去重）。
	Sites []ReferenceSite `json:"sites"`
	// Total 引用点总数（去重后；可能大于 len(Sites)，见 Truncated）。
	Total int `json:"total"`
	// Truncated 引用点过多被截断。
	Truncated bool `json:"truncated"`
}

// refCaller 引用点所在成员。
type refCaller struct {
	class string
	kind  string // method / field
	name  string
	desc  string
}

// refIndex 一个 jar 的反向引用索引。
type refIndex struct {
	// jarClasses 本 jar 内的类名集合（点分），用于过滤「目标不在 jar 内」的引用。
	jarClasses map[string]bool
	// refs 引用键 → 引用点集合。
	refs map[string][]refCaller
	// truncated 命中上限而被截断的引用键。
	truncated map[string]bool
	// skipped 解析失败（损坏条目）而被跳过的类数量。
	skipped int
}

// refCache 引用索引缓存项：jar 摘要变化即失效。
type refCache struct {
	sha1 string
	refs *refIndex
}

// refIndexBuilder 建索引期间的累加器。
type refIndexBuilder struct {
	jarClasses map[string]bool
	refs       map[string][]refCaller
	truncated  map[string]bool
	total      int
	skipped    int
}

func (b *refIndexBuilder) inJar(dotted string) bool { return b.jarClasses[dotted] }

// add 追加一个引用点（相邻重复合并，与上游 ReferenceList 的优化一致）。
func (b *refIndexBuilder) add(key string, c refCaller) {
	list := b.refs[key]
	if n := len(list); n > 0 && list[n-1] == c {
		return
	}
	if len(list) >= maxRefSitesPerKey || b.total >= maxRefSitesTotal {
		b.truncated[key] = true
		return
	}
	b.refs[key] = append(list, c)
	b.total++
}

// addClassRef 记录一条类引用（目标不在 jar 内时忽略）。
func (b *refIndexBuilder) addClassRef(c refCaller, dotted string) {
	if dotted == "" || !b.inJar(dotted) {
		return
	}
	b.add("c:"+dotted, c)
}

// addMemberRef 记录一条字段 / 方法引用（依常量池条目 tag 判定类型）。
func (b *refIndexBuilder) addMemberRef(c refCaller, pool cpPool, idx uint16) {
	if int(idx) >= len(pool) {
		return
	}
	tag := pool[idx].tag
	if tag != 9 && tag != 10 && tag != 11 {
		return
	}
	owner, name, desc, ok := pool.refParts(idx)
	if !ok {
		return
	}
	dotted := strings.ReplaceAll(owner, "/", ".")
	if !b.inJar(dotted) {
		return
	}
	if tag == 10 || tag == 11 {
		b.add("m:"+dotted+":"+name+":"+desc, c)
		// 构造器引用同时记一条类引用（对齐上游 indexMethodReference）。
		if name == "<init>" {
			b.addClassRef(c, dotted)
		}
		return
	}
	b.add("f:"+dotted+":"+name+":"+desc, c)
}

// addDescriptorRefs 记录描述符中出现的对象类型（作为类引用）。
func (b *refIndexBuilder) addDescriptorRefs(c refCaller, desc string) {
	for _, name := range descriptorClassRefs(desc) {
		b.addClassRef(c, name)
	}
}

// addHandleRef 记录方法句柄（MethodHandle）指向的成员引用。
func (b *refIndexBuilder) addHandleRef(c refCaller, pool cpPool, idx uint16) {
	if int(idx) >= len(pool) || pool[idx].tag != 15 {
		return
	}
	b.addMemberRef(c, pool, pool[idx].idx1)
}

// addConstantRef 记录引导方法参数（方法句柄 / 方法类型）中的引用。
func (b *refIndexBuilder) addConstantRef(c refCaller, pool cpPool, idx uint16) {
	if int(idx) >= len(pool) {
		return
	}
	switch pool[idx].tag {
	case 15: // MethodHandle
		b.addHandleRef(c, pool, idx)
	case 16: // MethodType
		b.addDescriptorRefs(c, pool.utf8At(pool[idx].idx1))
	}
}

// indexClass 扫描单个 class，登记其字段 / 方法产生的引用。
func (b *refIndexBuilder) indexClass(fc *fullClass) {
	this := strings.ReplaceAll(fc.pool.internalNameAt(fc.ThisIdx), "/", ".")
	if this == "" {
		return
	}
	// BootstrapMethods 属性在类属性表中，INVOKEDYNAMIC（lambda）需要它解析引导方法。
	var bootstraps []bootstrapMethod
	if bm := attrOf(fc.Attrs, "BootstrapMethods"); bm != nil {
		bootstraps = parseBootstrapMethods(bm.body)
	}

	for i := range fc.Fields {
		f := &fc.Fields[i]
		caller := refCaller{class: this, kind: "field", name: fc.pool.utf8At(f.NameIdx), desc: fc.pool.utf8At(f.DescIdx)}
		b.addDescriptorRefs(caller, caller.desc)
	}
	for i := range fc.Methods {
		m := &fc.Methods[i]
		caller := refCaller{class: this, kind: "method", name: fc.pool.utf8At(m.NameIdx), desc: fc.pool.utf8At(m.DescIdx)}
		b.addDescriptorRefs(caller, caller.desc)

		code := attrOf(m.Attrs, "Code")
		if code == nil {
			continue
		}
		body, ok := codeBody(code.body)
		if !ok {
			continue
		}
		b.indexCode(caller, fc.pool, body, bootstraps)
	}
}

// indexCode 扫描方法 Code 正文，登记其中的引用。
func (b *refIndexBuilder) indexCode(c refCaller, pool cpPool, code []byte, bootstraps []bootstrapMethod) {
	for _, ref := range decodeCodeRefs(code) {
		switch ref.opcode {
		case 0xb2, 0xb3, 0xb4, 0xb5, // GETSTATIC / PUTSTATIC / GETFIELD / PUTFIELD
			0xb6, 0xb7, 0xb8, 0xb9: // INVOKEVIRTUAL / SPECIAL / STATIC / INTERFACE
			b.addMemberRef(c, pool, ref.index)
		case 0xbb, 0xbd, 0xc0, 0xc1, 0xc5: // NEW / ANEWARRAY / CHECKCAST / INSTANCEOF / MULTIANEWARRAY
			b.addClassRef(c, strings.ReplaceAll(pool.internalNameAt(ref.index), "/", "."))
		case 0x12, 0x13: // LDC / LDC_W：可能是类常量（Foo.class）
			if int(ref.index) < len(pool) && pool[ref.index].tag == 7 {
				b.addClassRef(c, strings.ReplaceAll(pool.internalNameAt(ref.index), "/", "."))
			}
		case 0xba: // INVOKEDYNAMIC：解析引导方法（lambda 目标）
			bsmIdx := idx1Of(pool, ref.index)
			if int(bsmIdx) >= len(bootstraps) {
				continue
			}
			bsm := bootstraps[bsmIdx]
			b.addHandleRef(c, pool, bsm.handleIdx)
			for _, a := range bsm.argIdx {
				b.addConstantRef(c, pool, a)
			}
		}
	}
}

// codeRef 一条带常量池索引的指令。
type codeRef struct {
	opcode byte
	index  uint16
}

// decodeCodeRefs 扫描 Code 数组，收集所有带常量池索引的指令（不渲染文本）。
// 长度推进规则与 disassembler.decode 保持一致；遇到未知 / 截断的操作码即停止。
func decodeCodeRefs(code []byte) []codeRef {
	out := make([]codeRef, 0, len(code)/2)
	for pos := 0; pos < len(code); {
		op := code[pos]
		info := opcodeTable[op]
		if info.name == "" {
			return out
		}
		pos++
		have := func(n int) bool { return pos+n <= len(code) }
		switch info.kind {
		case opNone:
		case opLocal, opByteConst, opNewArray:
			if !have(1) {
				return out
			}
			pos++
		case opShortConst, opBranch2, opIInc:
			if !have(2) {
				return out
			}
			pos += 2
		case opBranch4:
			if !have(4) {
				return out
			}
			pos += 4
		case opCP1:
			if !have(1) {
				return out
			}
			out = append(out, codeRef{op, uint16(code[pos])})
			pos++
		case opCP2:
			if !have(2) {
				return out
			}
			out = append(out, codeRef{op, binary.BigEndian.Uint16(code[pos:])})
			pos += 2
		case opCP2Pad:
			if !have(4) {
				return out
			}
			out = append(out, codeRef{op, binary.BigEndian.Uint16(code[pos:])})
			pos += 4
		case opMultiArray:
			if !have(3) {
				return out
			}
			out = append(out, codeRef{op, binary.BigEndian.Uint16(code[pos:])})
			pos += 3
		case opInvokeIface:
			if !have(4) {
				return out
			}
			out = append(out, codeRef{op, binary.BigEndian.Uint16(code[pos:])})
			pos += 4
		case opTableSwitch:
			pad := (4 - pos%4) % 4
			if !have(pad + 12) {
				return out
			}
			pos += pad
			low := int32(binary.BigEndian.Uint32(code[pos+4:]))
			high := int32(binary.BigEndian.Uint32(code[pos+8:]))
			pos += 12
			n := int(high-low) + 1
			if n < 0 || !have(n*4) {
				return out
			}
			pos += n * 4
		case opLookupSwitch:
			pad := (4 - pos%4) % 4
			if !have(pad + 8) {
				return out
			}
			pos += pad
			n := int(int32(binary.BigEndian.Uint32(code[pos+4:])))
			pos += 8
			if n < 0 || !have(n*8) {
				return out
			}
			pos += n * 8
		case opWide:
			if !have(1) {
				return out
			}
			inner := opcodeTable[code[pos]]
			pos++
			if !have(2) {
				return out
			}
			pos += 2
			if inner.name == "IINC" { // WIDE IINC：再跟一个 s2 常量
				if !have(2) {
					return out
				}
				pos += 2
			}
		}
	}
	return out
}

// codeBody 从 Code 属性正文取出指令字节数组。
func codeBody(body []byte) ([]byte, bool) {
	if len(body) < 8 {
		return nil, false
	}
	n := binary.BigEndian.Uint32(body[4:8])
	if n > maxCodeAttributeBytes || 8+int(n) > len(body) {
		return nil, false
	}
	return body[8 : 8+int(n)], true
}

// descriptorClassRefs 提取 JVM 描述符中的对象类型（点分全限定名）。
// 例：`(Ljava/lang/String;[Lnet/minecraft/Foo;)V` → [java.lang.String net.minecraft.Foo]。
func descriptorClassRefs(desc string) []string {
	var out []string
	for i := 0; i < len(desc); i++ {
		if desc[i] != 'L' {
			continue
		}
		j := strings.IndexByte(desc[i:], ';')
		if j < 0 {
			break
		}
		out = append(out, strings.ReplaceAll(desc[i+1:i+j], "/", "."))
		i += j
	}
	return out
}

// buildReferences 扫描 jar 内全部 class，构建反向引用索引。
// 单个条目损坏只跳过该条目，不影响整体（真实 jar 偶有异常条目）。
func buildReferences(ctx context.Context, jarPath string) (*refIndex, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, fmt.Errorf("打开 jar 失败: %w", err)
	}
	defer func() { _ = zr.Close() }()

	b := &refIndexBuilder{
		jarClasses: make(map[string]bool, len(zr.File)),
		refs:       make(map[string][]refCaller),
		truncated:  make(map[string]bool),
	}
	// 先收集本 jar 内的类名，供引用过滤（目标不在 jar 内即无法跳转）。
	for _, f := range zr.File {
		if isClassEntry(f.Name) {
			b.jarClasses[strings.ReplaceAll(strings.TrimSuffix(f.Name, ".class"), "/", ".")] = true
		}
	}
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isClassEntry(f.Name) {
			continue
		}
		data, err := readZipEntry(f)
		if err != nil {
			b.skipped++
			continue
		}
		fc, err := parseClassFull(data)
		if err != nil {
			b.skipped++
			continue
		}
		b.indexClass(fc)
	}
	return &refIndex{
		jarClasses: b.jarClasses,
		refs:       b.refs,
		truncated:  b.truncated,
		skipped:    b.skipped,
	}, nil
}

// sites 返回某引用键的去重、排序后的引用点。
func (r *refIndex) sites(key string) []ReferenceSite {
	list := r.refs[key]
	seen := make(map[refCaller]bool, len(list))
	out := make([]ReferenceSite, 0, len(list))
	for _, c := range list {
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, ReferenceSite{
			FromClass:      c.class,
			FromKind:       c.kind,
			FromName:       c.name,
			FromDescriptor: c.desc,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.FromClass != b.FromClass {
			return a.FromClass < b.FromClass
		}
		if a.FromKind != b.FromKind {
			return a.FromKind < b.FromKind
		}
		if a.FromName != b.FromName {
			return a.FromName < b.FromName
		}
		return a.FromDescriptor < b.FromDescriptor
	})
	return out
}

// referencesFor 取（必要时构建并缓存）反向引用索引。
func (s *Store) referencesFor(ctx context.Context, version, side, sha1sum string) (*refIndex, error) {
	key := version + "-" + side
	s.refMu.Lock()
	if c, ok := s.ref[key]; ok && c.sha1 == sha1sum {
		refs := c.refs
		s.refMu.Unlock()
		return refs, nil
	}
	s.refMu.Unlock()

	// 单飞：同一目标的扫描只做一次。
	lock := s.lockFor(key + "-ref")
	lock.Lock()
	defer lock.Unlock()
	s.refMu.Lock()
	if c, ok := s.ref[key]; ok && c.sha1 == sha1sum {
		refs := c.refs
		s.refMu.Unlock()
		return refs, nil
	}
	s.refMu.Unlock()

	refs, err := buildReferences(ctx, s.jarPath(version, side))
	if err != nil {
		return nil, err
	}
	s.refMu.Lock()
	if s.ref == nil {
		s.ref = make(map[string]refCache)
	}
	// 引用索引体积较大，只保留最近若干个版本。
	if len(s.ref) >= maxRefCacheEntries {
		for k := range s.ref {
			delete(s.ref, k)
			break
		}
	}
	s.ref[key] = refCache{sha1: sha1sum, refs: refs}
	s.refMu.Unlock()
	return refs, nil
}

// References 查找对某个类 / 方法 / 字段的引用。
// kind 取 class / method / field；method / field 需提供 name（与 descriptor）。
func (s *Store) References(ctx context.Context, version, side, kind, owner, name, descriptor string) (*ReferenceResult, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return nil, err
	}
	res, key, err := referenceKey(kind, owner, name, descriptor)
	if err != nil {
		return nil, err
	}

	st := s.statusLocked(v, sd)
	if !st.Cached {
		return nil, ErrNotFound
	}
	if !st.Indexed {
		return nil, ErrNotIndexed
	}

	idx, err := s.referencesFor(ctx, v, sd, st.SHA1)
	if err != nil {
		return nil, err
	}
	if !idx.jarClasses[res.Owner] {
		return nil, ErrClassNotFound
	}

	sites := idx.sites(key)
	res.Key = key
	res.Sites = sites
	res.Total = len(sites)
	res.Truncated = idx.truncated[key]
	return res, nil
}

// referenceKey 校验并归一化查找引用参数，返回结果骨架与引用键。
func referenceKey(kind, owner, name, descriptor string) (*ReferenceResult, string, error) {
	ownerName, err := normalizeClassName(owner)
	if err != nil {
		return nil, "", ErrBadReference
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "class":
		return &ReferenceResult{Kind: "class", Owner: ownerName}, "c:" + ownerName, nil
	case "method", "field":
		n := strings.TrimSpace(name)
		if n == "" {
			return nil, "", ErrBadReference
		}
		d := strings.TrimSpace(descriptor)
		k := strings.ToLower(strings.TrimSpace(kind))
		prefix := "m:"
		if k == "field" {
			prefix = "f:"
		}
		res := &ReferenceResult{Kind: k, Owner: ownerName, Name: n, Descriptor: d}
		return res, prefix + ownerName + ":" + n + ":" + d, nil
	default:
		return nil, "", ErrBadReference
	}
}
