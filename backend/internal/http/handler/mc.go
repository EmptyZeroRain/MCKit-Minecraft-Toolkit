package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"mckit/backend/internal/http/response"
	mcstructure "mckit/backend/internal/mcstructure"
)

// MCHandler 提供种子/结构/史莱姆区块等 Minecraft 工具（公开 API）。
type MCHandler struct{}

// NewMCHandler 创建 Minecraft 工具处理器。
func NewMCHandler() *MCHandler {
	return &MCHandler{}
}

// Structures POST /api/v1/mc/structures
// 计算指定种子下目标结构的理论坐标（可指定 region 范围）。
// 1.18+ 附群系可行性校验（feasible）。
func (h *MCHandler) Structures(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed    json.RawMessage `json:"seed"`
		Type    string          `json:"type"`
		Version string          `json:"version"`
		RegionX int             `json:"region_x"`
		RegionZ int             `json:"region_z"`
		CountX  int             `json:"count_x"`
		CountZ  int             `json:"count_z"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	seed, err := parseSeedValue(req.Seed)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "seed", Message: err.Error()}})
		return
	}

	st := mcstructure.Type(strings.TrimSpace(req.Type))
	if st == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "type", Message: "结构类型必填"}})
		return
	}
	if !mcstructure.IsKnown(st) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "type", Message: "未知结构类型: " + string(st)}})
		return
	}

	// 默认查询单个 region
	countX, countZ := req.CountX, req.CountZ
	if countX <= 0 {
		countX = 1
	}
	if countZ <= 0 {
		countZ = 1
	}
	// 限制查询规模，防止滥用
	if countX > 50 || countZ > 50 {
		response.Validation(w, r, []response.ErrorDetail{{Field: "count_x", Message: "范围过大（最大 50×50）"}})
		return
	}

	positions, err := mcstructure.PositionMulti(seed, st, req.RegionX, req.RegionZ, countX, countZ)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
		return
	}

	ver := mcstructure.NormalizeVersion(req.Version)

	// 版本支持：结构是否已加入该版本 + 分布算法是否与当前实现一致。
	available := mcstructure.AvailableIn(st, ver)
	placementOK := mcstructure.PlacementSupported(ver)
	desc, _ := mcstructure.Describe(st)

	// 独立超时：群系可行性校验（1.18+）逐点采样，点较多时限制总耗时。
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	sampler := newBiomeSampler(seed, ver)
	out := make([]structurePosDTO, 0, len(positions))
	timedOut := false
	for _, p := range positions {
		select {
		case <-ctx.Done():
			timedOut = true
		default:
		}
		if timedOut {
			break
		}
		out = append(out, newStructurePosDTO(sampler, st, p))
	}

	note := structureNote(available, placementOK)
	if timedOut {
		note += "；计算超时，仅返回已处理的部分候选点"
	}

	response.OK(w, map[string]any{
		"seed":                mcstructure.FormatSeed(seed),
		"type":                st,
		"version":             string(ver),
		"dimension":           desc.Dimension,
		"probability":         desc.Probability,
		"available":           available,
		"placement_supported": placementOK,
		"supported":           available && placementOK,
		"timed_out":           timedOut,
		"positions":           out,
		"note":                note,
	})
}

// structureNote 根据版本支持情况给出结果说明（明确「不支持 / 不可作为依据」的提示）。
func structureNote(available, placementOK bool) string {
	switch {
	case !available:
		return "该结构在所选版本区间尚未加入游戏，结果仅为占位；请改选其加入之后的版本"
	case !placementOK:
		return "1.12 及更早的结构分布算法与当前实现不同，候选坐标不可作为依据（未核实）"
	default:
		return "坐标为理论候选位置；1.18+ 已附群系可行性（feasible）：" +
			"valid=群系可行，shifted=邻近存在可行群系，invalid=邻近无合适群系，unknown=该结构无地表群系约束"
	}
}

// biomeSupported 判断版本区间是否支持生物群系查询（仅 1.18+）。
func biomeSupported(ver mcstructure.Version) bool {
	return ver == mcstructure.Ver1_18Plus
}

// structurePosDTO 结构候选坐标 + 群系可行性。
type structurePosDTO struct {
	X        int    `json:"x"`
	Z        int    `json:"z"`
	Biome    string `json:"biome,omitempty"`
	Feasible *bool  `json:"feasible,omitempty"`
	ShiftX   int    `json:"shift_x,omitempty"`
	ShiftZ   int    `json:"shift_z,omitempty"`
}

// newStructurePosDTO 由候选坐标构造 DTO 并附群系可行性。
func newStructurePosDTO(sampler mcstructure.BiomeSampler, t mcstructure.Type, p mcstructure.Pos) structurePosDTO {
	dto := structurePosDTO{X: p.X, Z: p.Z}
	res := mcstructure.CheckFeasibility(sampler, t, p)
	if res.Status == mcstructure.FeasibleUnknown {
		return dto
	}
	dto.Biome = res.Biome
	if res.Status == mcstructure.FeasibleShifted {
		dto.ShiftX, dto.ShiftZ = res.ShiftX, res.ShiftZ
	}
	feasible := res.Status != mcstructure.FeasibleInvalid
	dto.Feasible = boolPtr(feasible)
	return dto
}

// Strongholds POST /api/v1/mc/strongholds
// 估算前 N 个要塞位置。
func (h *MCHandler) Strongholds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed  json.RawMessage `json:"seed"`
		Count int             `json:"count"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	seed, err := parseSeedValue(req.Seed)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "seed", Message: err.Error()}})
		return
	}
	count := req.Count
	if count <= 0 || count > mcstructure.StrongholdCount {
		count = 3
	}
	positions := make([]mcstructure.Pos, 0, count)
	for i := 0; i < count; i++ {
		positions = append(positions, mcstructure.StrongholdPos(seed, i))
	}
	response.OK(w, map[string]any{
		"seed":      mcstructure.FormatSeed(seed),
		"count":     count,
		"positions": positions,
		"note":      "要塞坐标为环内估算，误差约 ±112 格；精确落点需生物群系检查",
	})
}

// SlimeChunks POST /api/v1/mc/slime-chunks
// 查询指定范围内的史莱姆区块。
func (h *MCHandler) SlimeChunks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed json.RawMessage `json:"seed"`
		MinX int             `json:"min_chunk_x"`
		MinZ int             `json:"min_chunk_z"`
		MaxX int             `json:"max_chunk_x"`
		MaxZ int             `json:"max_chunk_z"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	seed, err := parseSeedValue(req.Seed)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "seed", Message: err.Error()}})
		return
	}

	// 范围限制：最多 128×128 区块
	if req.MaxX < req.MinX {
		req.MaxX, req.MinX = req.MinX, req.MaxX
	}
	if req.MaxZ < req.MinZ {
		req.MaxZ, req.MinZ = req.MinZ, req.MaxZ
	}
	if req.MaxX-req.MinX > 128 || req.MaxZ-req.MinZ > 128 {
		response.Validation(w, r, []response.ErrorDetail{{Message: "查询范围过大（区块跨度最大 128）"}})
		return
	}

	type chunk struct {
		ChunkX int `json:"chunk_x"`
		ChunkZ int `json:"chunk_z"`
		X      int `json:"x"` // 方块坐标（区块中心）
		Z      int `json:"z"`
	}
	// 独立超时：128×128 = 16384 个区块的判定为同步计算，限制总耗时。
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var chunks []chunk
	timedOut := false
	for cx := req.MinX; cx <= req.MaxX && !timedOut; cx++ {
		select {
		case <-ctx.Done():
			timedOut = true
			continue
		default:
		}
		for cz := req.MinZ; cz <= req.MaxZ; cz++ {
			if mcstructure.SlimeChunk(seed, int64(cx), int64(cz)) {
				chunks = append(chunks, chunk{
					ChunkX: cx, ChunkZ: cz,
					X: cx*16 + 8, Z: cz*16 + 8,
				})
			}
		}
	}

	note := "仅 Java 版：Random(seed + 341873128712·cx + 132897987541·cz).nextInt(10)==0；基岩版算法不同"
	if timedOut {
		note += "；计算超时，结果为部分区域"
	}
	response.OK(w, map[string]any{
		"seed":      mcstructure.FormatSeed(seed),
		"count":     len(chunks),
		"chunks":    chunks,
		"timed_out": timedOut,
		"note":      note,
	})
}

// Capabilities POST /api/v1/mc/capabilities
// 返回世界分析工具各能力的版本支持矩阵（todo §10 版本支持梳理）。
func (h *MCHandler) Capabilities(w http.ResponseWriter, r *http.Request) {
	versions := mcstructure.AllVersions()
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		out = append(out, string(v))
	}
	response.OK(w, map[string]any{
		"versions": out,
		"features": mcstructure.Capabilities(),
	})
}

// Biome POST /api/v1/mc/biome
// 查询指定方块坐标处的地表生物群系（仅 1.18+ 支持）。
func (h *MCHandler) Biome(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed    json.RawMessage `json:"seed"`
		X       int             `json:"x"`
		Z       int             `json:"z"`
		Version string          `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	seed, err := parseSeedValue(req.Seed)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "seed", Message: err.Error()}})
		return
	}

	ver := mcstructure.NormalizeVersion(req.Version)
	sampler := newBiomeSampler(seed, ver)
	if sampler == nil {
		// 明确提示不支持，而非返回错误或伪造群系。
		response.OK(w, map[string]any{
			"seed":      mcstructure.FormatSeed(seed),
			"x":         req.X,
			"z":         req.Z,
			"version":   string(ver),
			"supported": false,
			"note":      "生物群系查询仅在 1.18+ 可用（服务端为 1.18+ 多噪声引擎移植）",
		})
		return
	}

	response.OK(w, map[string]any{
		"seed":      mcstructure.FormatSeed(seed),
		"x":         req.X,
		"z":         req.Z,
		"version":   string(ver),
		"supported": true,
		"biome":     sampler.BiomeAt(req.X, req.Z),
		"note":      "海平面附近地表采样（cubiomes SAMPLE_NO_DEPTH 语义）；坐标按 1:4 取 quart",
	})
}

// GenerateSeeds POST /api/v1/mc/seeds/generate
// 生成随机种子。
func (h *MCHandler) GenerateSeeds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Count int `json:"count"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req)
	}
	seeds, err := mcstructure.GenerateSeeds(req.Count)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	// 种子以字符串返回，避免 64 位整数在 JS 侧丢精度。
	response.OK(w, map[string]any{"seeds": mcstructure.FormatSeeds(seeds)})
}

// FindSeeds POST /api/v1/mc/seeds/find
// 搜索满足"结构在目标坐标附近"的种子。
func (h *MCHandler) FindSeeds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type        string `json:"type"`
		CenterX     int    `json:"center_x"`
		CenterZ     int    `json:"center_z"`
		Radius      int    `json:"radius"`
		Limit       int    `json:"limit"`
		MaxAttempts int    `json:"max_attempts"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	st := mcstructure.Type(strings.TrimSpace(req.Type))
	if !mcstructure.IsKnown(st) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "type", Message: "未知或不支持的结构类型"}})
		return
	}
	if req.Radius <= 0 {
		req.Radius = 256
	}
	// 限制搜索规模，防止滥用
	if req.MaxAttempts <= 0 || req.MaxAttempts > 500_000 {
		req.MaxAttempts = 500_000
	}

	// 搜索限时 5 秒，避免阻塞。
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	results, err := mcstructure.Find(ctx, mcstructure.FindRequest{
		Structure:   st,
		CenterX:     req.CenterX,
		CenterZ:     req.CenterZ,
		Radius:      req.Radius,
		Limit:       req.Limit,
		MaxAttempts: req.MaxAttempts,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			response.OK(w, map[string]any{"seeds": formatFoundSeeds(results), "note": "搜索超时，返回已找到的部分结果"})
			return
		}
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
		return
	}
	response.OK(w, map[string]any{"seeds": formatFoundSeeds(results)})
}

// foundSeedDTO 种子搜索结果（种子以字符串返回，避免 64 位精度丢失）。
type foundSeedDTO struct {
	Seed     string  `json:"seed"`
	X        int     `json:"x"`
	Z        int     `json:"z"`
	Distance float64 `json:"distance"`
}

// formatFoundSeeds 将搜索结果中的种子统一格式化为字符串。
func formatFoundSeeds(seeds []mcstructure.FoundSeed) []foundSeedDTO {
	out := make([]foundSeedDTO, 0, len(seeds))
	for _, s := range seeds {
		out = append(out, foundSeedDTO{
			Seed:     mcstructure.FormatSeed(s.Seed),
			X:        s.X,
			Z:        s.Z,
			Distance: s.Dist,
		})
	}
	return out
}

// ParseSeed POST /api/v1/mc/seeds/parse
// 将用户输入的种子文本解析为 Java 种子数值。
func (h *MCHandler) ParseSeed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed string `json:"seed"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	seed, err := mcstructure.ParseSeed(req.Seed)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "seed", Message: err.Error()}})
		return
	}
	response.OK(w, map[string]any{"seed": mcstructure.FormatSeed(seed)})
}

// ListStructures POST /api/v1/mc/structures/types
// 返回支持的结构类型清单与参数描述（版本、维度、频率门槛、群系约束）。
func (h *MCHandler) ListStructures(w http.ResponseWriter, r *http.Request) {
	versions := mcstructure.AllVersions()
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		out = append(out, string(v))
	}
	response.OK(w, map[string]any{
		"types":      mcstructure.ListKnown(),
		"structures": mcstructure.DescribeAll(),
		"versions":   out,
	})
}

// parseSeedValue 解析 JSON 种子字段（数字或字符串）。
func parseSeedValue(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, errors.New("种子必填")
	}
	// 先尝试数字
	var num int64
	if err := json.Unmarshal(raw, &num); err == nil {
		return num, nil
	}
	// 再尝试字符串（支持文本种子）
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return mcstructure.ParseSeed(str)
	}
	return 0, errors.New("种子格式非法")
}
