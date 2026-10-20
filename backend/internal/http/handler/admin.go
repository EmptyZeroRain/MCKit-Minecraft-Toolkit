// Package handler 提供 HTTP 处理器。
package handler

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/content"
	"mckit/backend/internal/service/resource"
)

// AdminHandler 管理端 CRUD 处理器（需求 §8.4）。
type AdminHandler struct {
	resources   *resource.Service
	contentSvc  *content.Service
	auditSvc    *audit.Service
	linkChecker linkChecker
	// linkLimiter 链接检查限流（需求 §5.8 第 8 条：对检查频率限流）。
	linkLimiter *keyedLimiter
	// feedbackLimiter 公开反馈限流（需求 §5.10：服务端限流与反垃圾）。
	feedbackLimiter *keyedLimiter
}

// LinkCheckResult 链接检查结果（需求 §5.8：记录 HTTP 状态与最终地址）。
type LinkCheckResult struct {
	Status     string
	StatusCode int
	FinalURL   string
}

// linkChecker 链接检查接口（由 main 注入实现）。
type linkChecker interface {
	CheckURL(rawURL string) (LinkCheckResult, error)
}

// NewAdminHandler 创建管理端处理器。
func NewAdminHandler(resources *resource.Service, contentSvc *content.Service, auditSvc *audit.Service) *AdminHandler {
	return &AdminHandler{
		resources:  resources,
		contentSvc: contentSvc,
		auditSvc:   auditSvc,
		// 每个操作者每 6 秒最多 1 次链接检查（突发 3 次），避免被当作探测跳板。
		linkLimiter: newKeyedLimiter(rate.Every(6*time.Second), 3, 10*time.Minute),
		// 每个 IP 每 30 秒最多 1 条反馈（突发 5 条），降低垃圾提交。
		feedbackLimiter: newKeyedLimiter(rate.Every(30*time.Second), 5, 30*time.Minute),
	}
}

// SetLinkChecker 注入链接检查器。
func (h *AdminHandler) SetLinkChecker(lc linkChecker) {
	h.linkChecker = lc
}

// actorID 从会话读取操作者账号 ID（统一账号，含后台账号与普通用户）。
func actorID(r *http.Request) string {
	s := middleware.SessionFrom(r.Context())
	if s == nil {
		return ""
	}
	return s.AccountID
}

// actorName 返回当前管理员的用户名（用于审核留痕；未登录返回空串）。
func actorName(r *http.Request) string {
	s := middleware.SessionFrom(r.Context())
	if s == nil {
		return ""
	}
	return s.Username
}

// ---- 资源 CRUD ----

// resourceCreateRequest 创建资源请求体。
type resourceCreateRequest struct {
	Slug              string          `json:"slug"`
	Name              string          `json:"name"`
	Summary           string          `json:"summary"`
	Description       string          `json:"description"`
	ResourceType      string          `json:"resource_type"`
	CoverImageURL     string          `json:"cover_image_url"`
	AuthorName        string          `json:"author_name"`
	SourcePlatform    string          `json:"source_platform"`
	SourceURL         string          `json:"source_url"`
	License           string          `json:"license"`
	GameEdition       string          `json:"game_edition"`
	SupportedVersions []string        `json:"supported_versions"`
	LoaderTypes       []string        `json:"loader_types"`
	Dependencies      json.RawMessage `json:"dependencies"`
	InstallGuide      string          `json:"install_guide"`
	Tags              []string        `json:"tags"`
	CompatibilityNote string          `json:"compatibility_note"`
	Status            string          `json:"status"`
}

// CreateResource POST /api/v1/admin/resources
func (h *AdminHandler) CreateResource(w http.ResponseWriter, r *http.Request) {
	var req resourceCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	res, err := h.resources.Create(r.Context(), &resource.CreateInput{
		Slug:              req.Slug,
		Name:              req.Name,
		Summary:           req.Summary,
		Description:       req.Description,
		ResourceType:      req.ResourceType,
		CoverImageURL:     req.CoverImageURL,
		AuthorName:        req.AuthorName,
		SourcePlatform:    req.SourcePlatform,
		SourceURL:         req.SourceURL,
		License:           req.License,
		GameEdition:       req.GameEdition,
		SupportedVersions: req.SupportedVersions,
		LoaderTypes:       req.LoaderTypes,
		Dependencies:      req.Dependencies,
		InstallGuide:      req.InstallGuide,
		Tags:              req.Tags,
		CompatibilityNote: req.CompatibilityNote,
		Status:            req.Status,
		CreatedBy:         actorID(r),
		ActorName:         actorName(r),
	})
	if err != nil {
		writeResourceError(w, r, err)
		return
	}

	h.auditLog(r, "resource.create", "resource", res.ID.String(), nil, summaryMap(res.Name, req.ResourceType))

	response.Created(w, res)
}

// listResourcesRequest 资源列表筛选。
type listResourcesRequest struct {
	Q              string `json:"q"`
	Type           string `json:"type"`
	GameVersion    string `json:"game_version"`
	Loader         string `json:"loader"`
	SourcePlatform string `json:"source_platform"`
	Tag            string `json:"tag"`
	Status         string `json:"status"`
	Sort           string `json:"sort"`
	Page           int    `json:"page"`
	PageSize       int    `json:"page_size"`
}

// ListResources POST /api/v1/admin/resources（管理端列表）
func (h *AdminHandler) ListResources(w http.ResponseWriter, r *http.Request) {
	var req listResourcesRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	items, total, err := h.resources.List(r.Context(), resource.ListFilter{
		Q:              req.Q,
		ResourceType:   req.Type,
		GameVersion:    req.GameVersion,
		Loader:         req.Loader,
		SourcePlatform: req.SourcePlatform,
		Tag:            req.Tag,
		Status:         req.Status,
		Sort:           req.Sort,
		Page:           req.Page,
		PageSize:       req.PageSize,
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
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	})
}

// ExportResources POST /api/v1/admin/resources/export
//
// 按与列表相同的筛选条件导出 CSV（不分页，最多 resource.ExportMax 条）。
// 导出属于「批量内容处理」的低风险形态：只读、可审计、不引入写入放大，
// 因此先于「批量在线编辑」落地（见 docs/批量编辑与CSV导入导出评估.md）。
func (h *AdminHandler) ExportResources(w http.ResponseWriter, r *http.Request) {
	var req listResourcesRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}

	items, truncated, err := h.resources.Export(r.Context(), resource.ListFilter{
		Q:              req.Q,
		ResourceType:   req.Type,
		GameVersion:    req.GameVersion,
		Loader:         req.Loader,
		SourcePlatform: req.SourcePlatform,
		Tag:            req.Tag,
		Status:         req.Status,
		Sort:           req.Sort,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	filename := "resources-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Export-Truncated", strconv.FormatBool(truncated))
	w.WriteHeader(http.StatusOK)

	// 前置 UTF-8 BOM，保证 Excel 正确识别中文。
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"ID", "名称", "Slug", "类型", "状态", "游戏版本类型", "来源平台",
		"加载器", "支持版本", "标签", "来源链接", "更新时间",
	})
	for _, it := range items {
		_ = cw.Write([]string{
			it.ID.String(),
			csvSafe(it.Name),
			csvSafe(it.Slug),
			csvSafe(it.ResourceType),
			csvSafe(it.Status),
			csvSafe(it.GameEdition),
			csvSafe(it.SourcePlatform),
			csvSafe(strings.Join(it.LoaderTypes, "|")),
			csvSafe(strings.Join(it.SupportedVersions, "|")),
			csvSafe(strings.Join(it.Tags, "|")),
			csvSafe(it.SourceURL),
			it.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	cw.Flush()

	if h.auditSvc != nil {
		h.auditSvc.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "resource.export",
			ObjectType:   "resource_list",
			After: map[string]any{
				"count":     len(items),
				"truncated": truncated,
				"q":         req.Q,
				"type":      req.Type,
			},
		})
	}
}

// ResourceDetail POST /api/v1/admin/resources/:id
// 返回资源本体 + 发布版本 + 兼容性结论，供后台编辑页直接渲染（TODO §6）。
func (h *AdminHandler) ResourceDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.resources.GetDetailByID(r.Context(), id)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// UpdateResource PATCH /api/v1/admin/resources/:id
func (h *AdminHandler) UpdateResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req resourceCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	in := &resource.UpdateInput{
		Slug:              strPtr(req.Slug),
		Name:              strPtr(req.Name),
		Summary:           strPtr(req.Summary),
		Description:       strPtr(req.Description),
		ResourceType:      strPtr(req.ResourceType),
		CoverImageURL:     strPtr(req.CoverImageURL),
		AuthorName:        strPtr(req.AuthorName),
		SourcePlatform:    strPtr(req.SourcePlatform),
		SourceURL:         strPtr(req.SourceURL),
		License:           strPtr(req.License),
		GameEdition:       strPtr(req.GameEdition),
		SupportedVersions: sPtr(req.SupportedVersions),
		LoaderTypes:       sPtr(req.LoaderTypes),
		Dependencies:      req.Dependencies,
		InstallGuide:      strPtr(req.InstallGuide),
		Tags:              sPtr(req.Tags),
		CompatibilityNote: strPtr(req.CompatibilityNote),
		Status:            strPtr(req.Status),
		UpdatedBy:         actorID(r),
		ActorName:         actorName(r),
	}
	res, err := h.resources.Update(r.Context(), id, in)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}

	h.auditLog(r, "resource.update", "resource", id, nil, summaryMap(res.Name, res.ResourceType))
	response.OK(w, res)
}

// UnpublishResource POST /api/v1/admin/resources/unpublish（下架，保留后台记录）
func (h *AdminHandler) UnpublishResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.resources.UnpublishByID(r.Context(), id); err != nil {
		writeResourceError(w, r, err)
		return
	}
	h.auditLog(r, "resource.unpublish", "resource", id, nil, nil)
	response.NoContent(w)
}

// RemoveResource DELETE /api/v1/admin/resources/:id（软删除）
func (h *AdminHandler) RemoveResource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.resources.RemoveByID(r.Context(), id); err != nil {
		writeResourceError(w, r, err)
		return
	}
	h.auditLog(r, "resource.remove", "resource", id, nil, nil)
	response.NoContent(w)
}

// CheckResourceLink POST /api/v1/admin/resources/:id/check-link
// 需求 §5.8：仅检查可达性；失败只更新检查状态，不删除资源；
// 记录操作者、URL、HTTP 状态与结果。
func (h *AdminHandler) CheckResourceLink(w http.ResponseWriter, r *http.Request) {
	if h.linkChecker == nil {
		response.HandleError(w, r, errors.New("链接检查器未配置"))
		return
	}
	id := chi.URLParam(r, "id")
	res, err := h.resources.GetByID(r.Context(), id)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}

	if !h.linkLimiter.allow(actorID(r)) {
		response.TooManyRequests(w, r)
		return
	}

	result, checkErr := h.linkChecker.CheckURL(res.SourceURL)
	status := result.Status
	message := ""
	if checkErr != nil {
		// 未给出分类时按网络错误处理；SSRF 拦截由适配器标记为 blocked。
		if status == "" {
			status = domain.LinkStatusError
		}
		if status == domain.LinkStatusBlocked {
			message = "链接不符合安全要求，已拒绝检查"
		} else {
			message = "链接检查失败，请稍后重试"
		}
	}

	// 无论成功失败都更新检查状态（§5.8 第 10 条），资源本身保持不动。
	checkedAt := time.Now()
	_ = h.resources.UpdateLinkStatus(r.Context(), id, status, checkedAt)

	after := map[string]any{
		"url":         res.SourceURL,
		"link_status": status,
		"status_code": result.StatusCode,
		"final_url":   result.FinalURL,
	}
	if checkErr != nil {
		after["result"] = "failure"
	} else {
		after["result"] = "success"
	}
	h.auditLog(r, "resource.check_link", "resource", id, nil, after)

	body := map[string]any{
		"link_status":     status,
		"status_code":     result.StatusCode,
		"final_url":       result.FinalURL,
		"last_checked_at": checkedAt.Format(time.RFC3339),
	}
	if message != "" {
		body["message"] = message
	}
	response.OK(w, body)
}

// ---- 工具 CRUD ----

// toolRequest 工具请求体。
type toolRequest struct {
	Slug              string          `json:"slug"`
	Name              string          `json:"name"`
	Summary           string          `json:"summary"`
	Description       string          `json:"description"`
	Category          string          `json:"category"`
	Icon              string          `json:"icon"`
	Tags              []string        `json:"tags"`
	SupportedVersions []string        `json:"supported_versions"`
	InputSchema       json.RawMessage `json:"input_schema"`
	OutputSchema      json.RawMessage `json:"output_schema"`
	IsFeatured        bool            `json:"is_featured"`
	SortOrder         int             `json:"sort_order"`
	Status            string          `json:"status"`
}

// CreateTool POST /api/v1/admin/tools
func (h *AdminHandler) CreateTool(w http.ResponseWriter, r *http.Request) {
	var req toolRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	tool, err := h.contentSvc.CreateTool(r.Context(), &content.ToolInput{
		Slug:              req.Slug,
		Name:              req.Name,
		Summary:           req.Summary,
		Description:       req.Description,
		Category:          req.Category,
		Icon:              req.Icon,
		Tags:              req.Tags,
		SupportedVersions: req.SupportedVersions,
		InputSchema:       req.InputSchema,
		OutputSchema:      req.OutputSchema,
		IsFeatured:        req.IsFeatured,
		SortOrder:         req.SortOrder,
		Status:            req.Status,
		ActorID:           actorID(r),
		ActorName:         actorName(r),
	})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "tool.create", "tool", tool.ID.String(), nil, summaryMap(tool.Name, tool.Category))
	response.Created(w, tool)
}

// ListToolsRequest 工具列表请求。
type ListToolsRequest struct {
	Category      string `json:"category"`
	Q             string `json:"q"`
	Tag           string `json:"tag"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	PublishedOnly bool   `json:"-"`
}

// ListTools POST /api/v1/admin/tools
func (h *AdminHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	var req ListToolsRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.contentSvc.ListTools(r.Context(), content.ToolFilter{
		Category:      req.Category,
		Q:             req.Q,
		Tag:           req.Tag,
		PublishedOnly: false,
		Page:          req.Page,
		PageSize:      req.PageSize,
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
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	})
}

// ToolDetail POST /api/v1/admin/tools/:id
func (h *AdminHandler) ToolDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tool, err := h.contentSvc.GetToolByID(r.Context(), id)
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	response.OK(w, tool)
}

// UpdateTool PATCH /api/v1/admin/tools/:id
func (h *AdminHandler) UpdateTool(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req toolRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	tool, err := h.contentSvc.UpdateTool(r.Context(), id, &content.ToolInput{
		Slug:              req.Slug,
		Name:              req.Name,
		Summary:           req.Summary,
		Description:       req.Description,
		Category:          req.Category,
		Icon:              req.Icon,
		Tags:              req.Tags,
		SupportedVersions: req.SupportedVersions,
		InputSchema:       req.InputSchema,
		OutputSchema:      req.OutputSchema,
		IsFeatured:        req.IsFeatured,
		SortOrder:         req.SortOrder,
		Status:            req.Status,
		ActorID:           actorID(r),
		ActorName:         actorName(r),
	})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "tool.update", "tool", id, nil, summaryMap(tool.Name, tool.Category))
	response.OK(w, tool)
}

// RemoveTool DELETE /api/v1/admin/tools/:id
func (h *AdminHandler) RemoveTool(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.contentSvc.RemoveTool(r.Context(), id); err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "tool.remove", "tool", id, nil, nil)
	response.NoContent(w)
}

// ---- 百科 CRUD ----

// wikiRequest 百科请求体。
type wikiRequest struct {
	Slug             string          `json:"slug"`
	Title            string          `json:"title"`
	Summary          string          `json:"summary"`
	Content          string          `json:"content"`
	ContentFormat    string          `json:"content_format"`
	ContentType      string          `json:"content_type"`
	GameEdition      string          `json:"game_edition"`
	GameVersions     []string        `json:"game_versions"`
	Tags             []string        `json:"tags"`
	SourceReferences json.RawMessage `json:"source_references"`
	Status           string          `json:"status"`
}

// CreateWiki POST /api/v1/admin/wiki
func (h *AdminHandler) CreateWiki(w http.ResponseWriter, r *http.Request) {
	var req wikiRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	entry, err := h.contentSvc.CreateWiki(r.Context(), &content.WikiInput{
		Slug:             req.Slug,
		Title:            req.Title,
		Summary:          req.Summary,
		Content:          req.Content,
		ContentFormat:    req.ContentFormat,
		ContentType:      req.ContentType,
		GameEdition:      req.GameEdition,
		GameVersions:     req.GameVersions,
		Tags:             req.Tags,
		SourceReferences: req.SourceReferences,
		Status:           req.Status,
		ActorName:        actorName(r),
	}, actorID(r))
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "wiki.create", "wiki_entry", entry.ID.String(), nil, summaryMap(entry.Title, entry.ContentType))
	response.Created(w, entry)
}

// ListWiki POST /api/v1/admin/wiki
func (h *AdminHandler) ListWiki(w http.ResponseWriter, r *http.Request) {
	var req ListToolsRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.contentSvc.ListWiki(r.Context(), content.ToolFilter{
		Q:             req.Q,
		Tag:           req.Tag,
		PublishedOnly: false,
		Page:          req.Page,
		PageSize:      req.PageSize,
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
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	})
}

// WikiDetail POST /api/v1/admin/wiki/:id
func (h *AdminHandler) WikiDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	entry, err := h.contentSvc.GetWikiByID(r.Context(), id)
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	response.OK(w, entry)
}

// UpdateWiki PATCH /api/v1/admin/wiki/:id
func (h *AdminHandler) UpdateWiki(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req wikiRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	entry, err := h.contentSvc.UpdateWiki(r.Context(), id, &content.WikiInput{
		Slug:             req.Slug,
		Title:            req.Title,
		Summary:          req.Summary,
		Content:          req.Content,
		ContentFormat:    req.ContentFormat,
		ContentType:      req.ContentType,
		GameEdition:      req.GameEdition,
		GameVersions:     req.GameVersions,
		Tags:             req.Tags,
		SourceReferences: req.SourceReferences,
		Status:           req.Status,
		ActorName:        actorName(r),
	}, actorID(r))
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "wiki.update", "wiki_entry", id, nil, summaryMap(entry.Title, entry.ContentType))
	response.OK(w, entry)
}

// RemoveWiki DELETE /api/v1/admin/wiki/:id
func (h *AdminHandler) RemoveWiki(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.contentSvc.RemoveWiki(r.Context(), id); err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "wiki.remove", "wiki_entry", id, nil, nil)
	response.NoContent(w)
}

// ---- 分类与标签 ----

// CreateCategory POST /api/v1/admin/categories
func (h *AdminHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		ContentType string `json:"content_type"`
		SortOrder   int    `json:"sort_order"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	cat, err := h.contentSvc.CreateCategory(r.Context(), &content.CategoryInput{
		Name:        req.Name,
		Slug:        req.Slug,
		ContentType: req.ContentType,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "category.create", "category", cat.ID.String(), nil, nil)
	response.Created(w, cat)
}

// ListCategories POST /api/v1/admin/categories 与公开 POST /api/v1/categories 共用
func (h *AdminHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContentType string `json:"content_type"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, err := h.contentSvc.ListCategories(r.Context(), req.ContentType)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, items)
}

// CreateTag POST /api/v1/admin/tags
func (h *AdminHandler) CreateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	tag, err := h.contentSvc.CreateTag(r.Context(), &content.TagInput{Name: req.Name, Slug: req.Slug})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "tag.create", "tag", tag.ID.String(), nil, nil)
	response.Created(w, tag)
}

// ListTags POST /api/v1/admin/tags 与公开 /api/v1/tags 共用
func (h *AdminHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q string `json:"q"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, err := h.contentSvc.ListTags(r.Context(), req.Q)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, items)
}

// TagUsage POST /api/v1/admin/tags/usage
// 返回标签使用统计（跨资源 / 工具 / 百科，来自关联表聚合）。
// body: {limit?}（默认 200，按使用次数降序）。
func (h *AdminHandler) TagUsage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Limit int `json:"limit"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	rows, err := h.contentSvc.TagUsage(r.Context(), req.Limit)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rows)
}

// UpdateCategory POST /api/v1/admin/categories/update
func (h *AdminHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		ContentType string `json:"content_type"`
		SortOrder   int    `json:"sort_order"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if req.ID == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "分类 id 必填"}})
		return
	}
	cat, err := h.contentSvc.UpdateCategory(r.Context(), req.ID, &content.CategoryInput{
		Name:        req.Name,
		Slug:        req.Slug,
		ContentType: req.ContentType,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "category.update", "category", req.ID, nil, nil)
	response.OK(w, cat)
}

// DeleteCategory POST /api/v1/admin/categories/delete
func (h *AdminHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := h.contentSvc.DeleteCategory(r.Context(), id); err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "category.delete", "category", id, nil, nil)
	response.NoContent(w)
}

// UpdateTag POST /api/v1/admin/tags/update
func (h *AdminHandler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if req.ID == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "标签 id 必填"}})
		return
	}
	tag, err := h.contentSvc.UpdateTag(r.Context(), req.ID, &content.TagInput{Name: req.Name, Slug: req.Slug})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "tag.update", "tag", req.ID, nil, nil)
	response.OK(w, tag)
}

// DeleteTag POST /api/v1/admin/tags/delete
func (h *AdminHandler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := decodeID(w, r)
	if !ok {
		return
	}
	if err := h.contentSvc.DeleteTag(r.Context(), id); err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "tag.delete", "tag", id, nil, nil)
	response.NoContent(w)
}

// ---- 反馈 ----

// ListFeedback POST /api/v1/admin/feedback
func (h *AdminHandler) ListFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status       string `json:"status"`
		FeedbackType string `json:"feedback_type"`
		Page         int    `json:"page"`
		PageSize     int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.contentSvc.ListFeedback(r.Context(), req.Status, req.FeedbackType, req.Page, req.PageSize)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)),
	})
}

// UpdateFeedback PATCH /api/v1/admin/feedback/:id（TODO §17：审核并记录审核人与变更历史）
func (h *AdminHandler) UpdateFeedback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	reviewer := actorName(r)
	fb, err := h.contentSvc.ReviewFeedback(r.Context(), id, req.Status, reviewer, req.Note)
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	h.auditLog(r, "feedback.review", "feedback", id, nil, map[string]any{
		"status": fb.Status, "reviewer": fb.Reviewer,
	})
	response.OK(w, fb)
}

// ListFeedbackReviews POST /api/v1/admin/feedback/:id/reviews（TODO §17：审核变更历史）
func (h *AdminHandler) ListFeedbackReviews(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := h.contentSvc.ListFeedbackReviews(r.Context(), id)
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	response.OK(w, items)
}

// SubmitFeedback POST /api/v1/feedback（公开）
func (h *AdminHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if !h.feedbackLimiter.allow(clientIP(r)) {
		response.TooManyRequests(w, r)
		return
	}
	var req struct {
		FeedbackType   string `json:"feedback_type"`
		Title          string `json:"title"`
		Content        string `json:"content"`
		RelatedURL     string `json:"related_url"`
		Contact        string `json:"contact"`
		TargetKind     string `json:"target_kind"`
		TargetRef      string `json:"target_ref"`
		LicenseVersion string `json:"license_version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	fb, err := h.contentSvc.SubmitFeedback(r.Context(), &domain.Feedback{
		FeedbackType:   req.FeedbackType,
		Title:          req.Title,
		Content:        req.Content,
		RelatedURL:     req.RelatedURL,
		Contact:        req.Contact,
		TargetKind:     req.TargetKind,
		TargetRef:      req.TargetRef,
		LicenseVersion: req.LicenseVersion,
	})
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	response.Created(w, fb)
}

// ---- 内部辅助 ----

// auditLog 写审计日志。
func (h *AdminHandler) auditLog(r *http.Request, action, objType, objID string, before, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   objType,
		ObjectID:     objID,
		Before:       before,
		After:        after,
	})
}

func summaryMap(args ...string) map[string]any {
	if len(args) == 0 {
		return nil
	}
	m := make(map[string]any)
	for i := 0; i+1 < len(args); i += 2 {
		m[args[i]] = args[i+1]
	}
	if len(args)%2 == 1 {
		m["note"] = args[len(args)-1]
	}
	return m
}

func writeResourceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, resource.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, resource.ErrSlugTaken):
		response.Conflict(w, r, "slug 已被占用")
	case errors.Is(err, resource.ErrMustUnpublishFirst):
		response.Conflict(w, r, resource.ErrMustUnpublishFirst.Error())
	case errors.Is(err, resource.ErrNotPublished):
		response.Conflict(w, r, resource.ErrNotPublished.Error())
	case err == nil:
		response.HandleError(w, r, err)
	default:
		// 业务校验错误（含错误信息）以 422 返回
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	}
}

func writeContentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, content.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, content.ErrSlugTaken):
		response.Conflict(w, r, "slug 已被占用")
	case errors.Is(err, content.ErrBadEnum):
		response.Validation(w, r, []response.ErrorDetail{{Message: "枚举值非法"}})
	case errors.Is(err, content.ErrDuplicateSubmission):
		response.Conflict(w, r, "请勿重复提交相同内容，如有补充请在原投稿基础上说明")
	case err == nil:
		response.HandleError(w, r, err)
	default:
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func sPtr(items []string) *[]string {
	if items == nil {
		return nil
	}
	return &items
}
