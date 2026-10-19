package mappings

import (
	"io"
	"strings"
)

// 命名空间归类关键词：用于把 Tiny 的任意命名空间列对应到本站三列
// （obf / mid / named），而不是盲目按下标取，避免列含义错位。
var (
	nsObfKeys   = []string{"official", "obf", "obfuscated"}
	nsMidKeys   = []string{"intermediary", "srg", "middle"}
	nsNamedKeys = []string{"named", "mojang", "mcp", "parchment", "yarn"}
)

// parseTinyV2 解析 Tiny v2（Fabric Yarn 等）。
//
// 结构：
//
//	tiny	2	0	official	intermediary	named
//	c	aaa	net/minecraft/class_1297	net/minecraft/entity/Entity
//		m	()V	a	method_1	method_1
//		f	I	b	field_1	field_1
//		p	1	param	param
//
// 描述符只存在于**第一命名空间**，故记入 obf_desc；注释行与元素行的区分依据
// 字段数（注释行只有 1 个命名空间列），与 Tiny v2 规范一致。
func parseTinyV2(r io.Reader) (*Result, error) {
	sc := newScanner(r)
	var (
		namespaces []string
		res        *Result
		cur        *Class
		last       *Member
	)

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		// Tiny v2 用前导制表符表示嵌套层级（类行无缩进，成员行缩进 1 个制表符），
		// 故先剥离缩进再按制表符分列，避免首个制表符产生空标签列。
		content := strings.TrimLeft(line, "\t")
		fields := strings.Split(content, "\t")
		tag := fields[0]

		// 头部
		if tag == "tiny" && res == nil {
			if len(fields) < 4 {
				return nil, ErrUnknownFormat
			}
			namespaces = fields[3:]
			res = &Result{Format: FormatTinyV2, Namespaces: append([]string{}, namespaces...)}
			continue
		}
		if res == nil {
			continue
		}

		switch tag {
		case "c":
			// 类行：1 + 命名空间数列；其余为注释（仅 2 列）。
			if len(fields) != 1+len(namespaces) {
				continue
			}
			cls := buildClass(namespaces, fields[1:])
			res.Classes = append(res.Classes, cls)
			cur = &res.Classes[len(res.Classes)-1]
			last = nil
		case "m", "f", "p":
			if cur == nil || len(fields) != 2+len(namespaces) {
				continue
			}
			kind := map[string]string{"m": "method", "f": "field", "p": "param"}[tag]
			m := buildMember(namespaces, kind, fields[1], fields[2:])
			cur.Members = append(cur.Members, m)
			last = &cur.Members[len(cur.Members)-1]
		default:
			// 未知标签（如注释扩展）忽略，不影响已解析数据。
			_ = last
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		return nil, ErrUnknownFormat
	}
	res.MemberCount = countMembers(res.Classes)
	return res, nil
}

// parseTinyV1 解析 Tiny v1（历史 Yarn 输出）。
//
// 结构：
//
//	v1	official	intermediary	named
//	CLASS	aaa	net/minecraft/class_1297	net/minecraft/entity/Entity
//		METHOD	()V	a	method_1	method_1
//		FIELD	I	b	field_1	field_1
func parseTinyV1(r io.Reader) (*Result, error) {
	sc := newScanner(r)
	var (
		namespaces []string
		res        *Result
		cur        *Class
	)

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)

		if fields[0] == "v1" && res == nil {
			namespaces = fields[1:]
			res = &Result{Format: FormatTinyV1, Namespaces: append([]string{}, namespaces...)}
			continue
		}
		if res == nil || len(namespaces) == 0 {
			continue
		}

		indented := strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ")
		switch {
		case !indented && fields[0] == "CLASS":
			if len(fields) != 1+len(namespaces) {
				continue
			}
			cls := buildClass(namespaces, fields[1:])
			res.Classes = append(res.Classes, cls)
			cur = &res.Classes[len(res.Classes)-1]
		case indented && (fields[0] == "METHOD" || fields[0] == "FIELD") && cur != nil:
			if len(fields) != 2+len(namespaces) {
				continue
			}
			kind := "field"
			if fields[0] == "METHOD" {
				kind = "method"
			}
			cur.Members = append(cur.Members, buildMember(namespaces, kind, fields[1], fields[2:]))
		default:
			// COMMENT 等忽略。
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		return nil, ErrUnknownFormat
	}
	res.MemberCount = countMembers(res.Classes)
	return res, nil
}

// buildClass 按命名空间归类构造类映射。
func buildClass(namespaces, names []string) Class {
	obf, mid, named := splitNamespaces(namespaces, names)
	cls := Class{ObfName: obf, MidName: mid, NamedName: named}
	cls.Package = packageOf(firstNonEmpty(named, mid, obf))
	return cls
}

// buildMember 按命名空间归类构造成员映射。
// desc 为第一命名空间描述符（Tiny 只提供这一份）。
func buildMember(namespaces []string, kind, desc string, names []string) Member {
	obf, mid, named := splitNamespaces(namespaces, names)
	m := Member{
		Kind:      kind,
		ObfName:   obf,
		MidName:   mid,
		NamedName: named,
		DescKey:   desc,
		ObfDesc:   desc,
	}
	if m.DescKey == "" {
		m.DescKey = kind
	}
	return m
}

// splitNamespaces 依据命名空间名称归类 obf / mid / named 三列。
// 无法按名称归类时回退为下标顺序（0 → obf，1 → mid，2 → named）。
func splitNamespaces(namespaces, names []string) (obf, mid, named string) {
	if len(names) == 0 {
		return "", "", ""
	}
	matched := false
	for i, ns := range namespaces {
		if i >= len(names) {
			break
		}
		lower := strings.ToLower(ns)
		switch {
		case containsAny(lower, nsObfKeys):
			obf = names[i]
			matched = true
		case containsAny(lower, nsMidKeys):
			mid = names[i]
			matched = true
		case containsAny(lower, nsNamedKeys):
			named = names[i]
			matched = true
		}
	}
	if matched {
		return obf, mid, named
	}
	obf = names[0]
	if len(names) > 1 {
		mid = names[1]
	}
	if len(names) > 2 {
		named = names[2]
	}
	return obf, mid, named
}

// containsAny 判断 s 是否包含任一关键词。
func containsAny(s string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
