package mcsrc

import (
	"errors"
	"fmt"
	"strings"
)

// JVM 描述符渲染（JVMS §4.3）。
//
// 目标：把 class 文件里的紧凑描述符转成人可读的 Java 类型与签名，
// 例如 "(Ljava/lang/String;I[Z)V" → 参数 [java.lang.String, int, boolean[]]、返回 void。

// ErrBadDescriptor 描述符非法。
var ErrBadDescriptor = errors.New("描述符非法")

// baseTypes 基础类型描述符 → Java 类型名。
var baseTypes = map[byte]string{
	'B': "byte",
	'C': "char",
	'D': "double",
	'F': "float",
	'I': "int",
	'J': "long",
	'S': "short",
	'Z': "boolean",
	'V': "void",
}

// FieldType 把字段描述符渲染为 Java 类型名。
// 例："I" → int；"[I" → int[]；"Ljava/lang/String;" → java.lang.String。
func FieldType(desc string) (string, error) {
	t, n, err := parseType(desc)
	if err != nil {
		return "", err
	}
	if n != len(desc) {
		return "", fmt.Errorf("%w: 尾随字符 %q", ErrBadDescriptor, desc[n:])
	}
	// void 只能作为方法返回类型，字段不允许（与 JVMS §4.3.2 一致）。
	if t == "void" {
		return "", fmt.Errorf("%w: 字段类型不能是 void", ErrBadDescriptor)
	}
	return t, nil
}

// MethodSignature 把方法描述符拆成参数类型列表与返回类型。
func MethodSignature(desc string) (params []string, ret string, err error) {
	if !strings.HasPrefix(desc, "(") {
		return nil, "", fmt.Errorf("%w: 缺少 '('", ErrBadDescriptor)
	}
	i := 1
	for {
		if i >= len(desc) {
			return nil, "", fmt.Errorf("%w: 缺少 ')'", ErrBadDescriptor)
		}
		if desc[i] == ')' {
			i++
			break
		}
		t, n, err := parseType(desc[i:])
		if err != nil {
			return nil, "", err
		}
		// 参数不允许 void（JVMS §4.3.3）。
		if t == "void" {
			return nil, "", fmt.Errorf("%w: 参数类型不能是 void", ErrBadDescriptor)
		}
		params = append(params, t)
		i += n
	}
	ret, n, err := parseType(desc[i:])
	if err != nil {
		return nil, "", err
	}
	if i+n != len(desc) {
		return nil, "", fmt.Errorf("%w: 尾随字符 %q", ErrBadDescriptor, desc[i+n:])
	}
	return params, ret, nil
}

// parseType 解析单个类型描述符，返回 Java 类型名与消耗字节数。
func parseType(s string) (string, int, error) {
	if s == "" {
		return "", 0, fmt.Errorf("%w: 描述符为空", ErrBadDescriptor)
	}
	switch c := s[0]; {
	case c == '[':
		inner, n, err := parseType(s[1:])
		if err != nil {
			return "", 0, err
		}
		// 不允许 void[]，与 JVM 一致。
		if inner == "void" {
			return "", 0, fmt.Errorf("%w: void 不能作为数组元素", ErrBadDescriptor)
		}
		return inner + "[]", n + 1, nil
	case c == 'L':
		end := strings.IndexByte(s, ';')
		if end < 0 {
			return "", 0, fmt.Errorf("%w: 缺少 ';'", ErrBadDescriptor)
		}
		name := s[1:end]
		if name == "" {
			return "", 0, fmt.Errorf("%w: 类名为空", ErrBadDescriptor)
		}
		return strings.ReplaceAll(name, "/", "."), end + 1, nil
	default:
		if t, ok := baseTypes[c]; ok {
			return t, 1, nil
		}
		return "", 0, fmt.Errorf("%w: 未知类型描述符 %q", ErrBadDescriptor, string(c))
	}
}

// ClassModifiers 渲染类级修饰符（不含 interface / enum / annotation，它们由 Kind 表达）。
func ClassModifiers(flags uint16) []string {
	out := make([]string, 0, 4)
	out = appendAccess(out, flags)
	if flags&accAbstract != 0 && flags&accInterface == 0 {
		out = append(out, "abstract")
	}
	if flags&accFinal != 0 {
		out = append(out, "final")
	}
	if flags&accSynthetic != 0 {
		out = append(out, "synthetic")
	}
	return out
}

// FieldModifiers 渲染字段修饰符。
func FieldModifiers(flags uint16) []string {
	out := make([]string, 0, 6)
	out = appendAccess(out, flags)
	if flags&accStatic != 0 {
		out = append(out, "static")
	}
	if flags&accFinal != 0 {
		out = append(out, "final")
	}
	if flags&accVolatile != 0 {
		out = append(out, "volatile")
	}
	if flags&accTransient != 0 {
		out = append(out, "transient")
	}
	if flags&accEnum != 0 {
		out = append(out, "enum")
	}
	if flags&accSynthetic != 0 {
		out = append(out, "synthetic")
	}
	return out
}

// MethodModifiers 渲染方法修饰符。
func MethodModifiers(flags uint16) []string {
	out := make([]string, 0, 8)
	out = appendAccess(out, flags)
	if flags&accStatic != 0 {
		out = append(out, "static")
	}
	if flags&accFinal != 0 {
		out = append(out, "final")
	}
	if flags&accAbstract != 0 {
		out = append(out, "abstract")
	}
	if flags&accSynchronized != 0 {
		out = append(out, "synchronized")
	}
	if flags&accNative != 0 {
		out = append(out, "native")
	}
	if flags&accStrict != 0 {
		out = append(out, "strictfp")
	}
	if flags&accBridge != 0 {
		out = append(out, "bridge")
	}
	if flags&accVarargs != 0 {
		out = append(out, "varargs")
	}
	if flags&accSynthetic != 0 {
		out = append(out, "synthetic")
	}
	return out
}

// appendAccess 追加可见性修饰符（包级私有不输出关键字）。
func appendAccess(out []string, flags uint16) []string {
	switch {
	case flags&accPublic != 0:
		out = append(out, "public")
	case flags&accProtected != 0:
		out = append(out, "protected")
	case flags&accPrivate != 0:
		out = append(out, "private")
	}
	return out
}
