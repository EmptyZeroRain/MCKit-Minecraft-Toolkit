// Package versioning 提供 Minecraft 版本号解析、比较与「适用版本」声明校验
// （TODO §2 版本化资料管理）。
//
// 支持三种「适用版本」声明形式：
//   - 单版本：1.21 / 1.21.1 / 1.21-pre1 / 1.21-rc2 / 24w14a
//   - 范围：1.16-1.17（闭区间）、1.18+（[1.18, ∞) 的开上界）
//   - 所有版本：all（等价写法 *），必须独占出现
//
// 设计取舍：
//   - 只覆盖本站资料实际会用到的写法；不做「是否为已发布版本」的联网判断，
//     避免把外部数据源的不确定性引入校验结果；
//   - 快照与正式版「不同族」，跨族比较仅保证排序稳定，不具备版本语义；
//   - 范围语法与单版本语法可能形似（如 1.21-pre1 含连字符），因此解析时
//     先尝试单版本，失败后才按范围解释。
package versioning

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind 版本（或其声明）类型。
type Kind string

const (
	KindRelease  Kind = "release"  // 正式版：1.21 / 1.21.1
	KindSnapshot Kind = "snapshot" // 快照：24w14a
	KindPre      Kind = "pre"      // 预发布：1.21-pre1
	KindRC       Kind = "rc"       // 候选版：1.21-rc1
	KindRange    Kind = "range"    // 范围声明：1.16-1.17 / 1.18+
	KindAll      Kind = "all"      // 「所有版本」通配声明：all（等价写法 *）
)

// AllKeyword 「所有版本」的规范写法；声明该值时表示适用于全部 MC 版本。
// 由于它是最宽泛的声明，必须独占出现（同时列出具体版本属于冗余，视为不合理）。
const AllKeyword = "all"

// CanonicalSpec 归一化单条「适用版本」声明：
// 去首尾空白；把 all / ALL / * 统一为 AllKeyword。
func CanonicalSpec(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "*" || strings.EqualFold(s, AllKeyword) {
		return AllKeyword
	}
	return s
}

// IsAll 判断声明列表是否包含「所有版本」通配。
func IsAll(versions []string) bool {
	for _, raw := range versions {
		if CanonicalSpec(raw) == AllKeyword {
			return true
		}
	}
	return false
}

var (
	reRelease  = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?(?:-(pre|rc)(\d+))?$`)
	reSnapshot = regexp.MustCompile(`^(\d{2})w(\d{2})([a-z])$`)
	reOpenMax  = regexp.MustCompile(`^(\S+)\+$`)
	reClosed   = regexp.MustCompile(`^(\S+?)\s*-\s*(\S+)$`)
)

// Version 解析后的单版本号。
type Version struct {
	Raw      string
	Kind     Kind
	Major    int    // release 族
	Minor    int    // release 族
	Patch    int    // release 族（可缺省）
	Stage    string // "" | "pre" | "rc"
	StageNum int    // 预发布 / 候选版序号
	Year     int    // snapshot：两位年份
	Week     int    // snapshot：周
	Letter   string // snapshot：修订字母
}

// Spec 一条「适用版本」声明：单版本或范围。
type Spec struct {
	Raw     string
	Kind    Kind    // 单版本时为该版本的类型；范围时为 KindRange
	Single  Version // Kind != KindRange 时有效
	Min     Version // 范围下界（含）
	Max     Version // 范围上界（含）；OpenMax 为 true 时无意义
	OpenMax bool    // true 表示 1.18+ 形式（无上界）
}

// Parse 解析单版本字符串；无法识别时返回错误。
func Parse(raw string) (Version, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Version{}, fmt.Errorf("版本号为空")
	}
	if m := reRelease.FindStringSubmatch(s); m != nil {
		v := Version{Raw: s, Kind: KindRelease}
		v.Major = atoi(m[1])
		v.Minor = atoi(m[2])
		if m[3] != "" {
			v.Patch = atoi(m[3])
		}
		switch m[4] {
		case "pre":
			v.Kind, v.Stage = KindPre, "pre"
		case "rc":
			v.Kind, v.Stage = KindRC, "rc"
		}
		if m[5] != "" {
			v.StageNum = atoi(m[5])
		}
		return v, nil
	}
	if m := reSnapshot.FindStringSubmatch(s); m != nil {
		return Version{
			Raw: s, Kind: KindSnapshot,
			Year: atoi(m[1]), Week: atoi(m[2]), Letter: m[3],
		}, nil
	}
	return Version{}, fmt.Errorf("无法识别的版本号 %q", raw)
}

// IsValid 判断字符串是否为合法**单版本**（用于 game_version 等单值字段）。
func IsValid(raw string) bool {
	_, err := Parse(raw)
	return err == nil
}

// ParseSpec 解析「适用版本」声明；单版本优先，其次按范围解释，最后识别「所有版本」通配。
func ParseSpec(raw string) (Spec, error) {
	s := CanonicalSpec(raw)
	if s == "" {
		return Spec{}, fmt.Errorf("版本号为空")
	}
	if s == AllKeyword {
		return Spec{Raw: AllKeyword, Kind: KindAll}, nil
	}
	if v, err := Parse(s); err == nil {
		return Spec{Raw: s, Kind: v.Kind, Single: v, Min: v, Max: v}, nil
	}
	if m := reOpenMax.FindStringSubmatch(s); m != nil {
		min, err := Parse(m[1])
		if err != nil {
			return Spec{}, fmt.Errorf("无法识别的版本号 %q", raw)
		}
		return Spec{Raw: s, Kind: KindRange, Min: min, OpenMax: true}, nil
	}
	if m := reClosed.FindStringSubmatch(s); m != nil {
		min, errMin := Parse(m[1])
		max, errMax := Parse(m[2])
		if errMin != nil || errMax != nil {
			return Spec{}, fmt.Errorf("无法识别的版本号 %q", raw)
		}
		if Compare(min, max) > 0 {
			return Spec{}, fmt.Errorf("版本范围起点大于终点：%s", s)
		}
		return Spec{Raw: s, Kind: KindRange, Min: min, Max: max}, nil
	}
	return Spec{}, fmt.Errorf("无法识别的版本号 %q", raw)
}

// IsValidSpec 判断字符串是否为合法的「适用版本」声明（含范围）。
func IsValidSpec(raw string) bool {
	_, err := ParseSpec(raw)
	return err == nil
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// stageRank 同一主版本内 release > rc > pre。
func stageRank(stage string) int {
	switch stage {
	case "pre":
		return 0
	case "rc":
		return 1
	default: // 正式版
		return 2
	}
}

// IsSnapshot 是否为快照版本。
func (v Version) IsSnapshot() bool { return v.Kind == KindSnapshot }

// Compare 返回 -1 / 0 / 1。
//
// 正式版族（release/pre/rc）之间按 主版本 → 次版本 → 修订号 → 阶段 → 阶段序号 比较；
// 快照与正式版「不同族」，跨族比较仅用于稳定排序（快照排在正式版之后），不具备版本语义。
func Compare(a, b Version) int {
	as, bs := a.IsSnapshot(), b.IsSnapshot()
	if as != bs {
		if as {
			return 1
		}
		return -1
	}
	if as {
		if c := cmpInt(a.Year, b.Year); c != 0 {
			return c
		}
		if c := cmpInt(a.Week, b.Week); c != 0 {
			return c
		}
		return strings.Compare(a.Letter, b.Letter)
	}
	if c := cmpInt(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpInt(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpInt(a.Patch, b.Patch); c != 0 {
		return c
	}
	if c := cmpInt(stageRank(a.Stage), stageRank(b.Stage)); c != 0 {
		return c
	}
	return cmpInt(a.StageNum, b.StageNum)
}

// CompareSpec 比较两条声明的排序位置（「所有版本」最靠前；其余按下界，同下界时范围声明更靠后）。
func CompareSpec(a, b Spec) int {
	if a.Kind == KindAll || b.Kind == KindAll {
		if a.Kind == b.Kind {
			return 0
		}
		if a.Kind == KindAll {
			return -1
		}
		return 1
	}
	if c := Compare(a.Min, b.Min); c != 0 {
		return c
	}
	if (a.Kind == KindRange) != (b.Kind == KindRange) {
		if a.Kind == KindRange {
			return 1
		}
		return -1
	}
	return strings.Compare(a.Raw, b.Raw)
}

// Validate 校验「适用版本」声明列表：
//   - 每一项都必须可解析（单版本、合法范围或「所有版本」通配）；
//   - 范围声明的起点不得大于终点；
//   - 不允许重复（按归一化后的原文比较）；
//   - 「所有版本」（all / *）必须独占出现（与具体版本并列属于冗余）。
//
// 空列表不视为错误（由调用方决定是否作为质量提示）。
func Validate(versions []string) []error {
	var errs []error
	seen := make(map[string]int, len(versions))
	allCount := 0
	for i, raw := range versions {
		s := CanonicalSpec(raw)
		if _, err := ParseSpec(s); err != nil {
			errs = append(errs, fmt.Errorf("第 %d 项：%v", i+1, err))
			continue
		}
		if s == AllKeyword {
			allCount++
		}
		if first, ok := seen[s]; ok {
			errs = append(errs, fmt.Errorf("第 %d 项与第 %d 项重复：%s", i+1, first+1, s))
			continue
		}
		seen[s] = i
	}
	if allCount > 0 && len(versions) > 1 {
		errs = append(errs, fmt.Errorf("声明「所有版本」（%s）时不能同时列出其他版本", AllKeyword))
	}
	return errs
}

// Sort 按声明排序（升序，「所有版本」在最前；无法解析的项保持相对顺序排在末尾，
// 避免因脏数据丢失条目）。输出为归一化写法（* → all）。
func Sort(versions []string) []string {
	type item struct {
		raw   string
		spec  Spec
		valid bool
	}
	items := make([]item, 0, len(versions))
	for _, raw := range versions {
		s := CanonicalSpec(raw)
		spec, err := ParseSpec(s)
		items = append(items, item{raw: s, spec: spec, valid: err == nil})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].valid != items[j].valid {
			return items[i].valid
		}
		if !items[i].valid {
			return false
		}
		return CompareSpec(items[i].spec, items[j].spec) < 0
	})
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.raw)
	}
	return out
}

// Satisfies 判断一条「适用版本」声明列表是否涵盖目标版本 target。
//
// target 可为具体版本（1.20 / 1.20.1 / 1.21-pre1）或「所有版本」通配（all / *）。
// 语义：
//   - target 为 all：仅当声明列表含通配 all 时命中；
//   - 声明含 all：对任何具体 target 均命中；
//   - 声明为单版本：与 target 主次版本一致（1.20 与 1.20.1 视为同一主版本线）或完全相同时命中；
//   - 声明为闭区间 A-B：A <= target <= B 时命中；
//   - 声明为开上界 A+：target >= A 时命中。
//
// 与本站既有的「声明文本匹配」（search.versionFilter / 资源列表）不同：本函数按版本
// **大小**比较，因此 target=1.20 能命中声明 1.8+，但不会命中声明 1.21+。药水与酿造
// 资料的版本适用范围用「自某版本起」表述，故用本函数做精确筛选（TODO §9A）。
// 无法解析的 target 或声明一律不命中（不猜测、不放宽）。
func Satisfies(versions []string, target string) bool {
	t := CanonicalSpec(target)
	if t == "" {
		return false
	}
	if t == AllKeyword {
		return IsAll(versions)
	}
	tv, err := Parse(t)
	if err != nil {
		return false
	}
	for _, raw := range versions {
		spec, err := ParseSpec(raw)
		if err != nil {
			continue
		}
		switch spec.Kind {
		case KindAll:
			return true
		case KindRange:
			// 下界不满足则不可能命中。
			if Compare(tv, spec.Min) < 0 {
				continue
			}
			if spec.OpenMax || Compare(tv, spec.Max) <= 0 {
				return true
			}
		default:
			// 单版本声明：同主版本线（主次版本一致）或完全一致即命中。
			if Compare(tv, spec.Single) == 0 {
				return true
			}
			if tv.Major == spec.Single.Major && tv.Minor == spec.Single.Minor {
				return true
			}
		}
	}
	return false
}
