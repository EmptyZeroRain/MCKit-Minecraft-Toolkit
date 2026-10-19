package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/search"
	"mckit/backend/internal/service/catalog"
	"mckit/backend/internal/service/content"
	"mckit/backend/internal/service/resource"
	"mckit/backend/internal/service/searchstats"
)

// searchTimeout 搜索查询超时（TODO §5「查询超时防护」）。
// 超时后返回 500（错误体不泄漏内部细节），避免慢查询长期占用连接。
const searchTimeout = 3 * time.Second

// PublicHandler 公开 API 处理器（需求 §8.2）。
// 所有列表/搜索接口均 POST（项目决策）。公开列表只返回已发布内容。
type PublicHandler struct {
	resources   *resource.Service
	contentSvc  *content.Service
	catalog     *catalog.Service
	search      *search.Service
	searchStats *searchstats.Service
	views       ResourceViewRecorder
}

// ResourceViewRecorder 资源详情访问统计（TODO §13）。可为 nil（不做统计）。
type ResourceViewRecorder interface {
	RecordResourceView(slug string)
}

// NewPublicHandler 创建公开处理器。
func NewPublicHandler(resources *resource.Service, contentSvc *content.Service,
	catalogSvc *catalog.Service, searchSvc *search.Service) *PublicHandler {
	return &PublicHandler{
		resources:  resources,
		contentSvc: contentSvc,
		catalog:    catalogSvc,
		search:     searchSvc,
	}
}

// WithSearchStats 注入搜索词统计服务（TODO §5）。未注入时搜索仍可用，只是不做统计。
func (h *PublicHandler) WithSearchStats(svc *searchstats.Service) *PublicHandler {
	h.searchStats = svc
	return h
}

// WithResourceViewRecorder 注入资源详情访问统计（TODO §13）。未注入时不做统计。
func (h *PublicHandler) WithResourceViewRecorder(r ResourceViewRecorder) *PublicHandler {
	h.views = r
	return h
}

// ListResources POST /api/v1/resources（公开，只含已发布）
func (h *PublicHandler) ListResources(w http.ResponseWriter, r *http.Request) {
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
		Sort:           req.Sort,
		Page:           req.Page,
		PageSize:       req.PageSize,
		PublishedOnly:  true,
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

// ResourceDetail POST /api/v1/resources/:slug（公开，只含已发布）
// 返回资源本体 + 发布版本 + 兼容性结论（TODO §6：无数据依据时返回未验证）。
func (h *PublicHandler) ResourceDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.resources.GetDetailBySlug(r.Context(), slug)
	if err != nil {
		writeResourceError(w, r, err)
		return
	}
	// TODO §13：记录资源详情访问量（best-effort，不影响响应）。
	if h.views != nil && slug != "" {
		h.views.RecordResourceView(slug)
	}
	response.OK(w, detail)
}

// ListTools POST /api/v1/tools（公开）
func (h *PublicHandler) ListTools(w http.ResponseWriter, r *http.Request) {
	var req ListToolsRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.contentSvc.ListTools(r.Context(), content.ToolFilter{
		Category:      req.Category,
		Q:             req.Q,
		Tag:           req.Tag,
		PublishedOnly: true,
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

// ToolDetail POST /api/v1/tools/:slug（公开）
func (h *PublicHandler) ToolDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	tool, err := h.contentSvc.GetToolBySlug(r.Context(), slug, true)
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	response.OK(w, tool)
}

// WikiDetail POST /api/v1/wiki/:slug（公开）
func (h *PublicHandler) WikiDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	entry, err := h.contentSvc.GetWikiBySlug(r.Context(), slug, true)
	if err != nil {
		writeContentError(w, r, err)
		return
	}
	response.OK(w, entry)
}

// Categories POST /api/v1/categories（公开）
func (h *PublicHandler) Categories(w http.ResponseWriter, r *http.Request) {
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

// Search POST /api/v1/search（全站搜索，需求 §5.1）
func (h *PublicHandler) Search(w http.ResponseWriter, r *http.Request) {
	if h.search == nil {
		response.HandleError(w, r, errSearchUnavailable())
		return
	}
	var req struct {
		Q           string `json:"q"`
		Type        string `json:"type"`
		GameVersion string `json:"game_version"`
		Sort        string `json:"sort"`
		Page        int    `json:"page"`
		PageSize    int    `json:"page_size"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}

	// TODO §5 输入防护：超长关键词直接拒绝，避免超大 ILIKE 拖垮数据库。
	if utf8.RuneCountInString(strings.TrimSpace(req.Q)) > search.MaxQueryLength {
		response.Validation(w, r, []response.ErrorDetail{{
			Field:   "q",
			Message: fmt.Sprintf("搜索关键词最多 %d 个字符", search.MaxQueryLength),
		}})
		return
	}

	// TODO §5 查询超时防护：慢查询不得长期占用连接。
	ctx, cancel := context.WithTimeout(r.Context(), searchTimeout)
	defer cancel()

	results, err := h.search.Search(ctx, search.Filter{
		Q:           req.Q,
		Type:        req.Type,
		GameVersion: req.GameVersion,
		Sort:        req.Sort,
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		// TODO §5：记录「查询失败」而非「无结果」——两者的处置完全不同。
		if h.searchStats != nil && strings.TrimSpace(req.Q) != "" {
			_ = h.searchStats.RecordFailure(context.WithoutCancel(r.Context()), req.Q)
		}
		response.HandleError(w, r, err)
		return
	}

	// TODO §5 匿名化统计：只记录归一化关键词与结果数，best-effort，失败不影响搜索。
	if h.searchStats != nil && strings.TrimSpace(req.Q) != "" {
		// 统计不应被请求取消或查询超时中断，故剥离取消信号。
		_ = h.searchStats.Record(context.WithoutCancel(r.Context()), req.Q, int(results.Total))
	}

	meta := response.ResourceList{
		Total:      results.Total,
		Page:       results.Page,
		PageSize:   results.PageSize,
		TotalPages: pages(results.Total, results.PageSize),
	}
	// TODO §5 无结果时给出可尝试的关键词（基于已记录搜索词的字面关系，非语义猜测）。
	if results.Total == 0 && h.searchStats != nil {
		if suggestions, serr := h.searchStats.Suggestions(r.Context(), req.Q, 5); serr == nil {
			meta.Suggestions = suggestions
		}
	}
	response.Page(w, r, results.Items, meta)
}

// Items POST /api/v1/items（物品查询）
func (h *PublicHandler) Items(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListItems(r.Context(), catalog.ItemFilter{
		Q:        req.Q,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// itemDetailResponse 物品详情响应：物品本体 + 关联配方 + 生成式获取指令。
// 嵌入 domain.Item 使物品字段保持扁平（与既有 Item 契约兼容）。
type itemDetailResponse struct {
	domain.Item
	RelatedRecipes *catalog.ItemRecipeLinks `json:"related_recipes"`
	// ObtainCommand 由 item_id 生成的 /give 模板，不依赖任何未收录数据。
	ObtainCommand string `json:"obtain_command"`
}

// ItemDetail POST /api/v1/items/:item_id
// item_id 含冒号（如 minecraft:stone），可能以 URL 编码形式传入，需手动解码。
// 同时返回关联配方（产出 / 作为材料），供物品详情页展示（TODO §1）。
func (h *PublicHandler) ItemDetail(w http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "item_id")
	itemID, err := url.PathUnescape(raw)
	if err != nil {
		itemID = raw
	}
	item, err := h.catalog.GetItemByID(r.Context(), itemID)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}

	related, err := h.catalog.RelatedItemRecipes(r.Context(), item.ItemID)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	response.OK(w, itemDetailResponse{
		Item:           *item,
		RelatedRecipes: related,
		ObtainCommand:  fmt.Sprintf("/give @s %s 1", item.ItemID),
	})
}

// recipeListItem 配方列表项：配方本体 + 已解析的关联物品（TODO §3）。
// ResolvedItems 仅含**真实存在于物品库**的物品（键为 item_id），
// 未收录物品不会出现，前端据此回退为纯文本 ID，避免断链。
type recipeListItem struct {
	domain.Recipe
	ResolvedItems map[string]catalog.ItemRef `json:"resolved_items"`
}

// Recipes POST /api/v1/recipes（配方查询，需求 §8.5）
func (h *PublicHandler) Recipes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID      string `json:"item_id"`
		GameVersion string `json:"game_version"`
		RecipeType  string `json:"recipe_type"`
		Page        int    `json:"page"`
		PageSize    int    `json:"page_size"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	items, total, err := h.catalog.ListRecipes(r.Context(), catalog.RecipeFilter{
		ItemID:      req.ItemID,
		GameVersion: req.GameVersion,
		RecipeType:  req.RecipeType,
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}

	// TODO §3：批量解析本页配方引用到的物品，只保留真实存在的物品（一次查询完成）。
	refs, err := h.catalog.ResolveItemRefs(r.Context(), catalog.RecipeItems(items))
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	list := make([]recipeListItem, 0, len(items))
	for _, it := range items {
		sub := make(map[string]catalog.ItemRef)
		for _, id := range catalog.RecipeRefItems(it) {
			if ref, ok := refs[id]; ok {
				sub[id] = ref
			}
		}
		list = append(list, recipeListItem{Recipe: it, ResolvedItems: sub})
	}

	page, pageSize := normalizePage(req.Page, req.PageSize)
	response.Page(w, r, list, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: pages(total, pageSize),
	})
}

// Commands POST /api/v1/commands（指令列表与搜索）
func (h *PublicHandler) Commands(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListCommands(r.Context(), catalog.CommandFilter{
		Q:        req.Q,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// CommandDetail POST /api/v1/commands/:slug
func (h *PublicHandler) CommandDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	cmd, err := h.catalog.GetCommandBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, cmd)
}

// Potions POST /api/v1/potions（药水列表与筛选，TODO §9A）
func (h *PublicHandler) Potions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Category string `json:"category"`
		Version  string `json:"version"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListPotions(r.Context(), catalog.PotionFilter{
		Q:        req.Q,
		Category: req.Category,
		Version:  req.Version,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// BrewingRules POST /api/v1/potions/rules（通用酿造规则，TODO §9A）
// 注意：chi 静态段优先于参数段，故 /potions/rules 不会被 /potions/{slug} 吞掉。
func (h *PublicHandler) BrewingRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.catalog.GenericBrewingRules(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, rules)
}

// PotionDetail POST /api/v1/potions/:slug（药水详情：本体 + 酿造步骤 + 关联物品，TODO §9A）
func (h *PublicHandler) PotionDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.catalog.GetPotionBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// Villagers POST /api/v1/villagers（村民职业列表与筛选，TODO §9B）
func (h *PublicHandler) Villagers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Kind     string `json:"kind"`
		Version  string `json:"version"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListVillagerProfessions(r.Context(), catalog.VillagerFilter{
		Q:        req.Q,
		Kind:     req.Kind,
		Version:  req.Version,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// VillagerTradeLevels POST /api/v1/villagers/levels（职业等级通用规则，TODO §9B）
// 注意：chi 静态段优先于参数段，故 /villagers/levels 不会被 /villagers/{slug} 吞掉。
func (h *PublicHandler) VillagerTradeLevels(w http.ResponseWriter, r *http.Request) {
	levels, err := h.catalog.VillagerTradeLevels(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, levels)
}

// VillagerDetail POST /api/v1/villagers/:slug（职业详情：本体 + 等级规则 + 分组交易 + 关联物品，TODO §9B）
func (h *PublicHandler) VillagerDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.catalog.GetVillagerProfessionBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// MobDetail POST /api/v1/mobs/:slug（生物详情：本体 + 掉落 + 关联物品，TODO §9C）
func (h *PublicHandler) MobDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.catalog.GetMobBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// Mobs POST /api/v1/mobs（生物列表与筛选，TODO §9C）
func (h *PublicHandler) Mobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Category string `json:"category"`
		Version  string `json:"version"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListMobs(r.Context(), catalog.MobFilter{
		Q:        req.Q,
		Category: req.Category,
		Version:  req.Version,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// RedstoneComponents POST /api/v1/redstone/components（红石元件列表与筛选，TODO §15）
func (h *PublicHandler) RedstoneComponents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q        string `json:"q"`
		Category string `json:"category"`
		Version  string `json:"version"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListRedstoneComponents(r.Context(), catalog.RedstoneComponentFilter{
		Query:    req.Q,
		Category: req.Category,
		Version:  req.Version,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// RedstoneComponentDetail POST /api/v1/redstone/components/:slug（红石元件详情，TODO §15）
func (h *PublicHandler) RedstoneComponentDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	component, err := h.catalog.GetRedstoneComponentBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, component)
}

// RedstoneDesigns POST /api/v1/redstone/designs（机器设计列表与筛选，TODO §15）
// 只返回 review_status = published 的记录（内容审核流程）。
func (h *PublicHandler) RedstoneDesigns(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q          string `json:"q"`
		Purpose    string `json:"purpose"`
		Difficulty string `json:"difficulty"`
		Version    string `json:"version"`
		Page       int    `json:"page"`
		PageSize   int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	items, total, err := h.catalog.ListRedstoneDesigns(r.Context(), catalog.RedstoneDesignFilter{
		Query:      req.Q,
		Purpose:    req.Purpose,
		Difficulty: req.Difficulty,
		Version:    req.Version,
		Page:       req.Page,
		PageSize:   req.PageSize,
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

// RedstoneDesignDetail POST /api/v1/redstone/designs/:slug（机器设计详情 + 关联物品，TODO §15）
// 只返回 review_status = published 的记录（内容审核流程）。
func (h *PublicHandler) RedstoneDesignDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	detail, err := h.catalog.GetRedstoneDesignBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

// DevDocs POST /api/v1/dev-docs（开发文档列表与筛选，TODO §21）
func (h *PublicHandler) DevDocs(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.catalog.ListDevDocs(r.Context(), catalog.DevDocFilter{
		Q:        req.Q,
		Loader:   req.Loader,
		Category: req.Category,
		Version:  req.Version,
		Page:     req.Page,
		PageSize: req.PageSize,
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

// DevDocFacets POST /api/v1/dev-docs/facets（平台 / 分类统计，TODO §21）
// 注意：chi 静态段优先于参数段，故 /dev-docs/facets 不会被 /dev-docs/{slug} 吞掉。
func (h *PublicHandler) DevDocFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := h.catalog.Facets(r.Context())
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, facets)
}

// DevDocDetail POST /api/v1/dev-docs/:slug（开发文档详情 + 同平台相关条目，TODO §21）
func (h *PublicHandler) DevDocDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	doc, err := h.catalog.GetDevDocBySlug(r.Context(), slug)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	related, err := h.catalog.RelatedDevDocs(r.Context(), doc, 6)
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, map[string]any{
		"doc":     doc,
		"related": related,
	})
}

// MappingSets POST /api/v1/mappings/sets（已导入的映射数据集列表，TODO §21）
func (h *PublicHandler) MappingSets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q           string `json:"q"`
		MCVersion   string `json:"mc_version"`
		MappingType string `json:"mapping_type"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	sets, err := h.catalog.ListMappingSets(r.Context(), catalog.MappingSetFilter{
		Q:           req.Q,
		MCVersion:   req.MCVersion,
		MappingType: req.MappingType,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	response.OK(w, map[string]any{"sets": sets})
}

// MappingSearch POST /api/v1/mappings/search（类 / 方法 / 字段查询，TODO §21）
func (h *PublicHandler) MappingSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Q           string `json:"q"`
		SetID       string `json:"set_id"`
		MCVersion   string `json:"mc_version"`
		MappingType string `json:"mapping_type"`
		Kind        string `json:"kind"`
		Page        int    `json:"page"`
		PageSize    int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req)
	}
	result, err := h.catalog.SearchMappings(r.Context(), catalog.MappingSearchFilter{
		Q:           req.Q,
		SetID:       req.SetID,
		MCVersion:   req.MCVersion,
		MappingType: req.MappingType,
		Kind:        req.Kind,
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	// 不使用通用分页响应：映射查询还需回传「命中集是否被扫描上限截断」，
	// 该信号属于业务语义而非分页元信息，故与分页字段一并放在 data 中。
	response.OK(w, map[string]any{
		"hits":        result.Hits,
		"total":       result.Total,
		"truncated":   result.Truncated,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": pages(result.Total, pageSize),
	})
}

// MappingClassDetail POST /api/v1/mappings/class/:id（类映射与成员列表，TODO §21）
func (h *PublicHandler) MappingClassDetail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind     string `json:"kind"`
		Q        string `json:"q"`
		Page     int    `json:"page"`
		PageSize int    `json:"page_size"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	detail, err := h.catalog.GetMappingClass(r.Context(), chi.URLParam(r, "id"),
		req.Kind, req.Q, req.Page, req.PageSize)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	response.OK(w, detail)
}

func pages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	p := int((total + int64(pageSize) - 1) / int64(pageSize))
	if p < 1 {
		return 1
	}
	return p
}

func writeCatalogError(w http.ResponseWriter, r *http.Request, err error) {
	if err == catalog.ErrNotFound {
		response.NotFound(w, r)
		return
	}
	response.HandleError(w, r, err)
}

// errSearchUnavailable 搜索服务未配置的返回。
func errSearchUnavailable() error {
	return &errServiceUnavailable{}
}

// errServiceUnavailable 服务不可用错误。
type errServiceUnavailable struct{}

func (e *errServiceUnavailable) Error() string { return "搜索服务未配置" }
