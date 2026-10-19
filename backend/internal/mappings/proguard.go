package mappings

import (
	"io"
	"regexp"
	"strings"
)

// proguardLinePrefix 匹配 ProGuard 方法行的行号前缀（如 "10:10:"）。
var proguardLinePrefix = regexp.MustCompile(`^\d+:\d+:`)

// parseProGuard 解析 Mojang 官方映射（ProGuard 格式）。
//
// 行结构：
//
//	<可读类名> -> <混淆类名>:
//	# {"fileName":"X.java",...}          （注释行，忽略）
//	    10:10:void <init>() -> <init>    （方法：可带行号前缀，描述符用可读类型书写）
//	    org.slf4j.Logger LOGGER -> b     （字段：<类型> <名字>）
//
// 说明：ProGuard 只给出「可读类名 → 混淆名」两个命名空间，且描述符使用可读类型，
// 因此第一命名空间描述符（obf_desc）留空，不臆造混淆后的类型。
func parseProGuard(r io.Reader) (*Result, error) {
	res := &Result{
		Format:     FormatProGuard,
		Namespaces: []string{"obf", "named"},
	}
	sc := newScanner(r)
	var cur *Class

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// 类行：不缩进，形如 "<named> -> <obf>:"
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if !strings.HasSuffix(trimmed, ":") {
				continue
			}
			parts := strings.SplitN(strings.TrimSuffix(trimmed, ":"), " -> ", 2)
			if len(parts) != 2 {
				continue
			}
			named := strings.TrimSpace(parts[0])
			obf := strings.TrimSpace(parts[1])
			if named == "" || obf == "" {
				continue
			}
			res.Classes = append(res.Classes, Class{
				ObfName:   obf,
				NamedName: named,
				Package:   packageOf(named),
			})
			cur = &res.Classes[len(res.Classes)-1]
			continue
		}

		if cur == nil {
			continue
		}
		parts := strings.SplitN(trimmed, " -> ", 2)
		if len(parts) != 2 {
			continue
		}
		left := proguardLinePrefix.ReplaceAllString(strings.TrimSpace(parts[0]), "")
		obf := strings.TrimSpace(parts[1])
		if left == "" || obf == "" {
			continue
		}

		if open := strings.Index(left, "("); open >= 0 {
			close := strings.Index(left[open:], ")")
			if close < 0 {
				continue
			}
			args := left[open : open+close+1]
			head := strings.Fields(left[:open])
			if len(head) < 2 {
				continue
			}
			name := head[len(head)-1]
			ret := strings.Join(head[:len(head)-1], " ")
			cur.Members = append(cur.Members, Member{
				Kind:      "method",
				ObfName:   obf,
				NamedName: name,
				DescKey:   args,
				NamedDesc: args + " -> " + ret,
			})
			continue
		}

		fields := strings.Fields(left)
		if len(fields) < 2 {
			continue
		}
		name := fields[len(fields)-1]
		typ := strings.Join(fields[:len(fields)-1], " ")
		cur.Members = append(cur.Members, Member{
			Kind:      "field",
			ObfName:   obf,
			NamedName: name,
			DescKey:   typ,
			NamedDesc: typ,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	res.MemberCount = countMembers(res.Classes)
	return res, nil
}

// countMembers 汇总成员总数。
func countMembers(classes []Class) int {
	n := 0
	for _, c := range classes {
		n += len(c.Members)
	}
	return n
}
