package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/resource"
)

// releaseRequest 单个发布版本请求体（TODO §6）。
// released_at 接受 RFC3339 或 YYYY-MM-DD；留空表示时间未知（不做猜测）。
type releaseRequest struct {
	VersionLabel string   `json:"version_label"`
	GameVersions []string `json:"game_versions"`
	LoaderTypes  []string `json:"loader_types"`
	SourceURL    string   `json:"source_url"`
	ReleasedAt   string   `json:"released_at"`
	Notes        string   `json:"notes"`
	IsLatest     bool     `json:"is_latest"`
}

// replaceReleasesRequest 整体替换某资源的发布版本。
type replaceReleasesRequest struct {
	Releases []releaseRequest `json:"releases"`
}

// ListResourceReleases POST /api/v1/admin/resources/releases/list
// 请求体 { "id": "<resource uuid>" }；返回该资源的发布版本列表。
func (h *AdminHandler) ListResourceReleases(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, r, "缺少资源 id")
		return
	}
	res, err := h.resources.GetByID(r.Context(), id)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}
	releases, err := h.resources.ListReleases(r.Context(), res.ID)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}
	response.OK(w, releases)
}

// ReplaceResourceReleases POST /api/v1/admin/resources/releases/replace
// 请求体 { "id": "<resource uuid>", "releases": [...] }。
// 以完整列表为准整体替换，天然幂等，避免残留陈旧版本。
func (h *AdminHandler) ReplaceResourceReleases(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.BadRequest(w, r, "缺少资源 id")
		return
	}

	var req replaceReleasesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	res, err := h.resources.GetByID(r.Context(), id)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}

	inputs := make([]resource.ReleaseInput, 0, len(req.Releases))
	for _, it := range req.Releases {
		inputs = append(inputs, resource.ReleaseInput{
			VersionLabel: it.VersionLabel,
			GameVersions: it.GameVersions,
			LoaderTypes:  it.LoaderTypes,
			SourceURL:    it.SourceURL,
			ReleasedAt:   it.ReleasedAt,
			Notes:        it.Notes,
			IsLatest:     it.IsLatest,
		})
	}

	releases, err := h.resources.ReplaceReleases(r.Context(), res.ID, inputs)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}

	h.auditLog(r, "resource.releases.replace", "resource", res.ID.String(), nil,
		map[string]any{"count": len(releases)})
	response.OK(w, releases)
}
