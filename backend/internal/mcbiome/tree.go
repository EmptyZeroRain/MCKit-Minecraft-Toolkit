package mcbiome

import (
	"embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// tablesFS 内嵌 cubiomes 的生物群系决策树数据（逐字节取自上游，保持可追溯）。
//
//go:embed tables/*.h
var tablesFS embed.FS

// version 表示 MC 版本，用于选择对应的决策树（值越大越新）。
type version int

// 支持 1.18+ 的决策树版本分界（与 cubiomes MCVersion 语义一致）。
const (
	v1_18 version = iota
	v1_19_2
	v1_19_4
	v1_20_6
	v1_21 // 1.21 Winter Drop
)

// biomeTree 复刻 cubiomes BiomeTree。
type biomeTree struct {
	steps  []uint32
	param  []int32
	nodes  []uint64
	order  uint32
	length uint32
}

var (
	treesOnce sync.Once
	trees     map[string]*biomeTree
	treesErr  error
)

// treeOrder 决策树按版本从新到旧的顺序。
var treeOrder = []struct {
	ver  version
	name string
}{
	{v1_21, "btree21wd"},
	{v1_20_6, "btree20"},
	{v1_19_4, "btree19"},
	{v1_19_2, "btree192"},
	{v1_18, "btree18"},
}

// treeFor 返回指定版本对应的决策树。
// 若目标版本的数据表缺失，则向下回退到最接近的可用版本（仍可工作）。
func treeFor(v version) *biomeTree {
	treesOnce.Do(loadTrees)
	if treesErr != nil {
		return nil
	}
	for _, o := range treeOrder {
		if v >= o.ver {
			if bt := trees[o.name]; bt != nil {
				return bt
			}
		}
	}
	for _, o := range treeOrder {
		if bt := trees[o.name]; bt != nil {
			return bt
		}
	}
	return nil
}

// loadTrees 解析全部内嵌决策树数据。
func loadTrees() {
	trees = make(map[string]*biomeTree)
	entries, err := tablesFS.ReadDir("tables")
	if err != nil {
		treesErr = err
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".h") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".h")
		raw, err := tablesFS.ReadFile("tables/" + e.Name())
		if err != nil {
			treesErr = err
			return
		}
		bt, err := parseTree(string(raw), name)
		if err != nil {
			treesErr = fmt.Errorf("解析 %s 失败: %w", e.Name(), err)
			return
		}
		trees[name] = bt
	}
}

var (
	reLineComment = regexp.MustCompile(`//[^\n]*`)
	reInts        = regexp.MustCompile(`-?\d+`)
	reHex         = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	reOrder       = regexp.MustCompile(`_order\s*=\s*(\d+)`)
)

// parseTree 解析单个 cubiomes 决策树头文件。
func parseTree(src, name string) (*biomeTree, error) {
	src = reLineComment.ReplaceAllString(src, "")

	orderM := reOrder.FindStringSubmatch(src)
	if orderM == nil {
		return nil, fmt.Errorf("缺少 %s_order", name)
	}
	order, err := strconv.ParseUint(orderM[1], 10, 32)
	if err != nil {
		return nil, err
	}

	stepsBlock, err := bracedBlock(src, name+"_steps")
	if err != nil {
		return nil, err
	}
	steps, err := parseUint32s(stepsBlock)
	if err != nil {
		return nil, err
	}

	paramBlock, err := braceSpan(src, name+"_param", "};")
	if err != nil {
		return nil, err
	}
	param, err := parseInt32s(paramBlock)
	if err != nil {
		return nil, err
	}

	nodesBlock, err := spanBlock(src, name+"_nodes", "};")
	if err != nil {
		return nil, err
	}
	nodes, err := parseHex(nodesBlock)
	if err != nil {
		return nil, err
	}

	bt := &biomeTree{
		steps:  steps,
		param:  param,
		nodes:  nodes,
		order:  uint32(order),
		length: uint32(len(nodes)),
	}
	if len(bt.steps) == 0 || len(bt.nodes) == 0 {
		return nil, fmt.Errorf("%s 数据为空", name)
	}
	return bt, nil
}

// bracedBlock 返回 marker 之后第一对花括号之间的内容。
func bracedBlock(src, marker string) (string, error) {
	s := strings.Index(src, marker)
	if s < 0 {
		return "", fmt.Errorf("未找到 %s", marker)
	}
	rest := src[s:]
	open := strings.IndexByte(rest, '{')
	if open < 0 {
		return "", fmt.Errorf("%s 缺少 '{'", marker)
	}
	close := strings.IndexByte(rest[open:], '}')
	if close < 0 {
		return "", fmt.Errorf("%s 缺少 '}'", marker)
	}
	return rest[open : open+close], nil
}

// braceSpan 返回 marker 之后第一个 '{' 到 endMarker 之间的内容。
func braceSpan(src, marker, endMarker string) (string, error) {
	s := strings.Index(src, marker)
	if s < 0 {
		return "", fmt.Errorf("未找到 %s", marker)
	}
	rest := src[s:]
	open := strings.IndexByte(rest, '{')
	if open < 0 {
		return "", fmt.Errorf("%s 缺少 '{'", marker)
	}
	rest = rest[open:]
	e := strings.Index(rest, endMarker)
	if e < 0 {
		return "", fmt.Errorf("%s 缺少结束标记 %q", marker, endMarker)
	}
	return rest[:e], nil
}

// spanBlock 返回从 marker 到 endMarker 之间的内容。
func spanBlock(src, marker, endMarker string) (string, error) {
	s := strings.Index(src, marker)
	if s < 0 {
		return "", fmt.Errorf("未找到 %s", marker)
	}
	rest := src[s:]
	e := strings.Index(rest, endMarker)
	if e < 0 {
		return "", fmt.Errorf("%s 缺少结束标记 %q", marker, endMarker)
	}
	return rest[:e], nil
}

func parseUint32s(s string) ([]uint32, error) {
	ms := reInts.FindAllString(s, -1)
	out := make([]uint32, 0, len(ms))
	for _, m := range ms {
		v, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, uint32(v))
	}
	return out, nil
}

func parseInt32s(s string) ([]int32, error) {
	ms := reInts.FindAllString(s, -1)
	out := make([]int32, 0, len(ms))
	for _, m := range ms {
		v, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, int32(v))
	}
	return out, nil
}

func parseHex(s string) ([]uint64, error) {
	ms := reHex.FindAllString(s, -1)
	out := make([]uint64, 0, len(ms))
	for _, m := range ms {
		v, err := strconv.ParseUint(m[2:], 16, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// paramIndex 返回节点的参数下标（对应 cubiomes node 的低 6 字节）。
func (bt *biomeTree) paramIndex(node uint64, i int) int {
	return int((node >> (8 * uint(i))) & 0xFF)
}

// npDist 复刻 cubiomes get_np_dist。
func (bt *biomeTree) npDist(np *[6]int64, idx int) uint64 {
	var ds uint64
	node := bt.nodes[idx]
	for i := 0; i < 6; i++ {
		pi := bt.paramIndex(node, i)
		a := np[i] - int64(bt.param[2*pi+1])
		b := int64(bt.param[2*pi+0]) - np[i]
		var d int64
		switch {
		case a > 0:
			d = a
		case b > 0:
			d = b
		default:
			d = 0
		}
		ds += uint64(d) * uint64(d)
	}
	return ds
}

// resultingNode 复刻 cubiomes get_resulting_node。
func (bt *biomeTree) resultingNode(np *[6]int64, idx, alt int, ds uint64, depth int) int {
	if bt.steps[depth] == 0 {
		return idx
	}
	var step uint32
	for {
		step = bt.steps[depth]
		depth++
		if idx+int(step) < int(bt.length) {
			break
		}
	}

	node := bt.nodes[idx]
	inner := int(node >> 48)
	leaf := alt

	for i, n := 0, int(bt.order); i < n; i++ {
		dsInner := bt.npDist(np, inner)
		if dsInner < ds {
			leaf2 := bt.resultingNode(np, inner, leaf, ds, depth)
			var dsLeaf2 uint64
			if inner == leaf2 {
				dsLeaf2 = dsInner
			} else {
				dsLeaf2 = bt.npDist(np, leaf2)
			}
			if dsLeaf2 < ds {
				ds = dsLeaf2
				leaf = leaf2
			}
		}
		inner += int(step)
		if inner >= int(bt.length) {
			break
		}
	}
	return leaf
}

// climateToBiome 复刻 cubiomes climateToBiome（独立采样，dat=nil）。
func climateToBiome(v version, np [6]int64) int {
	bt := treeFor(v)
	if bt == nil {
		return -1
	}
	idx := bt.resultingNode(&np, 0, 0, ^uint64(0), 0)
	return int((bt.nodes[idx] >> 48) & 0xFF)
}
