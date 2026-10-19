package mcsrc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// baseClasses 构造一棵测试继承树：
//
//	java.lang.Object
//	  ├─ a.Base (abstract)
//	       ├─ a.Mid
//	       │    └─ a.Leaf (final)
//	       └─ a.Side
//	  └─ a.Impl ── implements ── a.Iface
func baseClasses(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		classPath("a.Base"): buildClass(t, classSpec{
			Name: "a.Base", Super: "java.lang.Object", Access: accPublic | accAbstract,
			Methods: []memberSpec{{Access: accPublic | accAbstract, Name: "run", Desc: "()V"}},
		}),
		classPath("a.Mid"): buildClass(t, classSpec{
			Name: "a.Mid", Super: "a.Base", Access: accPublic,
			Fields:  []memberSpec{{Access: accPrivate, Name: "value", Desc: "J"}},
			Methods: []memberSpec{{Access: accPublic, Name: "run", Desc: "()V"}},
		}),
		classPath("a.Leaf"): buildClass(t, classSpec{
			Name: "a.Leaf", Super: "a.Mid", Access: accPublic | accFinal,
		}),
		classPath("a.Side"): buildClass(t, classSpec{
			Name: "a.Side", Super: "a.Base", Access: accPublic,
		}),
		classPath("a.Iface"): buildClass(t, classSpec{
			Name: "a.Iface", Access: accPublic | accInterface | accAbstract,
			Methods: []memberSpec{{Access: accPublic | accAbstract, Name: "go", Desc: "(I)La/Base;"}},
		}),
		classPath("a.Impl"): buildClass(t, classSpec{
			Name: "a.Impl", Super: "java.lang.Object", Interfaces: []string{"a.Iface"}, Access: accPublic,
		}),
	}
}

func TestClassStructure(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, baseClasses(t), true)

	st, err := s.ClassStructure("1.0", "client", "a.Mid")
	if err != nil {
		t.Fatalf("结构视图失败: %v", err)
	}
	if st.Name != "a.Mid" || st.Simple != "Mid" || st.Pkg != "a" {
		t.Errorf("类名拆分异常: %+v", st)
	}
	if st.Kind != "class" || st.Major != 52 || st.Inner {
		t.Errorf("种类 / 版本异常: kind=%s major=%d inner=%v", st.Kind, st.Major, st.Inner)
	}
	if st.Super != "a.Base" || !st.InJarSuper {
		t.Errorf("父类应为本 jar 内的 a.Base: super=%q inJar=%v", st.Super, st.InJarSuper)
	}
	if len(st.Interfaces) != 0 || len(st.InJarInterfaces) != 0 {
		t.Errorf("a.Mid 不应有接口: %+v / %+v", st.Interfaces, st.InJarInterfaces)
	}
	if len(st.Fields) != 1 {
		t.Fatalf("字段数量异常: %+v", st.Fields)
	}
	f := st.Fields[0]
	if f.Name != "value" || f.Type != "long" || !reflect.DeepEqual(f.Modifiers, []string{"private"}) {
		t.Errorf("字段渲染异常: %+v", f)
	}
	if f.Params != nil {
		t.Errorf("字段不应有参数: %+v", f.Params)
	}
	if len(st.Methods) != 1 || st.Methods[0].Name != "run" || st.Methods[0].Type != "void" {
		t.Errorf("方法渲染异常: %+v", st.Methods)
	}

	// 斜杠写法与点分写法等价。
	st2, err := s.ClassStructure("1.0", "client", "a/Mid")
	if err != nil || st2.Name != "a.Mid" {
		t.Fatalf("斜杠写法应等价: err=%v name=%v", err, st2)
	}

	// 接口：Kind=interface，接口方法带 abstract。
	iface, err := s.ClassStructure("1.0", "client", "a.Iface")
	if err != nil {
		t.Fatalf("接口结构视图失败: %v", err)
	}
	if iface.Kind != "interface" {
		t.Errorf("Kind 期望 interface，实际 %s", iface.Kind)
	}
	if len(iface.Methods) != 1 || iface.Methods[0].Type != "a.Base" ||
		!reflect.DeepEqual(iface.Methods[0].Params, []string{"int"}) {
		t.Errorf("接口方法渲染异常: %+v", iface.Methods)
	}

	// a.Impl：接口在本 jar 内 → 可下钻。
	impl, err := s.ClassStructure("1.0", "client", "a.Impl")
	if err != nil {
		t.Fatalf("a.Impl 结构视图失败: %v", err)
	}
	if impl.Super != "java.lang.Object" || impl.InJarSuper {
		t.Errorf("a.Impl 父类应为 jar 外的 java.lang.Object: %+v", impl)
	}
	if !reflect.DeepEqual(impl.Interfaces, []string{"a.Iface"}) ||
		!reflect.DeepEqual(impl.InJarInterfaces, []string{"a.Iface"}) {
		t.Errorf("接口字段异常: %+v / %+v", impl.Interfaces, impl.InJarInterfaces)
	}
}

func TestClassStructureErrors(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	classes := baseClasses(t)
	seedJar(t, s, "1.0", SideClient, classes, true)

	// 类不存在。
	if _, err := s.ClassStructure("1.0", "client", "a.Nope"); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("期望 ErrClassNotFound，实际 %v", err)
	}
	// 类名非法（空段）。
	if _, err := s.ClassStructure("1.0", "client", "a..B"); !errors.Is(err, ErrBadClass) {
		t.Errorf("期望 ErrBadClass，实际 %v", err)
	}
	if _, err := s.ClassStructure("1.0", "client", "  "); !errors.Is(err, ErrBadClass) {
		t.Errorf("空类名期望 ErrBadClass，实际 %v", err)
	}
	// 版本未准备。
	if _, err := s.ClassStructure("9.9", "client", "a.Mid"); !errors.Is(err, ErrNotFound) {
		t.Errorf("期望 ErrNotFound，实际 %v", err)
	}
	// 版本号非法 / 侧别非法。
	if _, err := s.ClassStructure("../etc/passwd", "client", "a.Mid"); !errors.Is(err, ErrBadVersion) {
		t.Errorf("期望 ErrBadVersion，实际 %v", err)
	}
	if _, err := s.ClassStructure("1.0", "bogus", "a.Mid"); !errors.Is(err, ErrBadSide) {
		t.Errorf("期望 ErrBadSide，实际 %v", err)
	}

	// 已缓存 jar 但未建索引：结构视图不依赖索引，仍可用；继承关系要求索引。
	seedJar(t, s, "2.0", SideClient, classes, false)
	if _, err := s.ClassStructure("2.0", "client", "a.Mid"); err != nil {
		t.Errorf("结构视图不应依赖类索引: %v", err)
	}
	if _, err := s.Hierarchy(context.Background(), "2.0", "client", "a.Mid"); !errors.Is(err, ErrNotIndexed) {
		t.Errorf("继承关系期望 ErrNotIndexed，实际 %v", err)
	}
}

func TestClassStructureRejectsBrokenClassBytes(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, map[string][]byte{
		classPath("a.Broken"): []byte("not-a-class-file"),
	}, true)

	_, err := s.ClassStructure("1.0", "client", "a.Broken")
	if !errors.Is(err, ErrBadClassFile) {
		t.Fatalf("损坏 class 期望 ErrBadClassFile，实际 %v", err)
	}
	// 关系索引构建时损坏条目应被跳过而非整体失败。
	rels, err := buildRelations(context.Background(), s.jarPath("1.0", SideClient))
	if err != nil {
		t.Fatalf("关系索引不应因单条损坏而失败: %v", err)
	}
	if rels.skipped != 1 || rels.known("a.Broken") {
		t.Errorf("损坏条目应被跳过: skipped=%d known=%v", rels.skipped, rels.known("a.Broken"))
	}
}

func TestHierarchy(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, baseClasses(t), true)
	ctx := context.Background()

	// 祖先链：a.Leaf → a.Mid → a.Base → java.lang.Object（跳出 jar）。
	h, err := s.Hierarchy(ctx, "1.0", "client", "a.Leaf")
	if err != nil {
		t.Fatalf("继承关系失败: %v", err)
	}
	if !reflect.DeepEqual(h.Ancestors, []string{"a.Mid", "a.Base", "java.lang.Object"}) {
		t.Errorf("祖先链异常: %+v", h.Ancestors)
	}
	if h.AncestorsInJar {
		t.Error("祖先链已跳出 jar，AncestorsInJar 应为 false")
	}
	if h.DescendantCount != 0 || h.TotalDescendants != 0 || len(h.Descendants) != 0 {
		t.Errorf("a.Leaf 不应有子类: %+v", h)
	}

	// 向下：a.Base 的直接子类与递归总数。
	h, err = s.Hierarchy(ctx, "1.0", "client", "a.Base")
	if err != nil {
		t.Fatalf("继承关系失败: %v", err)
	}
	names := make([]string, 0, len(h.Descendants))
	for _, d := range h.Descendants {
		names = append(names, d.Name)
	}
	if !reflect.DeepEqual(names, []string{"a.Mid", "a.Side"}) {
		t.Errorf("直接子类应升序为 [a.Mid a.Side]，实际 %+v", names)
	}
	if h.DescendantCount != 2 || h.TotalDescendants != 3 || h.Truncated {
		t.Errorf("子类计数异常: %+v", h)
	}
	if h.Super != "java.lang.Object" || h.AncestorsInJar {
		t.Errorf("a.Base 祖先信息异常: %+v", h)
	}

	// 接口的子类 = 实现类。
	h, err = s.Hierarchy(ctx, "1.0", "client", "a.Iface")
	if err != nil {
		t.Fatalf("接口继承关系失败: %v", err)
	}
	if len(h.Descendants) != 1 || h.Descendants[0].Name != "a.Impl" || h.TotalDescendants != 1 {
		t.Errorf("接口实现类异常: %+v", h)
	}

	// 类不存在。
	if _, err := s.Hierarchy(ctx, "1.0", "client", "java.lang.Object"); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("jar 外类期望 ErrClassNotFound，实际 %v", err)
	}
}

func TestHierarchyRelationsCacheInvalidatedByJarChange(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, baseClasses(t), true)
	ctx := context.Background()

	if _, err := s.Hierarchy(ctx, "1.0", "client", "a.Base"); err != nil {
		t.Fatalf("首次继承关系失败: %v", err)
	}

	// 换成另一棵树（jar 摘要变化）→ 缓存必须失效，重新扫描。
	changed := map[string][]byte{
		classPath("a.Base"): buildClass(t, classSpec{Name: "a.Base", Super: "java.lang.Object", Access: accPublic}),
		classPath("a.New"):  buildClass(t, classSpec{Name: "a.New", Super: "a.Base", Access: accPublic}),
	}
	seedJar(t, s, "1.0", SideClient, changed, true)

	h, err := s.Hierarchy(ctx, "1.0", "client", "a.Base")
	if err != nil {
		t.Fatalf("变更后继承关系失败: %v", err)
	}
	if len(h.Descendants) != 1 || h.Descendants[0].Name != "a.New" {
		t.Fatalf("jar 变更后关系索引应失效重建: %+v", h.Descendants)
	}
}

func TestHierarchyDescendantTruncation(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	classes := map[string][]byte{
		classPath("a.Root"): buildClass(t, classSpec{Name: "a.Root", Super: "java.lang.Object", Access: accPublic}),
	}
	// 造出超过上限的直接子类，验证截断标记与总数仍准确。
	const extra = maxDescendants + 3
	for i := 0; i < extra; i++ {
		name := "a.C" + padIndex(i)
		classes[classPath(name)] = buildClass(t, classSpec{Name: name, Super: "a.Root", Access: accPublic})
	}
	seedJar(t, s, "1.0", SideClient, classes, true)

	h, err := s.Hierarchy(context.Background(), "1.0", "client", "a.Root")
	if err != nil {
		t.Fatalf("继承关系失败: %v", err)
	}
	if !h.Truncated {
		t.Error("超过上限应标记 Truncated")
	}
	if h.DescendantCount != extra || h.TotalDescendants != extra {
		t.Errorf("计数应为 %d，实际 direct=%d total=%d", extra, h.DescendantCount, h.TotalDescendants)
	}
	if len(h.Descendants) != maxDescendants {
		t.Errorf("列表应截断到 %d，实际 %d", maxDescendants, len(h.Descendants))
	}
}

// padIndex 生成定长序号，保证类名有序可比。
func padIndex(i int) string { return fmt.Sprintf("%04d", i) }
