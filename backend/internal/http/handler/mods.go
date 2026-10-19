package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/catalog"
)

// Mods POST /api/v1/mods（Mod 列表与筛选，TODO §16）
// 只返回 review_status = published 的记录（内容审核流程）。
func (h *PublicHandler) Mods(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Loader   string `json:"loader"`
		Category string `json:"category"`
		Version  string `json:"version"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListMods(r.Context(), catalog.ModFilter{
		Query:     req.Q,
		Loader:    req.Loader,
		Category:  req.Category,
		MCVersion: req.Version,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: pages(total, pageSize),
	})
}

// ModDetail POST /api/v1/mods/:slug（Mod 详情：本体 + 版本 + 依赖与冲突 + 反向依赖，TODO §16）
// 只返回 review_status = published 的记录。
func (h *PublicHandler) ModDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.catalog.GetModBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// Modpacks POST /api/v1/modpacks（整合包列表与筛选，TODO §16）
func (h *PublicHandler) Modpacks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Loader   string `json:"loader"`
		Version  string `json:"version"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListModpacks(r.Context(), catalog.ModpackFilter{
		Query:     req.Q,
		Loader:    req.Loader,
		MCVersion: req.Version,
		Page:      req.Page,
		PageSize:  req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: pages(total, pageSize),
	})
}

// ModpackDetail POST /api/v1/modpacks/:slug（整合包详情：本体 + 包内 Mod 清单，TODO §16）
func (h *PublicHandler) ModpackDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.catalog.GetModpackBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// ModAdminHandler Mod 与整合包的审核处理器（TODO §16，admin 专属）。
// 内容审核流程：Mod / 整合包默认 pending，仅 published 对公开接口可见；
// 审核人取自登录会话（middleware.SessionFrom），并写入审计日志。
type ModAdminHandler struct {
	catalog  *catalog.Service
	auditSvc *audit.Service
}

// NewModAdminHandler 创建审核处理器。auditSvc 可为 nil（不写审计）。
func NewModAdminHandler(svc *catalog.Service, auditSvc *audit.Service) *ModAdminHandler {
	return &ModAdminHandler{catalog: svc, auditSvc: auditSvc}
}

type modListRequest struct {
	Q            string `json:"q"`
	ReviewStatus string `json:"review_status"`
	Loader       string `json:"loader"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
}

// AdminMods POST /api/v1/admin/mods/list
// 按审核状态筛选 Mod（不过滤发布态，供审核使用）。
func (h *ModAdminHandler) AdminMods(w http.ResponseWriter, r *http.Request) {
	var req modListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListModsForAdmin(r.Context(), catalog.ModAdminFilter{
		Query:        req.Q,
		ReviewStatus: req.ReviewStatus,
		Loader:       req.Loader,
		Page:         req.Page,
		PageSize:     req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

type modReviewRequest struct {
	ID           string `json:"id"`
	ReviewStatus string `json:"review_status"`
	Note         string `json:"note"`
}

// ReviewMod POST /api/v1/admin/mods/review
// 审核 Mod：更新审核状态并留痕（审核人取自登录会话）。
func (h *ModAdminHandler) ReviewMod(w http.ResponseWriter, r *http.Request) {
	var req modReviewRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	if req.ID == "" {
		response.BadRequest(w, r, "缺少 Mod ID")
		return
	}
	reviewer := actorName(r)
	m, err := h.catalog.ReviewMod(r.Context(), req.ID, req.ReviewStatus, reviewer, req.Note)
	if err != nil {
		writeReviewError(w, r, err)
		return
	}
	if h.auditSvc != nil {
		h.auditSvc.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "mod.review",
			ObjectType:   "mod",
			ObjectID:     m.ID.String(),
			After: map[string]any{
				"review_status": m.ReviewStatus,
				"reviewer":      m.Reviewer,
				"note":          m.ReviewNote,
			},
		})
	}
	response.OK(w, m)
}

// AdminModpacks POST /api/v1/admin/modpacks/list
// 按审核状态筛选整合包（不过滤发布态，供审核使用）。
func (h *ModAdminHandler) AdminModpacks(w http.ResponseWriter, r *http.Request) {
	var req modListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListModpacksForAdmin(r.Context(), catalog.ModpackAdminFilter{
		Query:        req.Q,
		ReviewStatus: req.ReviewStatus,
		Loader:       req.Loader,
		Page:         req.Page,
		PageSize:     req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

// ReviewModpack POST /api/v1/admin/modpacks/review
// 审核整合包：更新审核状态并留痕（审核人取自登录会话）。
func (h *ModAdminHandler) ReviewModpack(w http.ResponseWriter, r *http.Request) {
	var req modReviewRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	if req.ID == "" {
		response.BadRequest(w, r, "缺少整合包 ID")
		return
	}
	reviewer := actorName(r)
	pack, err := h.catalog.ReviewModpack(r.Context(), req.ID, req.ReviewStatus, reviewer, req.Note)
	if err != nil {
		writeReviewError(w, r, err)
		return
	}
	if h.auditSvc != nil {
		h.auditSvc.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "modpack.review",
			ObjectType:   "modpack",
			ObjectID:     pack.ID.String(),
			After: map[string]any{
				"review_status": pack.ReviewStatus,
				"reviewer":      pack.Reviewer,
				"note":          pack.ReviewNote,
			},
		})
	}
	response.OK(w, pack)
}

// writeReviewError 统一映射审核相关错误。
func writeReviewError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, catalog.ErrInvalidReviewStatus):
		response.BadRequest(w, r, "无效的审核状态（pending / published / rejected）")
	default:
		response.HandleError(w, r, err)
	}
}
