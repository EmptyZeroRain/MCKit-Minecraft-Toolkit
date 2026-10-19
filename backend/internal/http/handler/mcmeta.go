package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"mckit/backend/internal/blockids"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/mcversion"
)

// McMetaHandler 提供 MC 版本信息与物品 ID 对照表接口（移植自 CreeperCode）。
// 按项目契约：查询接口统一 POST，参数放 JSON body。
type McMetaHandler struct {
	client  *mcversion.Client
	entries []blockids.Entry
}

// NewMcMetaHandler 创建处理器。client 为 nil 时版本接口返回 503。
func NewMcMetaHandler(client *mcversion.Client, entries []blockids.Entry) *McMetaHandler {
	return &McMetaHandler{client: client, entries: entries}
}

// ListVersions POST /api/v1/versions
// body: {type?, q?}；type 支持 release/snapshot/old_beta/old_alpha/all，默认 all。
func (h *McMetaHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE", "版本信息源未配置", nil)
		return
	}
	var req struct {
		Type string `json:"type"`
		Q    string `json:"q"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	m, err := h.client.FetchManifest(r.Context())
	if err != nil {
		response.Error(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "获取版本清单失败", nil)
		return
	}

	typeFilter := strings.TrimSpace(req.Type)
	if typeFilter == "" {
		typeFilter = "all"
	}
	q := strings.ToLower(strings.TrimSpace(req.Q))

	versions := make([]mcversion.Entry, 0, len(m.Versions))
	for _, e := range m.Versions {
		if typeFilter != "all" && e.Type != typeFilter {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.ID), q) {
			continue
		}
		versions = append(versions, e)
	}

	response.OK(w, map[string]any{
		"latest":   m.Latest,
		"versions": versions,
	})
}

// VersionDetail POST /api/v1/versions/detail
// body: {id}；返回下载地址、启动命令、Java 要求、运行库。
func (h *McMetaHandler) VersionDetail(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE", "版本信息源未配置", nil)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "版本 id 必填"}})
		return
	}

	d, err := h.client.FetchVersion(r.Context(), id)
	if err != nil {
		// 版本不存在与网络故障区分开，方便前端提示。
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "缺少详情地址") {
			response.NotFound(w, r)
			return
		}
		response.Error(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "获取版本详情失败", nil)
		return
	}

	response.OK(w, mcversion.BuildDetail(d))
}

// BlockIDs POST /api/v1/block-ids
// body: {q?, kind?}；q 匹配命名空间 ID 或中文名，kind 为分类过滤，all/空 表示全部。
func (h *McMetaHandler) BlockIDs(w http.ResponseWriter, r *http.Request) {
	if h.entries == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "DATA_UNAVAILABLE", "物品对照表未加载", nil)
		return
	}
	var req struct {
		Q    string `json:"q"`
		Kind string `json:"kind"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	q := strings.ToLower(strings.TrimSpace(req.Q))
	kind := strings.TrimSpace(req.Kind)

	entries := make([]blockids.Entry, 0, len(h.entries))
	for _, e := range h.entries {
		if kind != "" && kind != "all" && e.Kind != kind {
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(e.ID), q) &&
			!strings.Contains(e.Name, q) {
			continue
		}
		entries = append(entries, e)
	}

	response.OK(w, map[string]any{
		"total":   len(entries),
		"entries": entries,
	})
}
