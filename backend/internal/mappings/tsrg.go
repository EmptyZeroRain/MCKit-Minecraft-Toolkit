package mappings

import (
	"io"
	"strings"
)

// parseTSRG 解析 TSRG / TSRG2 映射（MCPConfig，Forge / NeoForge 历史体系）。
//
// 真实格式（已对照 MCPConfig 仓库的 joined.tsrg 校验）：
//
//	<混淆类名> <SRG类名>                      ← v1 类行
//	<混淆类名> <SRG类名> <可读类名>            ← v2 类行
//		<混淆方法名> (<混淆描述符>) <SRG方法名> [<可读方法名>]
//		<混淆字段名> [<描述符>] <SRG字段名> [<可读字段名>]
//		<参数序号> <混淆参数名> [<可读参数名>]     ← 双缩进为参数行（TSRG2）
//
// 说明：TSRG 的描述符写在**混淆命名空间**，故记入 obf_desc；
// 缺少描述符的行（TSRG v1 字段）按 SRG 名前缀（func_ / field_）判断方法或字段，
// 该启发式在代码与文档中均有标注，不做静默猜测。
func parseTSRG(r io.Reader) (*Result, error) {
	res := &Result{
		Format:     FormatTSRG,
		Namespaces: []string{"obf", "srg"},
	}
	sc := newScanner(r)
	var cur *Class
	var last *Member
	v2 := false
	sawClass := false

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 双缩进：参数行（TSRG2）
		if strings.HasPrefix(line, "\t\t") {
			if cur == nil || last == nil {
				continue
			}
			tokens := strings.Fields(line)
			if len(tokens) < 2 {
				continue
			}
			param := Member{Kind: "param", DescKey: tokens[0], ObfName: tokens[0]}
			param.MidName = tokens[1]
			if len(tokens) >= 3 {
				param.NamedName = tokens[2]
			}
			cur.Members = append(cur.Members, param)
			continue
		}

		// 单缩进：成员行
		if strings.HasPrefix(line, "\t") {
			if cur == nil {
				continue
			}
			tokens := strings.Fields(line)
			if len(tokens) < 2 {
				continue
			}
			var m Member
			switch {
			case strings.HasPrefix(tokens[1], "("):
				// 方法：<obf> <desc> <srg> [<named>]
				m = Member{Kind: "method", ObfName: tokens[0], DescKey: tokens[1], ObfDesc: tokens[1]}
				if len(tokens) >= 3 {
					m.MidName = tokens[2]
				}
				if len(tokens) >= 4 {
					m.NamedName = tokens[3]
				}
			case len(tokens) == 4:
				// v2 字段：<obf> <desc> <srg> <named>
				m = Member{Kind: "field", ObfName: tokens[0], DescKey: tokens[1], ObfDesc: tokens[1], MidName: tokens[2], NamedName: tokens[3]}
			case len(tokens) == 2:
				// v1 字段（无描述符）：<obf> <srg>
				m = Member{Kind: fieldKindBySRG(tokens[1]), ObfName: tokens[0], MidName: tokens[1]}
			default:
				// 三列无描述符：<obf> <srg> <named>
				m = Member{Kind: fieldKindBySRG(tokens[1]), ObfName: tokens[0], MidName: tokens[1], NamedName: tokens[2]}
			}
			if m.DescKey == "" {
				m.DescKey = m.Kind
			}
			cur.Members = append(cur.Members, m)
			last = &cur.Members[len(cur.Members)-1]
			continue
		}

		// 类行
		tokens := strings.Fields(line)
		if len(tokens) < 2 {
			continue
		}
		cls := Class{ObfName: tokens[0], MidName: tokens[1]}
		if len(tokens) >= 3 {
			cls.NamedName = tokens[2]
			v2 = true
		}
		cls.Package = packageOf(firstNonEmpty(cls.NamedName, cls.MidName, cls.ObfName))
		res.Classes = append(res.Classes, cls)
		cur = &res.Classes[len(res.Classes)-1]
		last = nil
		sawClass = true
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if sawClass && v2 {
		res.Namespaces = []string{"obf", "srg", "named"}
	}
	res.MemberCount = countMembers(res.Classes)
	return res, nil
}

// fieldKindBySRG 依据 SRG 名前缀判断成员类型（TSRG v1 无描述符时的启发式）。
func fieldKindBySRG(srg string) string {
	if strings.HasPrefix(srg, "func_") || strings.HasPrefix(srg, "method_") {
		return "method"
	}
	return "field"
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
