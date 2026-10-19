package mcsrc

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseClassFileStructure(t *testing.T) {
	data := buildClass(t, classSpec{
		Name:       "a.A",
		Super:      "a.B",
		Interfaces: []string{"a.I", "java.io.Serializable"},
		Access:     accPublic | accFinal,
		Fields: []memberSpec{
			{Access: accPrivate | accStatic | accFinal, Name: "count", Desc: "I"},
			{Access: accPublic, Name: "names", Desc: "[Ljava/lang/String;"},
		},
		Methods: []memberSpec{
			{Access: accPublic, Name: "get", Desc: "(Ljava/lang/String;I)Z"},
			{Access: accPublic | accStatic, Name: "of", Desc: "()La/A;"},
		},
	})

	cf, err := ParseClassFile(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cf.ThisClass != "a.A" {
		t.Errorf("ThisClass 期望 a.A，实际 %q", cf.ThisClass)
	}
	if cf.SuperClass != "a.B" {
		t.Errorf("SuperClass 期望 a.B，实际 %q", cf.SuperClass)
	}
	if !reflect.DeepEqual(cf.Interfaces, []string{"a.I", "java.io.Serializable"}) {
		t.Errorf("Interfaces 异常: %+v", cf.Interfaces)
	}
	if cf.Major != 52 || cf.Minor != 0 {
		t.Errorf("版本期望 52.0，实际 %d.%d", cf.Major, cf.Minor)
	}
	if cf.Kind() != "class" || cf.IsAbstract() {
		t.Errorf("Kind 期望 class 且非抽象，实际 kind=%s abstract=%v", cf.Kind(), cf.IsAbstract())
	}
	if len(cf.Fields) != 2 || cf.Fields[0].Name != "count" || cf.Fields[0].Descriptor != "I" {
		t.Errorf("字段解析异常: %+v", cf.Fields)
	}
	if len(cf.Methods) != 2 || cf.Methods[0].Name != "get" || cf.Methods[0].Descriptor != "(Ljava/lang/String;I)Z" {
		t.Errorf("方法解析异常: %+v", cf.Methods)
	}
}

func TestParseClassFileObjectSuperZero(t *testing.T) {
	// java.lang.Object 自身的 super_class 为 0，应归一化为 java.lang.Object。
	data := buildClass(t, classSpec{Name: "java.lang.Object", Access: accPublic})
	cf, err := ParseClassFile(data)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cf.SuperClass != "java.lang.Object" {
		t.Errorf("super_class=0 期望归一化为 java.lang.Object，实际 %q", cf.SuperClass)
	}
}

func TestParseClassFileKinds(t *testing.T) {
	cases := []struct {
		name  string
		flags uint16
		want  string
	}{
		{"普通类", accPublic, "class"},
		{"接口", accPublic | accInterface | accAbstract, "interface"},
		{"枚举", accPublic | accFinal | accEnum, "enum"},
		{"注解", accPublic | accInterface | accAbstract | accAnnotation, "annotation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildClass(t, classSpec{Name: "a.K", Access: tc.flags})
			cf, err := ParseClassFile(data)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if got := cf.Kind(); got != tc.want {
				t.Errorf("Kind 期望 %s，实际 %s", tc.want, got)
			}
		})
	}
}

func TestParseClassFileRejectsMalformed(t *testing.T) {
	valid := buildClass(t, classSpec{
		Name:    "a.A",
		Super:   "a.B",
		Fields:  []memberSpec{{Access: accPublic, Name: "x", Desc: "I"}},
		Methods: []memberSpec{{Access: accPublic, Name: "m", Desc: "()V"}},
	})

	t.Run("空输入", func(t *testing.T) {
		if _, err := ParseClassFile(nil); !errors.Is(err, ErrBadClassFile) {
			t.Fatalf("期望 ErrBadClassFile，实际 %v", err)
		}
	})

	t.Run("魔数错误", func(t *testing.T) {
		bad := append([]byte{}, valid...)
		bad[0] = 0
		if _, err := ParseClassFile(bad); !errors.Is(err, ErrBadClassFile) {
			t.Fatalf("期望 ErrBadClassFile，实际 %v", err)
		}
	})

	t.Run("未知常量池 tag", func(t *testing.T) {
		bad := append([]byte{}, valid...)
		bad[10] = 99 // 常量池第一个 tag（magic4 + minor2 + major2 + count2 = 10）
		_, err := ParseClassFile(bad)
		if err == nil || !strings.Contains(err.Error(), "未知常量池 tag") {
			t.Fatalf("期望未知 tag 报错，实际 %v", err)
		}
	})

	t.Run("任意截断不得 panic", func(t *testing.T) {
		for n := 0; n < len(valid); n++ {
			// 只要不 panic 即通过；截断可能落在被跳过的属性区，允许解析成功。
			_, _ = ParseClassFile(valid[:n])
		}
	})

	t.Run("伪造属性长度", func(t *testing.T) {
		bad := append([]byte{}, valid...)
		// 方法表最后一个属性数量之后紧跟 (name_index, length)；这里把 length 拉满。
		// 直接改末尾 6 字节中的 length 字段（结构固定，越界即报错）。
		for i := len(bad) - 6; i < len(bad)-2; i++ {
			bad[i] = 0xff
		}
		_, _ = ParseClassFile(bad) // 允许成功或失败，不得 panic
	})
}

func TestFieldTypeRendering(t *testing.T) {
	cases := []struct {
		desc string
		want string
		bad  bool
	}{
		{desc: "I", want: "int"},
		{desc: "Z", want: "boolean"},
		{desc: "V", bad: true}, // void 仅可作方法返回类型
		{desc: "Ljava/lang/String;", want: "java.lang.String"},
		{desc: "[I", want: "int[]"},
		{desc: "[[Ljava/lang/Object;", want: "java.lang.Object[][]"},
		{desc: "[Ljava/util/Map$Entry;", want: "java.util.Map$Entry[]"},
		{desc: "", bad: true},
		{desc: "X", bad: true},
		{desc: "Ljava/lang/String", bad: true},
		{desc: "II", bad: true},
		{desc: "[V", bad: true},
	}
	for _, tc := range cases {
		got, err := FieldType(tc.desc)
		if tc.bad {
			if err == nil {
				t.Errorf("%q 期望报错，实际得到 %q", tc.desc, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q 解析失败: %v", tc.desc, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q 期望 %q，实际 %q", tc.desc, tc.want, got)
		}
	}
}

func TestMethodSignatureRendering(t *testing.T) {
	params, ret, err := MethodSignature("(Ljava/lang/String;I[Z)Ljava/util/List;")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !reflect.DeepEqual(params, []string{"java.lang.String", "int", "boolean[]"}) {
		t.Errorf("参数异常: %+v", params)
	}
	if ret != "java.util.List" {
		t.Errorf("返回类型异常: %q", ret)
	}

	// 无参无返回。
	params, ret, err = MethodSignature("()V")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(params) != 0 || ret != "void" {
		t.Errorf("()V 异常: params=%+v ret=%q", params, ret)
	}

	for _, bad := range []string{"", "I", "(I", "()", "(I)VV", "(V)V"} {
		if _, _, err := MethodSignature(bad); err == nil {
			t.Errorf("%q 期望报错", bad)
		}
	}
}

func TestModifiersRendering(t *testing.T) {
	if got := ClassModifiers(accPublic | accAbstract | accFinal); !reflect.DeepEqual(got, []string{"public", "abstract", "final"}) {
		t.Errorf("类修饰符异常: %+v", got)
	}
	if got := ClassModifiers(accPublic | accInterface | accAbstract); !reflect.DeepEqual(got, []string{"public"}) {
		t.Errorf("接口不应输出 abstract（由 Kind 表达）: %+v", got)
	}
	if got := FieldModifiers(accPrivate | accStatic | accFinal | accVolatile); !reflect.DeepEqual(got, []string{"private", "static", "final", "volatile"}) {
		t.Errorf("字段修饰符异常: %+v", got)
	}
	if got := MethodModifiers(accProtected | accStatic | accSynchronized | accNative); !reflect.DeepEqual(got, []string{"protected", "static", "synchronized", "native"}) {
		t.Errorf("方法修饰符异常: %+v", got)
	}
	if got := MethodModifiers(0); len(got) != 0 {
		t.Errorf("包级私有方法不应有可见性关键字: %+v", got)
	}
}
