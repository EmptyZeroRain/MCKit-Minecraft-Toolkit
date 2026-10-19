package handler

import (
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/nbt"
	"mckit/backend/internal/region"
)

// RegionHandler 提供区块读取器（解析上传的 .mca region 文件）。
// 安全约束：仅内存解析、不落盘、限制文件大小、限制解压输出。
type RegionHandler struct {
	cache *regionCache
}

// NewRegionHandler 创建区块读取器处理器。
// maxCacheBytes 为 region_token 缓存总量上限（<=0 取默认 256MiB），
// cacheTTL 为缓存有效期（<=0 取默认 10 分钟）。
func NewRegionHandler(maxCacheBytes int64, cacheTTL time.Duration) *RegionHandler {
	return &RegionHandler{cache: newRegionCache(maxCacheBytes, cacheTTL)}
}

// maxRegionUpload 单次上传上限（区域文件通常 < 10MiB）。
const maxRegionUpload = 64 << 20 // 64 MiB

// regionParseTimeout 单次请求内逐区块解压/解析的总时限。
// 头部结构诊断只读扫描极快，不受此限；此处约束的是逐区块 NBT 读取。
const regionParseTimeout = 5 * time.Second

// Inspect POST /api/v1/mc/region/inspect
// 上传 .mca 文件，返回区块清单与顶层信息，并附带可复用的 region_token。
func (h *RegionHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	// 限制请求体大小
	r.Body = http.MaxBytesReader(w, r.Body, maxRegionUpload+4096)

	if err := r.ParseMultipartForm(maxRegionUpload); err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "文件过大或表单格式错误（上限 64MiB）"}})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "file", Message: "请上传 .mca 区域文件"}})
		return
	}
	defer file.Close()

	fileName := header.Filename
	regionX, regionZ, err := region.ParseRegionName(fileName)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "file", Message: err.Error()}})
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxRegionUpload))
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "读取文件失败"}})
		return
	}

	rf, err := region.Parse(data, regionX, regionZ)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
		return
	}

	// 解析每个区块的顶层 NBT（DataVersion / Status / xPos / zPos）
	type chunkInfo struct {
		region.ChunkEntry
		DataVersion int32  `json:"data_version,omitempty"`
		Status      string `json:"status,omitempty"`
	}
	chunks := make([]chunkInfo, 0, len(rf.Chunks))
	deadline := time.Now().Add(regionParseTimeout)
	timedOut := false
	for _, c := range rf.Chunks {
		if time.Now().After(deadline) {
			timedOut = true
			// 超时后仍保留已解析区块的结构信息，避免整单失败。
			chunks = append(chunks, chunkInfo{ChunkEntry: c})
			continue
		}
		info := chunkInfo{ChunkEntry: c}
		// 解压并读取顶层字段（失败则保留基础信息）
		if nbtData, _, err := region.ExtractChunkNBT(data, c.LocalX, c.LocalZ); err == nil {
			if top, err := nbt.Parse(nbtData); err == nil {
				if dv, ok := top["DataVersion"].(int32); ok {
					info.DataVersion = dv
				}
				if st, ok := top["Status"].(string); ok {
					info.Status = st
				}
			}
		}
		chunks = append(chunks, info)
	}

	// 只读结构诊断（不修改文件，不做自动修复）。
	diagnosis := region.Diagnose(data, rf.RegionX, rf.RegionZ)

	resp := map[string]any{
		"region_x":    rf.RegionX,
		"region_z":    rf.RegionZ,
		"chunk_count": len(chunks),
		"chunks":      chunks,
		"stats":       diagnosis.Stats,
		"diagnosis":   diagnosis,
		"timed_out":   timedOut,
	}
	// 缓存原始字节，返回一次性 token 供 /mc/region/chunk 复用，避免重复上传。
	if token := h.cache.put(data, rf.RegionX, rf.RegionZ); token != "" {
		resp["region_token"] = token
	}
	response.OK(w, resp)
}

// ChunkDetail POST /api/v1/mc/region/chunk
// 返回指定区块的完整顶层 NBT（键值），用于深入查看。
//
// 两种调用方式：
//  1. 免重传：携带 query 参数 region_token（由 /mc/region/inspect 返回）+
//     local_x/local_z，无需上传文件；
//  2. 兼容旧契约：multipart 上传 file + local_x/local_z。
func (h *RegionHandler) ChunkDetail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	localX := parseIntDefault(q.Get("local_x"), 0)
	localZ := parseIntDefault(q.Get("local_z"), 0)

	// 方式 1：复用 inspect 缓存的区域文件（避免重传整个 .mca）。
	if token := strings.TrimSpace(q.Get("region_token")); token != "" {
		e, ok := h.cache.get(token)
		if !ok {
			response.Validation(w, r, []response.ErrorDetail{{Field: "region_token", Message: "region_token 无效或已过期，请重新上传文件"}})
			return
		}
		h.writeChunkDetail(w, r, e.data, e.regionX, e.regionZ, localX, localZ)
		return
	}

	// 方式 2：multipart 上传（向后兼容）。
	r.Body = http.MaxBytesReader(w, r.Body, maxRegionUpload+4096)
	if err := r.ParseMultipartForm(maxRegionUpload); err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "文件过大或表单格式错误"}})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "file", Message: "请上传 .mca 区域文件"}})
		return
	}
	defer file.Close()

	localX = parseIntDefault(r.FormValue("local_x"), localX)
	localZ = parseIntDefault(r.FormValue("local_z"), localZ)

	regionX, regionZ, err := region.ParseRegionName(header.Filename)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Field: "file", Message: err.Error()}})
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxRegionUpload))
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "读取文件失败"}})
		return
	}

	h.writeChunkDetail(w, r, data, regionX, regionZ, localX, localZ)
}

// writeChunkDetail 从区域文件字节中提取指定本地区块的顶层 NBT 摘要。
func (h *RegionHandler) writeChunkDetail(
	w http.ResponseWriter, r *http.Request,
	data []byte, regionX, regionZ, localX, localZ int,
) {
	nbtData, compression, err := region.ExtractChunkNBT(data, localX, localZ)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
		return
	}

	top, err := nbt.Parse(nbtData)
	if err != nil {
		response.Validation(w, r, []response.ErrorDetail{{Message: "NBT 解析失败: " + err.Error()}})
		return
	}

	// 只返回常见标量字段，避免响应过大
	summary := map[string]any{}
	for _, key := range []string{"DataVersion", "xPos", "zPos", "Status", "LastUpdate", "InhabitedTime"} {
		if v, ok := top[key]; ok {
			summary[key] = v
		}
	}

	response.OK(w, map[string]any{
		"region_x":       regionX,
		"region_z":       regionZ,
		"local_x":        localX,
		"local_z":        localZ,
		"global_chunk_x": regionX*32 + localX,
		"global_chunk_z": regionZ*32 + localZ,
		"compression":    compression,
		"fields":         summary,
		"top_level_keys": sortedKeys(top),
	})
}

func parseIntDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 64 {
		keys = keys[:64]
	}
	return keys
}
