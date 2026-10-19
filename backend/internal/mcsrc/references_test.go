package mcsrc

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// refClasses 构造一组带常量池引用的测试类：
//
//	a.Target  ← a.Caller 通过字段 / 方法 / 类 / 构造器引用
//	a.Iface   ← a.Impl 通过接口方法引用
//	a.Descr   ← 通过字段 / 方法描述符引用 a.Target
//	a.External ← 引用 jar 外的 java.lang.String（应被忽略）
func refClasses(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		classPath("a.Target"): buildClass(t, classSpec{
			Name: "a.Target", Super: "java.lang.Object", Access: accPublic,
			Fields:  []memberSpec{{Access: accPrivate, Name: "v", Desc: "I"}},
			Methods: []memberSpec{{Access: accPublic, Name: "foo", Desc: "()V"}},
		}),
		classPath("a.Caller"): buildClass(t, classSpec{
			Name: "a.Caller", Super: "java.lang.Object", Access: accPublic,
			Methods: []memberSpec{{Access: accPublic, Name: "call", Desc: "()V", Refs: []refSpec{
				{Kind: "field", Owner: "a.Target", Name: "v", Desc: "I"},
				{Kind: "method", Owner: "a.Target", Name: "foo", Desc: "()V"},
				{Kind: "method", Owner: "a.Target", Name: "<init>", Desc: "()V"},
				{Kind: "class", Owner: "a.Target"},
				// jar 外类：不应进入索引。
				{Kind: "method", Owner: "java.lang.String", Name: "length", Desc: "()I"},
			}}},
		}),
		classPath("a.Iface"): buildClass(t, classSpec{
			Name: "a.Iface", Access: accPublic | accInterface | accAbstract,
			Methods: []memberSpec{{Access: accPublic | accAbstract, Name: "go", Desc: "()V"}},
		}),
		classPath("a.Impl"): buildClass(t, classSpec{
			Name: "a.Impl", Super: "java.lang.Object", Interfaces: []string{"a.Iface"}, Access: accPublic,
			Methods: []memberSpec{{Access: accPublic, Name: "call", Desc: "()V", Refs: []refSpec{
				{Kind: "iface", Owner: "a.Iface", Name: "go", Desc: "()V"},
			}}},
		}),
		classPath("a.Descr"): buildClass(t, classSpec{
			Name: "a.Descr", Super: "java.lang.Object", Access: accPublic,
			Fields:  []memberSpec{{Access: accPublic, Name: "t", Desc: "La/Target;"}},
			Methods: []memberSpec{{Access: accPublic, Name: "get", Desc: "()La/Target;"}},
		}),
		classPath("a.External"): buildClass(t, classSpec{
			Name: "a.External", Super: "java.lang.Object", Access: accPublic,
			Methods: []memberSpec{{Access: accPublic, Name: "call", Desc: "()V", Refs: []refSpec{
				{Kind: "method", Owner: "java.lang.String", Name: "length", Desc: "()I"},
			}}},
		}),
	}
}

func TestReferencesClass(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, refClasses(t), true)

	res, err := s.References(context.Background(), "1.0", "client", "class", "a.Target", "", "")
	if err != nil {
		t.Fatalf("查找类引用失败: %v", err)
	}
	if res.Key != "c:a.Target" || res.Kind != "class" || res.Owner != "a.Target" {
		t.Fatalf("引用键异常: %+v", res)
	}

	// 期望引用点：a.Caller.call（字段 / 方法 / 构造器 / 类指令）、a.Descr.t 与 a.Descr.get（描述符）。
	want := []ReferenceSite{
		{FromClass: "a.Caller", FromKind: "method", FromName: "call", FromDescriptor: "()V"},
		{FromClass: "a.Descr", FromKind: "field", FromName: "t", FromDescriptor: "La/Target;"},
		{FromClass: "a.Descr", FromKind: "method", FromName: "get", FromDescriptor: "()La/Target;"},
	}
	if !reflect.DeepEqual(res.Sites, want) {
		t.Fatalf("类引用点异常:\n got %+v\nwant %+v", res.Sites, want)
	}
	if res.Total != len(want) || res.Truncated {
		t.Fatalf("计数异常: total=%d truncated=%v", res.Total, res.Truncated)
	}
}

func TestReferencesMethodAndField(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, refClasses(t), true)
	ctx := context.Background()

	// 方法引用。
	res, err := s.References(ctx, "1.0", "client", "method", "a.Target", "foo", "()V")
	if err != nil {
		t.Fatalf("查找方法引用失败: %v", err)
	}
	if res.Key != "m:a.Target:foo:()V" {
		t.Fatalf("方法引用键异常: %s", res.Key)
	}
	if len(res.Sites) != 1 || res.Sites[0].FromClass != "a.Caller" || res.Sites[0].FromName != "call" {
		t.Fatalf("方法引用点异常: %+v", res.Sites)
	}

	// 字段引用。
	res, err = s.References(ctx, "1.0", "client", "field", "a.Target", "v", "I")
	if err != nil {
		t.Fatalf("查找字段引用失败: %v", err)
	}
	if res.Key != "f:a.Target:v:I" || len(res.Sites) != 1 || res.Sites[0].FromName != "call" {
		t.Fatalf("字段引用点异常: %+v", res)
	}

	// 接口方法引用（INVOKEINTERFACE）。
	res, err = s.References(ctx, "1.0", "client", "method", "a.Iface", "go", "()V")
	if err != nil {
		t.Fatalf("查找接口方法引用失败: %v", err)
	}
	if len(res.Sites) != 1 || res.Sites[0].FromClass != "a.Impl" {
		t.Fatalf("接口方法引用点异常: %+v", res.Sites)
	}
}

func TestReferencesJarExternalIgnored(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, refClasses(t), true)

	// java.lang.String 不在 jar 内：应返回 CLASS_NOT_FOUND 而非空列表（目标不可解析）。
	_, err := s.References(context.Background(), "1.0", "client", "method", "java.lang.String", "length", "()I")
	if !errors.Is(err, ErrClassNotFound) {
		t.Fatalf("jar 外类期望 ErrClassNotFound，实际 %v", err)
	}

	// a.External 自身存在，但对它的引用为空（它只引用 jar 外类）。
	res, err := s.References(context.Background(), "1.0", "client", "class", "a.External", "", "")
	if err != nil {
		t.Fatalf("查找 a.External 引用失败: %v", err)
	}
	if len(res.Sites) != 0 || res.Total != 0 {
		t.Fatalf("a.External 不应有引用: %+v", res.Sites)
	}
}

func TestReferencesValidation(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, refClasses(t), true)
	ctx := context.Background()

	// 非法 kind。
	if _, err := s.References(ctx, "1.0", "client", "bogus", "a.Target", "", ""); !errors.Is(err, ErrBadReference) {
		t.Errorf("非法 kind 期望 ErrBadReference，实际 %v", err)
	}
	// 方法引用缺 name。
	if _, err := s.References(ctx, "1.0", "client", "method", "a.Target", "", "()V"); !errors.Is(err, ErrBadReference) {
		t.Errorf("缺 name 期望 ErrBadReference，实际 %v", err)
	}
	// 非法类名。
	if _, err := s.References(ctx, "1.0", "client", "class", "a..B", "", ""); !errors.Is(err, ErrBadReference) {
		t.Errorf("非法类名期望 ErrBadReference，实际 %v", err)
	}
	// 版本未准备。
	if _, err := s.References(ctx, "9.9", "client", "class", "a.Target", "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("未准备版本期望 ErrNotFound，实际 %v", err)
	}
	// 已缓存 jar 但未建索引。
	seedJar(t, s, "2.0", SideClient, refClasses(t), false)
	if _, err := s.References(ctx, "2.0", "client", "class", "a.Target", "", ""); !errors.Is(err, ErrNotIndexed) {
		t.Errorf("未建索引期望 ErrNotIndexed，实际 %v", err)
	}
}

func TestDecodeCodeRefs(t *testing.T) {
	// GETFIELD #1（0xb4 0x00 0x01）+ INVOKEVIRTUAL #2（0xb6 0x00 0x02）
	// + NEW #3（0xbb 0x00 0x03）+ LDC #4（0x12 0x04）+ RETURN（0xb1）
	code := []byte{0xb4, 0x00, 0x01, 0xb6, 0x00, 0x02, 0xbb, 0x00, 0x03, 0x12, 0x04, 0xb1}
	got := decodeCodeRefs(code)
	want := []codeRef{
		{0xb4, 1}, {0xb6, 2}, {0xbb, 3}, {0x12, 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("指令引用解析异常:\n got %+v\nwant %+v", got, want)
	}

	// 截断的操作码：应安全停止而非 panic。
	if out := decodeCodeRefs([]byte{0xb4, 0x00}); len(out) != 0 {
		t.Fatalf("截断指令应返回空，实际 %+v", out)
	}
	// 未知操作码：立即停止。
	if out := decodeCodeRefs([]byte{0xfe, 0xb1}); len(out) != 0 {
		t.Fatalf("未知操作码应停止，实际 %+v", out)
	}
}

func TestDescriptorClassRefs(t *testing.T) {
	got := descriptorClassRefs("(Ljava/lang/String;[Lnet/minecraft/Foo;I)V")
	want := []string{"java.lang.String", "net.minecraft.Foo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("描述符类引用异常: got %+v want %+v", got, want)
	}
	if out := descriptorClassRefs("()V"); len(out) != 0 {
		t.Fatalf("无对象类型应返回空，实际 %+v", out)
	}
}

func TestReferencesCacheInvalidatedByJarChange(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, refClasses(t), true)
	ctx := context.Background()

	if _, err := s.References(ctx, "1.0", "client", "class", "a.Target", "", ""); err != nil {
		t.Fatalf("首次查找引用失败: %v", err)
	}

	// 换成新 jar（摘要变化）→ 引用索引缓存必须失效重建。
	changed := map[string][]byte{
		classPath("a.Target"): buildClass(t, classSpec{Name: "a.Target", Super: "java.lang.Object", Access: accPublic}),
		classPath("a.NewCaller"): buildClass(t, classSpec{
			Name: "a.NewCaller", Super: "java.lang.Object", Access: accPublic,
			Methods: []memberSpec{{Access: accPublic, Name: "call", Desc: "()V", Refs: []refSpec{
				{Kind: "class", Owner: "a.Target"},
			}}},
		}),
	}
	seedJar(t, s, "1.0", SideClient, changed, true)

	res, err := s.References(ctx, "1.0", "client", "class", "a.Target", "", "")
	if err != nil {
		t.Fatalf("变更后查找引用失败: %v", err)
	}
	if len(res.Sites) != 1 || res.Sites[0].FromClass != "a.NewCaller" {
		t.Fatalf("jar 变更后引用索引应失效重建: %+v", res.Sites)
	}
}
