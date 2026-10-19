package mappings

import (
	"strings"
	"testing"
)

// 说明：以下夹具均为**合成数据**（不来自任何真实映射文件），
// 只用于校验解析器对格式的结构化能力；真实数据的导入由 cmd/import-mappings 完成。
// 制表符用 "\t" 显式书写，避免编辑器把制表符替换为空格后测试静默失效。

const proguardFixture = `# 合成示例（ProGuard / Mojang 官方映射格式）
com.example.mod.Example -> aaa:
# {"fileName":"Example.java","id":"sourceFile"}
    10:10:void <init>() -> <init>
    12:13:void run(com.example.mod.Target,int) -> a
    com.example.mod.Target target -> b
    int count -> c
com.example.mod.Target -> aab:
    5:5:boolean isValid() -> a
`

const tsrgV1Fixture = "aaa com/example/mod/Example\n" +
	"\tmethod_1 ()V func_1\n" +
	"\tfield_1 Lcom/example/mod/Target; field_1\n" +
	"aab com/example/mod/Target\n" +
	"\tfield_2 I field_2\n"

const tsrgV2Fixture = "aaa com/example/mod/Example com/example/mod/Example\n" +
	"\tmethod_1 ()V func_1 run\n" +
	"\tfield_1 Lcom/example/mod/Target; field_1 target\n"

const tinyV2Fixture = "tiny\t2\t0\tofficial\tintermediary\tnamed\n" +
	"c\taaa\tnet/minecraft/class_1\tnet/minecraft/entity/Example\n" +
	"\tm\t()V\ta\tmethod_1\trun\n" +
	"\tf\tI\tb\tfield_1\tcount\n"

const tinyV1Fixture = "v1\tofficial\tintermediary\tnamed\n" +
	"CLASS\taaa\tnet/minecraft/class_1\tnet/minecraft/entity/Example\n" +
	"\tMETHOD\t()V\ta\tmethod_1\trun\n" +
	"\tFIELD\tI\tb\tfield_1\tcount\n"

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Format
	}{
		{"proguard", proguardFixture, FormatProGuard},
		{"tsrg", tsrgV1Fixture, FormatTSRG},
		{"tiny v2", tinyV2Fixture, FormatTinyV2},
		{"tiny v1", tinyV1Fixture, FormatTinyV1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Detect([]byte(c.in))
			if err != nil {
				t.Fatalf("Detect 失败: %v", err)
			}
			if got != c.want {
				t.Fatalf("Detect = %q, 期望 %q", got, c.want)
			}
		})
	}
}

func TestParseProGuard(t *testing.T) {
	res, err := Parse([]byte(proguardFixture), "auto")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.Format != FormatProGuard {
		t.Fatalf("格式 = %q", res.Format)
	}
	if len(res.Classes) != 2 {
		t.Fatalf("类数 = %d, 期望 2", len(res.Classes))
	}
	first := res.Classes[0]
	if first.ObfName != "aaa" || first.NamedName != "com.example.mod.Example" {
		t.Fatalf("首类映射错误: %+v", first)
	}
	if first.Package != "com.example.mod" {
		t.Fatalf("包名 = %q", first.Package)
	}
	if len(first.Members) != 4 {
		t.Fatalf("首类成员数 = %d, 期望 4", len(first.Members))
	}
	// 方法：行号前缀需被剥离，名字与描述符正确
	method := first.Members[1]
	if method.Kind != "method" || method.NamedName != "run" || method.ObfName != "a" {
		t.Fatalf("方法解析错误: %+v", method)
	}
	if method.DescKey != "(com.example.mod.Target,int)" {
		t.Fatalf("方法描述符 = %q", method.DescKey)
	}
	if method.ObfDesc != "" {
		t.Fatalf("ProGuard 不应臆造混淆描述符，得到 %q", method.ObfDesc)
	}
	if !strings.Contains(method.NamedDesc, "-> void") {
		t.Fatalf("可读描述符 = %q", method.NamedDesc)
	}
	// 字段
	field := first.Members[2]
	if field.Kind != "field" || field.NamedName != "target" || field.NamedDesc != "com.example.mod.Target" {
		t.Fatalf("字段解析错误: %+v", field)
	}
	if res.MemberCount != 5 {
		t.Fatalf("成员总数 = %d, 期望 5", res.MemberCount)
	}
}

func TestParseTSRG(t *testing.T) {
	res, err := Parse([]byte(tsrgV1Fixture), "auto")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.Format != FormatTSRG {
		t.Fatalf("格式 = %q", res.Format)
	}
	if len(res.Classes) != 2 {
		t.Fatalf("类数 = %d", len(res.Classes))
	}
	cls := res.Classes[0]
	if cls.ObfName != "aaa" || cls.MidName != "com/example/mod/Example" {
		t.Fatalf("类映射错误: %+v", cls)
	}
	if len(cls.Members) != 2 {
		t.Fatalf("成员数 = %d", len(cls.Members))
	}
	if cls.Members[0].Kind != "method" || cls.Members[0].MidName != "func_1" {
		t.Fatalf("方法解析错误: %+v", cls.Members[0])
	}
	// TSRG 描述符位于混淆命名空间
	if cls.Members[0].ObfDesc != "()V" {
		t.Fatalf("混淆描述符 = %q", cls.Members[0].ObfDesc)
	}
	if cls.Members[1].Kind != "field" {
		t.Fatalf("字段解析错误: %+v", cls.Members[1])
	}
	// 无描述符的 func_ 前缀应识别为方法
	if res.Classes[1].Members[0].Kind != "field" {
		t.Fatalf("field_2 应识别为字段: %+v", res.Classes[1].Members[0])
	}

	// v2：出现第三列（可读名）
	res2, err := Parse([]byte(tsrgV2Fixture), "tsrg")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(res2.Namespaces) != 3 {
		t.Fatalf("命名空间 = %v, 期望 3 列", res2.Namespaces)
	}
	if res2.Classes[0].NamedName != "com/example/mod/Example" {
		t.Fatalf("v2 可读类名 = %q", res2.Classes[0].NamedName)
	}
	if res2.Classes[0].Members[0].NamedName != "run" {
		t.Fatalf("v2 可读方法名 = %q", res2.Classes[0].Members[0].NamedName)
	}
}

func TestParseTiny(t *testing.T) {
	for _, c := range []struct {
		name   string
		in     string
		format Format
	}{
		{"v2", tinyV2Fixture, FormatTinyV2},
		{"v1", tinyV1Fixture, FormatTinyV1},
	} {
		t.Run(c.name, func(t *testing.T) {
			res, err := Parse([]byte(c.in), "auto")
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if res.Format != c.format {
				t.Fatalf("格式 = %q, 期望 %q", res.Format, c.format)
			}
			if len(res.Classes) != 1 {
				t.Fatalf("类数 = %d", len(res.Classes))
			}
			cls := res.Classes[0]
			// 命名空间按名称归类：official→obf，intermediary→mid，named→named
			if cls.ObfName != "aaa" {
				t.Fatalf("obf = %q", cls.ObfName)
			}
			if cls.MidName != "net/minecraft/class_1" {
				t.Fatalf("mid = %q", cls.MidName)
			}
			if cls.NamedName != "net/minecraft/entity/Example" {
				t.Fatalf("named = %q", cls.NamedName)
			}
			if cls.Package != "net/minecraft/entity" {
				t.Fatalf("包名 = %q", cls.Package)
			}
			if len(cls.Members) != 2 {
				t.Fatalf("成员数 = %d", len(cls.Members))
			}
			if cls.Members[0].Kind != "method" || cls.Members[0].NamedName != "run" {
				t.Fatalf("方法解析错误: %+v", cls.Members[0])
			}
			if cls.Members[0].ObfDesc != "()V" {
				t.Fatalf("描述符 = %q", cls.Members[0].ObfDesc)
			}
			if cls.Members[1].Kind != "field" || cls.Members[1].NamedName != "count" {
				t.Fatalf("字段解析错误: %+v", cls.Members[1])
			}
		})
	}
}

func TestParseUnknownFormat(t *testing.T) {
	// 无任何可识别结构（每行只有一个字段）→ 无法识别。
	// 注意：TSRG 是最宽松的格式（两列即可），故只有单列内容才判定为未知。
	if _, err := Parse([]byte("这不是映射文件\n没有分隔符\n"), "auto"); err == nil {
		t.Fatal("应返回无法识别格式的错误")
	}
	if _, err := Parse([]byte(proguardFixture), "not-a-format"); err == nil {
		t.Fatal("非法格式提示应报错")
	}
}
