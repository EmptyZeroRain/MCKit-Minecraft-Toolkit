package mcsrc

import (
	"errors"
	"strings"
	"testing"
)

// bytecodeClass 构造一个含 Code 属性的类（方法体引用 a.Target）。
func bytecodeClass(t *testing.T) []byte {
	t.Helper()
	return buildClass(t, classSpec{
		Name: "a.Sample", Super: "java.lang.Object", Access: accPublic | accFinal,
		Fields: []memberSpec{{Access: accPrivate, Name: "v", Desc: "I"}},
		Methods: []memberSpec{
			{Access: accPublic, Name: "run", Desc: "()V", Refs: []refSpec{
				{Kind: "field", Owner: "a.Target", Name: "v", Desc: "I"},
				{Kind: "method", Owner: "a.Target", Name: "foo", Desc: "()V"},
				{Kind: "class", Owner: "a.Target"},
			}},
			// 无 Code 的方法（抽象 / native 语义），覆盖 MethodCount 与无 Code 分支。
			{Access: accPublic | accAbstract, Name: "extra", Desc: "()V"},
		},
	})
}

func TestDisassemble(t *testing.T) {
	dis, err := Disassemble(bytecodeClass(t))
	if err != nil {
		t.Fatalf("反汇编失败: %v", err)
	}
	if dis.Major != 52 || dis.Minor != 0 {
		t.Errorf("class 版本异常: %d.%d", dis.Major, dis.Minor)
	}
	if dis.MethodCount != 2 {
		t.Errorf("方法总数期望 2，实际 %d", dis.MethodCount)
	}
	// run 方法体：GETFIELD + INVOKEVIRTUAL + NEW + RETURN = 4 条指令。
	if dis.InstructionCount != 4 {
		t.Errorf("指令总数期望 4，实际 %d", dis.InstructionCount)
	}
	if dis.Truncated {
		t.Error("小样本不应截断")
	}

	for _, want := range []string{
		"// class version 52.0",
		"a/Sample",
		"run",
		"GETFIELD a/Target.v : I",
		"INVOKEVIRTUAL a/Target.foo (()V)",
		"NEW a/Target",
		"RETURN",
		"MAXSTACK = 1",
		"MAXLOCALS = 1",
		"extra",
	} {
		if !strings.Contains(dis.Text, want) {
			t.Errorf("反汇编文本缺少 %q：\n%s", want, dis.Text)
		}
	}
}

func TestDisassembleRejectsBrokenBytes(t *testing.T) {
	if _, err := Disassemble([]byte("not-a-class-file")); !errors.Is(err, ErrBadClassFile) {
		t.Fatalf("损坏字节期望 ErrBadClassFile，实际 %v", err)
	}
	// 截断的合法头（魔数正确但后续缺失）。
	truncated := []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x00}
	if _, err := Disassemble(truncated); !errors.Is(err, ErrBadClassFile) {
		t.Fatalf("截断字节期望 ErrBadClassFile，实际 %v", err)
	}
}

func TestClassBytecode(t *testing.T) {
	s := NewStore(t.TempDir(), 0, 0)
	seedJar(t, s, "1.0", SideClient, map[string][]byte{
		classPath("a.Sample"): bytecodeClass(t),
		classPath("a.Target"): buildClass(t, classSpec{
			Name: "a.Target", Super: "java.lang.Object", Access: accPublic,
			Fields:  []memberSpec{{Access: accPrivate, Name: "v", Desc: "I"}},
			Methods: []memberSpec{{Access: accPublic, Name: "foo", Desc: "()V"}},
		}),
	}, false)

	bc, err := s.ClassBytecode("1.0", "client", "a.Sample")
	if err != nil {
		t.Fatalf("取字节码失败: %v", err)
	}
	if bc.Name != "a.Sample" || bc.Simple != "Sample" || bc.Pkg != "a" || bc.Kind != "class" {
		t.Fatalf("字节码元信息异常: %+v", bc)
	}
	if bc.MethodCount != 2 || bc.InstructionCount != 4 {
		t.Fatalf("字节码计数异常: methods=%d insns=%d", bc.MethodCount, bc.InstructionCount)
	}
	if !strings.Contains(bc.Text, "INVOKEVIRTUAL a/Target.foo (()V)") {
		t.Fatalf("字节码文本异常:\n%s", bc.Text)
	}

	// 斜杠写法等价。
	if _, err := s.ClassBytecode("1.0", "client", "a/Sample"); err != nil {
		t.Fatalf("斜杠写法应等价: %v", err)
	}

	// 类不存在。
	if _, err := s.ClassBytecode("1.0", "client", "a.Nope"); !errors.Is(err, ErrClassNotFound) {
		t.Errorf("类不存在期望 ErrClassNotFound，实际 %v", err)
	}
	// 类名非法。
	if _, err := s.ClassBytecode("1.0", "client", "a..B"); !errors.Is(err, ErrBadClass) {
		t.Errorf("非法类名期望 ErrBadClass，实际 %v", err)
	}
	// 版本未准备。
	if _, err := s.ClassBytecode("9.9", "client", "a.Sample"); !errors.Is(err, ErrNotFound) {
		t.Errorf("未准备版本期望 ErrNotFound，实际 %v", err)
	}
	// 未建索引也可用（字节码只依赖 jar 本体）。
	if _, err := s.ClassBytecode("1.0", "client", "a.Sample"); err != nil {
		t.Errorf("字节码不应依赖类索引: %v", err)
	}
}
