package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/nbt"
)

// LevelHandler 解析世界存档的 level.dat（基础信息 + 只读诊断）。
//
// 支持范围：Minecraft Java 版单人存档的 level.dat（gzip 压缩的 NBT，也兼容未压缩）。
// 仅读取基础字段，不在服务端保存文件、不修改文件、不提供自动修复。
type LevelHandler struct{}

// NewLevelHandler 创建 level.dat 处理器。
func NewLevelHandler() *LevelHandler { return &LevelHandler{} }

// maxLevelUpload 单次上传上限。level.dat 通常仅几 KB～数 MB。
const maxLevelUpload = 64 << 20 // 64 MiB

// levelWarning 一条诊断告警（不确定但值得提示的情况）。
type levelWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// worldVersion 存档版本信息（Data.Version）。
type worldVersion struct {
	ID       int32  `json:"id"`
	Name     string `json:"name,omitempty"`
	Snapshot bool   `json:"snapshot,omitempty"`
}

// worldInfo level.dat 的 Data 复合标签中提取的基础信息。
// 数值字段使用指针，以区分「字段缺失」与「真实值 0」。
type worldInfo struct {
	LevelName     *string           `json:"level_name,omitempty"`
	DataVersion   *int32            `json:"data_version,omitempty"`
	Version       *worldVersion     `json:"version,omitempty"`
	LastPlayedMS  *int64            `json:"last_played_ms,omitempty"`
	LastPlayed    string            `json:"last_played,omitempty"`
	RandomSeed    *string           `json:"random_seed,omitempty"`
	SpawnX        *int32            `json:"spawn_x,omitempty"`
	SpawnY        *int32            `json:"spawn_y,omitempty"`
	SpawnZ        *int32            `json:"spawn_z,omitempty"`
	Time          *int64            `json:"time,omitempty"`
	DayTime       *int64            `json:"day_time,omitempty"`
	GameType      *int32            `json:"game_type,omitempty"`
	Difficulty    *int8             `json:"difficulty,omitempty"`
	Hardcore      *bool             `json:"hardcore,omitempty"`
	AllowCommands *bool             `json:"allow_commands,omitempty"`
	GeneratorName *string           `json:"generator_name,omitempty"`
	GameRules     map[string]string `json:"game_rules,omitempty"`
}

// Parse POST /api/v1/mc/level/parse
// 上传 level.dat，返回世界基础信息与只读告警列表。
func (h *LevelHandler) Parse(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLevelUpload+4096)
	if err := r.ParseMultipartForm(maxLevelUpload); err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "文件过大或表单格式错误（上限 64MiB）"}})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "file", Message: "请上传 level.dat 文件"}})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxLevelUpload))
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "读取文件失败"}})
		return
	}
	if len(data) == 0 {
		response.Validation(w, r, []response.ErrorDetail{{Field: "file", Message: "文件为空"}})
		return
	}

	root, err := nbt.ParseFile(data)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{
			Field:   "file",
			Message: "无法解析 level.dat（需为 NBT 格式，通常为 gzip 压缩）：" + err.Error(),
		}})
		return
	}

	dataTag, ok := root["Data"].(map[string]any)
	if !ok {
		response.Validation(w, r, []response.ErrorDetail{{
			Field:   "file",
			Message: "level.dat 缺少顶层 Data 复合标签，可能不是有效存档",
		}})
		return
	}

	world, warnings := buildWorldInfo(dataTag)
	if !strings.HasSuffix(strings.ToLower(header.Filename), "level.dat") {
		warnings = append(warnings, levelWarning{
			Code:    "unexpected_file_name",
			Message: "文件名不是 level.dat，已按 level.dat 结构尝试解析",
		})
	}

	response.OK(w, map[string]any{
		"file_name":      header.Filename,
		"world":          world,
		"warnings":       warnings,
		"top_level_keys": sortedKeys(root),
		"note":           "仅解析基础信息与只读诊断，不修改存档、不提供自动修复。",
	})
}

// buildWorldInfo 从 Data 复合标签提取基础信息，并给出只读告警。
func buildWorldInfo(data map[string]any) (*worldInfo, []levelWarning) {
	wi := &worldInfo{}
	var warnings = []levelWarning{}

	if s, ok := getString(data, "LevelName"); ok {
		wi.LevelName = &s
	}
	if v, ok := getInt32(data, "DataVersion"); ok {
		wi.DataVersion = &v
	}
	if ver, ok := getCompound(data, "Version"); ok {
		wv := &worldVersion{}
		if id, ok := getInt32(ver, "Id"); ok {
			wv.ID = id
		}
		if name, ok := getString(ver, "Name"); ok {
			wv.Name = name
		}
		if snap, ok := getBoolByte(ver, "Snapshot"); ok {
			wv.Snapshot = snap
		}
		wi.Version = wv
	}
	if ms, ok := getInt64(data, "LastPlayed"); ok {
		wi.LastPlayedMS = &ms
		if ms > 0 {
			wi.LastPlayed = time.UnixMilli(ms).UTC().Format(time.RFC3339)
		}
	}
	if seed, ok := getInt64(data, "RandomSeed"); ok {
		// 以字符串返回，避免 int64 超过 2^53 时 JS 解析丢精度。
		s := strconv.FormatInt(seed, 10)
		wi.RandomSeed = &s
	}
	if x, ok := getInt32(data, "SpawnX"); ok {
		wi.SpawnX = &x
	}
	if y, ok := getInt32(data, "SpawnY"); ok {
		wi.SpawnY = &y
	}
	if z, ok := getInt32(data, "SpawnZ"); ok {
		wi.SpawnZ = &z
	}
	if t, ok := getInt64(data, "Time"); ok {
		wi.Time = &t
	}
	if t, ok := getInt64(data, "DayTime"); ok {
		wi.DayTime = &t
	}
	if gt, ok := getInt32(data, "GameType"); ok {
		wi.GameType = &gt
	}
	if d, ok := getInt8(data, "Difficulty"); ok {
		wi.Difficulty = &d
	}
	// 早期版本使用大写 Hardcore / allowCommands，这里两种键名都尝试。
	if hc, ok := getBoolByteAny(data, "hardcore", "Hardcore"); ok {
		wi.Hardcore = &hc
	}
	if ac, ok := getBoolByteAny(data, "allowCommands", "AllowCommands"); ok {
		wi.AllowCommands = &ac
	}
	if g, ok := getString(data, "generatorName"); ok {
		wi.GeneratorName = &g
	} else if g, ok := getString(data, "GeneratorName"); ok {
		wi.GeneratorName = &g
	}
	if rules, ok := getCompound(data, "GameRules"); ok {
		wi.GameRules = make(map[string]string, len(rules))
		for k, v := range rules {
			if s, ok := v.(string); ok {
				wi.GameRules[k] = s
			}
		}
	}

	// ---- 只读诊断（告警，非确定错误）----
	if wi.DataVersion == nil || *wi.DataVersion == 0 {
		warnings = append(warnings, levelWarning{
			Code:    "missing_data_version",
			Message: "缺少 DataVersion（可能来自极旧版本或文件不完整），版本相关判断不可靠",
		})
	}
	if wi.Version == nil {
		warnings = append(warnings, levelWarning{
			Code:    "missing_version",
			Message: "缺少 Version 字段（1.9 前的存档常见），无法给出精确版本名",
		})
	}
	if wi.LastPlayedMS == nil || *wi.LastPlayedMS <= 0 {
		warnings = append(warnings, levelWarning{
			Code:    "missing_last_played",
			Message: "LastPlayed 缺失或非正数，无法判断最近游玩时间",
		})
	}
	if wi.RandomSeed == nil {
		warnings = append(warnings, levelWarning{
			Code:    "missing_seed",
			Message: "缺少 RandomSeed，无法用于种子分析",
		})
	}
	if wi.SpawnY != nil && (*wi.SpawnY < -64 || *wi.SpawnY > 320) {
		warnings = append(warnings, levelWarning{
			Code:    "spawn_y_out_of_range",
			Message: "出生点 Y 超出主世界常见高度范围（-64 ~ 320），可能来自超平坦/模组或数据异常",
		})
	}
	if wi.GameRules == nil {
		warnings = append(warnings, levelWarning{
			Code:    "missing_game_rules",
			Message: "缺少 GameRules，可能来自极旧版本或文件不完整",
		})
	}

	return wi, warnings
}

// ---- NBT 取值辅助：全部做类型断言，缺失或类型不符时返回 ok=false ----

func getString(m map[string]any, key string) (string, bool) {
	v, ok := m[key].(string)
	return v, ok
}

func getInt8(m map[string]any, key string) (int8, bool) {
	v, ok := m[key].(int8)
	return v, ok
}

func getInt32(m map[string]any, key string) (int32, bool) {
	v, ok := m[key].(int32)
	return v, ok
}

func getInt64(m map[string]any, key string) (int64, bool) {
	v, ok := m[key].(int64)
	return v, ok
}

func getCompound(m map[string]any, key string) (map[string]any, bool) {
	v, ok := m[key].(map[string]any)
	return v, ok
}

// getBoolByte 读取以 TAG_Byte 存储的布尔（0/1）。
func getBoolByte(m map[string]any, key string) (bool, bool) {
	v, ok := m[key].(int8)
	if !ok {
		return false, false
	}
	return v != 0, true
}

// getBoolByteAny 依次尝试多个键名，命中第一个可解析的布尔值。
func getBoolByteAny(m map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		if v, ok := getBoolByte(m, k); ok {
			return v, true
		}
	}
	return false, false
}
