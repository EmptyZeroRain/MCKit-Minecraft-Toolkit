package mcsrc

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

// 类结构视图与继承关系。
//
// 全部基于 class 字节码解析（classfile.go），**不反编译、不依赖 JVM**：
//   - ClassStructure 只读单个 class 条目 + zip 中央目录（判断父类是否在本 jar 内），
//     因此与 jar 大小无关，单次毫秒级；
//   - Hierarchy 需要全量扫描本 jar 内所有类的「父类 / 接口」关系，因此
//     按 (version, side, jar 摘要) 在内存缓存关系索引，命中后为零扫描。

const (
	// maxClassBytes 单个 class 条目解析上限（防御畸形 zip 撑爆内存）。
	maxClassBytes = 16 << 20
	// maxDescendants 向下直接子类的返回上限。
	maxDescendants = 500
	// maxTotalDescendants 递归子类统计上限（防御极宽继承树）。
	maxTotalDescendants = 5000
	// maxAncestorDepth 祖先链深度上限（防御畸形数据成环）。
	maxAncestorDepth = 64
)

var (
	// ErrClassNotFound 该类不在该版本的 jar 中。
	ErrClassNotFound = errors.New("类不存在")
	// ErrBadClass 类名非法。
	ErrBadClass = errors.New("类名非法")
	// ErrClassTooLarge class 条目超出解析上限。
	ErrClassTooLarge = errors.New("class 文件超出解析上限")
)

// classPattern 类名白名单：字母数字 / 下划线 / $ / 点，且以字母或下划线开头。
var classPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)*$`)

// Member 结构视图中的字段 / 方法（描述符已渲染为 Java 类型）。
type Member struct {
	Name      string   `json:"name"`
	Modifiers []string `json:"modifiers"`
	// Type 字段类型或方法返回类型。
	Type string `json:"type"`
	// Params 方法参数类型；字段为 null（omitempty）。
	Params []string `json:"params,omitempty"`
	// Descriptor 原始 JVM 描述符，便于对照。
	Descriptor string `json:"descriptor"`
	// Synthetic 是否为编译器生成的成员（bridge / synthetic / 内部类访问器）。
	Synthetic bool `json:"synthetic"`
}

// ClassStructure 单个类的结构视图。
type ClassStructure struct {
	Name      string   `json:"name"`
	Simple    string   `json:"simple"`
	Pkg       string   `json:"pkg"`
	Kind      string   `json:"kind"`
	Modifiers []string `json:"modifiers"`
	// Major / Minor class 文件版本（major 52 = Java 8，50 = Java 6）。
	Major int    `json:"major"`
	Minor int    `json:"minor"`
	Super string `json:"super"`
	// InJarSuper 父类是否也在本 jar 内；false 表示 JDK / 第三方库类。
	InJarSuper bool `json:"in_jar_super"`
	// Interfaces 全部直接接口。
	Interfaces []string `json:"interfaces"`
	// InJarInterfaces 直接接口中位于本 jar 内的部分（可继续下钻）。
	InJarInterfaces []string `json:"in_jar_interfaces"`
	Fields          []Member `json:"fields"`
	Methods         []Member `json:"methods"`
	// Inner 是否为内部类（简单名含 $）。
	Inner bool `json:"inner"`
}

// Descendant 子类条目。
type Descendant struct {
	Name   string `json:"name"`
	Simple string `json:"simple"`
	Inner  bool   `json:"inner"`
}

// Hierarchy 继承关系：向上祖先链 + 向下直接子类。
type Hierarchy struct {
	Name       string   `json:"name"`
	Super      string   `json:"super"`
	Interfaces []string `json:"interfaces"`
	// Ancestors 祖先链：直接父类起逐级向上，含跳出本 jar 的终点（如 java.lang.Object）。
	Ancestors []string `json:"ancestors"`
	// AncestorsInJar 祖先链是否全程在本 jar 内（false 表示祖先链断在 jar 外）。
	AncestorsInJar bool `json:"ancestors_in_jar"`
	// Descendants 直接子类（含实现本接口的类）。
	Descendants []Descendant `json:"descendants"`
	// DescendantCount 直接子类总数（可能大于 len(Descendants)，见 Truncated）。
	DescendantCount int `json:"descendant_count"`
	// TotalDescendants 递归子类总数（含间接）。
	TotalDescendants int `json:"total_descendants"`
	// Truncated 直接子类列表被截断。
	Truncated bool `json:"truncated"`
}

// relations 一个 jar 内的继承关系索引（父类 / 接口 / 直接子类）。
type relations struct {
	// sup 类名 → 父类名（空串表示 java.lang.Object 自身）。
	sup map[string]string
	// ifaces 类名 → 直接接口。
	ifaces map[string][]string
	// children 类 / 接口名 → 直接子类或实现类。
	children map[string][]string
	// inner 类名 → 是否内部类。
	inner map[string]bool
	// skipped 解析失败（损坏条目）而被跳过的类数量。
	skipped int
}

// relCache 关系索引缓存项：jar 摘要变化即失效。
type relCache struct {
	sha1 string
	rels *relations
}

// normalizeClassName 校验并归一化类名（接受点分或斜杠分隔）。
func normalizeClassName(name string) (string, error) {
	n := strings.TrimSpace(strings.ReplaceAll(name, "/", "."))
	if n == "" {
		return "", ErrBadClass
	}
	if len(n) > 255 || !classPattern.MatchString(n) {
		return "", ErrBadClass
	}
	return n, nil
}

// ClassStructure 返回单个类的结构视图。
func (s *Store) ClassStructure(version, side, className string) (*ClassStructure, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return nil, err
	}
	name, err := normalizeClassName(className)
	if err != nil {
		return nil, err
	}

	// 未缓存 jar 与未建索引同样视为「尚未准备」：结构视图依赖 jar 本体。
	if st := s.statusLocked(v, sd); !st.Cached {
		return nil, ErrNotFound
	}

	zr, err := zip.OpenReader(s.jarPath(v, sd))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("打开 jar 失败: %w", err)
	}
	defer func() { _ = zr.Close() }()

	data, err := readClassEntry(zr, name)
	if err != nil {
		return nil, err
	}
	cf, err := ParseClassFile(data)
	if err != nil {
		return nil, err
	}

	// 父类 / 接口是否也在本 jar 内：只需查 zip 中央目录，无需扫描全部类。
	inJar := func(cn string) bool {
		return entryExists(zr, cn)
	}
	pkg, simple := splitClassName(name)
	st := &ClassStructure{
		Name:       name,
		Simple:     simple,
		Pkg:        pkg,
		Kind:       cf.Kind(),
		Modifiers:  ClassModifiers(cf.AccessFlags),
		Major:      int(cf.Major),
		Minor:      int(cf.Minor),
		Super:      cf.SuperClass,
		InJarSuper: cf.SuperClass != "java.lang.Object" && inJar(cf.SuperClass),
		Interfaces: append([]string{}, cf.Interfaces...),
		Inner:      strings.Contains(simple, "$"),
		Fields:     renderMembers(cf.Fields, false),
		Methods:    renderMembers(cf.Methods, true),
	}
	st.InJarInterfaces = make([]string, 0, len(cf.Interfaces))
	for _, i := range cf.Interfaces {
		if inJar(i) {
			st.InJarInterfaces = append(st.InJarInterfaces, i)
		}
	}
	return st, nil
}

// Hierarchy 返回指定类的继承关系（祖先链 + 直接子类）。
func (s *Store) Hierarchy(ctx context.Context, version, side, className string) (*Hierarchy, error) {
	v, sd, err := normalize(version, side)
	if err != nil {
		return nil, err
	}
	name, err := normalizeClassName(className)
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

	rels, err := s.relationsFor(ctx, v, sd, st.SHA1)
	if err != nil {
		return nil, err
	}
	if !rels.known(name) {
		return nil, ErrClassNotFound
	}

	h := &Hierarchy{
		Name:           name,
		Super:          rels.sup[name],
		Interfaces:     append([]string{}, rels.ifaces[name]...),
		AncestorsInJar: true,
	}
	if h.Super == "" {
		h.Super = "java.lang.Object"
	}

	// 向上：逐级父类，直到跳出本 jar（含终点）或达到深度上限。
	seen := map[string]bool{name: true}
	cur := rels.sup[name]
	for depth := 0; cur != "" && depth < maxAncestorDepth; depth++ {
		h.Ancestors = append(h.Ancestors, cur)
		if seen[cur] {
			break // 畸形数据成环，停止并保留已收集链路
		}
		seen[cur] = true
		if !rels.known(cur) {
			h.AncestorsInJar = false
			break
		}
		cur = rels.sup[cur]
	}
	if h.Ancestors == nil {
		h.Ancestors = []string{}
	}

	// 向下：直接子类（含实现类），按全限定名升序。
	direct := rels.children[name]
	h.DescendantCount = len(direct)
	h.TotalDescendants = rels.countDescendants(name, seen)
	limit := len(direct)
	if limit > maxDescendants {
		limit = maxDescendants
		h.Truncated = true
	}
	h.Descendants = make([]Descendant, 0, limit)
	for _, c := range direct[:limit] {
		_, simple := splitClassName(c)
		h.Descendants = append(h.Descendants, Descendant{
			Name:   c,
			Simple: simple,
			Inner:  rels.inner[c],
		})
	}
	return h, nil
}

// known 类是否存在于本 jar。
func (r *relations) known(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.sup[name]
	return ok
}

// countDescendants 递归统计子类总数（含间接），带环保护与上限。
func (r *relations) countDescendants(root string, seen map[string]bool) int {
	visited := map[string]bool{root: true}
	for k := range seen { // 复用祖先链已访问集合，避免把祖先也算进子类
		visited[k] = true
	}
	queue := append([]string{}, r.children[root]...)
	total := 0
	for len(queue) > 0 && total < maxTotalDescendants {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		total++
		queue = append(queue, r.children[cur]...)
	}
	return total
}

// relationsFor 取（必要时构建并缓存）继承关系索引。
func (s *Store) relationsFor(ctx context.Context, version, side, sha1sum string) (*relations, error) {
	key := version + "-" + side
	s.relMu.Lock()
	if c, ok := s.rel[key]; ok && c.sha1 == sha1sum {
		rels := c.rels
		s.relMu.Unlock()
		return rels, nil
	}
	s.relMu.Unlock()

	// 单飞：同一目标的扫描只做一次。
	lock := s.lockFor(key + "-rel")
	lock.Lock()
	defer lock.Unlock()
	// 二次检查（可能已被其它协程填充）。
	s.relMu.Lock()
	if c, ok := s.rel[key]; ok && c.sha1 == sha1sum {
		rels := c.rels
		s.relMu.Unlock()
		return rels, nil
	}
	s.relMu.Unlock()

	rels, err := buildRelations(ctx, s.jarPath(version, side))
	if err != nil {
		return nil, err
	}
	s.relMu.Lock()
	if s.rel == nil {
		s.rel = make(map[string]relCache)
	}
	// 只保留最近若干个版本的索引，避免长期运行累积内存。
	if len(s.rel) >= 4 {
		for k := range s.rel {
			delete(s.rel, k)
			break
		}
	}
	s.rel[key] = relCache{sha1: sha1sum, rels: rels}
	s.relMu.Unlock()
	return rels, nil
}

// buildRelations 扫描 jar 内全部 class 的父类 / 接口关系。
// 单个条目损坏只跳过该条目，不影响整体（真实 jar 偶有异常条目）。
func buildRelations(ctx context.Context, jarPath string) (*relations, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, fmt.Errorf("打开 jar 失败: %w", err)
	}
	defer func() { _ = zr.Close() }()

	rels := &relations{
		sup:      make(map[string]string, len(zr.File)),
		ifaces:   make(map[string][]string),
		children: make(map[string][]string),
		inner:    make(map[string]bool),
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
			rels.skipped++
			continue
		}
		cf, err := ParseClassFile(data)
		if err != nil {
			rels.skipped++
			continue
		}
		name := cf.ThisClass
		if name == "" {
			rels.skipped++
			continue
		}
		super := cf.SuperClass
		if super == "java.lang.Object" && cf.ThisClass == "java.lang.Object" {
			super = ""
		}
		rels.sup[name] = super
		_, simple := splitClassName(name)
		rels.inner[name] = strings.Contains(simple, "$")
		if super != "" {
			rels.children[super] = append(rels.children[super], name)
		}
		if len(cf.Interfaces) > 0 {
			rels.ifaces[name] = append([]string{}, cf.Interfaces...)
			for _, i := range cf.Interfaces {
				rels.children[i] = append(rels.children[i], name)
			}
		}
	}
	for k := range rels.children {
		sort.Strings(rels.children[k])
	}
	return rels, nil
}

// isClassEntry 是否为可索引的 class 条目（与 BuildIndex 口径一致）。
func isClassEntry(name string) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".class") {
		return false
	}
	if strings.HasPrefix(name, "META-INF/") || strings.HasSuffix(name, "module-info.class") {
		return false
	}
	return len(name) > len(".class")
}

// entryPath 类名 → zip 条目路径。
func entryPath(className string) string {
	return strings.ReplaceAll(className, ".", "/") + ".class"
}

// entryExists 判断 zip 中是否存在该类的条目。
func entryExists(zr *zip.ReadCloser, className string) bool {
	want := entryPath(className)
	for _, f := range zr.File {
		if f.Name == want {
			return true
		}
	}
	return false
}

// readClassEntry 读取指定类名的 class 字节。
func readClassEntry(zr *zip.ReadCloser, className string) ([]byte, error) {
	want := entryPath(className)
	for _, f := range zr.File {
		if f.Name != want {
			continue
		}
		return readZipEntry(f)
	}
	return nil, ErrClassNotFound
}

// readZipEntry 读取单个 zip 条目（带上限）。
func readZipEntry(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxClassBytes {
		return nil, ErrClassTooLarge
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, maxClassBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxClassBytes {
		return nil, ErrClassTooLarge
	}
	return data, nil
}

// renderMembers 把原始成员渲染为结构视图成员。
func renderMembers(list []MemberInfo, method bool) []Member {
	out := make([]Member, 0, len(list))
	for _, m := range list {
		var mods []string
		var typ string
		var params []string
		if method {
			mods = MethodModifiers(m.AccessFlags)
			ps, ret, err := MethodSignature(m.Descriptor)
			if err != nil {
				// 描述符异常：保留原始描述符，类型留空，避免整次请求失败。
				typ = ""
			} else {
				params = ps
				typ = ret
			}
		} else {
			mods = FieldModifiers(m.AccessFlags)
			if t, err := FieldType(m.Descriptor); err == nil {
				typ = t
			}
		}
		out = append(out, Member{
			Name:       m.Name,
			Modifiers:  mods,
			Type:       typ,
			Params:     params,
			Descriptor: m.Descriptor,
			Synthetic:  m.AccessFlags&accSynthetic != 0 || m.AccessFlags&accBridge != 0,
		})
	}
	return out
}
