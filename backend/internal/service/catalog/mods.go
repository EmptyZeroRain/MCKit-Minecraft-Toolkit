package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// maxModScan Mod 版本筛选需在内存中进行（精确版本语义），故先取全集再分页。
// 资料量级较小（数十条），此上限仅用于防御异常数据。
const maxModScan = 500

// maxModVersions 单个 Mod 返回的发布版本条数上限，避免异常数据导致无界返回。
const maxModVersions = 200

// maxModDependencies 单个 Mod 返回的依赖 / 冲突条数上限。
const maxModDependencies = 200

// maxModpackMods 单个整合包返回的 Mod 清单条数上限。
const maxModpackMods = 500

// ModRef 关联展示用的 Mod 摘要（只含跳转与展示所需字段）。
type ModRef struct {
	Slug    string   `json:"slug"`
	NameZh  string   `json:"name_zh"`
	NameEn  string   `json:"name_en"`
	Loaders []string `json:"loaders"`
	// URL Mod 详情页路径；由后端统一生成，保证链接形态一致。
	URL string `json:"url"`
}

// modRefOf 由 Mod 构造展示摘要。
func modRefOf(m domain.Mod) ModRef {
	loaders := []string(m.Loaders)
	if loaders == nil {
		loaders = []string{}
	}
	return ModRef{
		Slug:    m.Slug,
		NameZh:  m.NameZh,
		NameEn:  m.NameEn,
		Loaders: loaders,
		URL:     "/mods/" + m.Slug,
	}
}

// ModFilter Mod 查询筛选（TODO §16：按名称、加载器、类别与游戏版本检索）。
type ModFilter struct {
	Query     string
	Loader    string
	Category  string
	MCVersion string
	Page      int
	PageSize  int
}

// ListMods 按名称/别名、加载器、类别、适用游戏版本查询**已发布**的 Mod。
//
// 只返回 review_status = published 的记录（内容审核流程：未审核内容不对公开接口可见）。
// 游戏版本筛选使用 versioning.Satisfies 的精确语义（1.20 可命中声明 1.20-1.21，
// 但不会命中声明 1.19+ 之外的 1.22），因此需在内存中过滤后再分页。
func (s *Service) ListMods(ctx context.Context, f ModFilter) ([]domain.Mod, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Mod{}).
		Where("review_status = ?", domain.ModReviewPublished)
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ?",
			like, like, like, like,
		)
	}
	if loader := strings.TrimSpace(f.Loader); loader != "" {
		// JSONB 数组成员判定：参数绑定，杜绝注入。
		q = q.Where("loaders @> jsonb_build_array(?::text)", loader)
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		q = q.Where("categories @> jsonb_build_array(?::text)", category)
	}

	var all []domain.Mod
	if err := q.Order("sort_order ASC, name_zh ASC").
		Limit(maxModScan).Find(&all).Error; err != nil {
		return nil, 0, err
	}

	target := strings.TrimSpace(f.MCVersion)
	if target != "" {
		filtered := make([]domain.Mod, 0, len(all))
		for _, m := range all {
			if versioning.Satisfies(m.MCVersions, target) {
				filtered = append(filtered, m)
			}
		}
		all = filtered
	}

	total := int64(len(all))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []domain.Mod{}, total, nil
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

// ModDependencyView 依赖展示视图：附带**已收录**对方 Mod 的摘要引用。
type ModDependencyView struct {
	domain.ModDependency
	// TargetMod 已收录对方 Mod 的摘要；未收录（或已删除）时为 nil，前端回退为纯文本名。
	TargetMod *ModRef `json:"target_mod"`
}

// ModDependentView 反向依赖：谁依赖了当前 Mod（TODO §16）。
type ModDependentView struct {
	// DepKind 依赖方记录的依赖类型。
	DepKind string `json:"dep_kind"`
	// Mod 依赖方 Mod 的摘要。
	Mod ModRef `json:"mod"`
	// VersionNote / Note / SourceURL / Verified 原样透出依赖方记录，便于核验。
	VersionNote string `json:"version_note"`
	Note        string `json:"note"`
	SourceURL   string `json:"source_url"`
	Verified    bool   `json:"verified"`
}

// ModDetail Mod 详情：本体 + 发布版本 + 依赖与冲突 + 反向依赖（TODO §16）。
type ModDetail struct {
	domain.Mod
	// Versions 同一 Mod 的发布版本（按排序号 / 版本倒序）。
	Versions []domain.ModVersion `json:"versions"`
	// Dependencies 已记录的依赖与冲突（按类型分组顺序），逐条带来源与验证状态。
	Dependencies []ModDependencyView `json:"dependencies"`
	// Dependents 反向依赖：记录了「依赖当前 Mod」的其它 Mod（便于查看前置被谁使用）。
	Dependents []ModDependentView `json:"dependents"`
}

// GetModBySlug 按 slug 查询**已发布**的 Mod 详情（TODO §16）。
func (s *Service) GetModBySlug(ctx context.Context, slug string) (*ModDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var m domain.Mod
	err := s.db.WithContext(ctx).
		Where("slug = ? AND review_status = ?", slug, domain.ModReviewPublished).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &ModDetail{
		Mod:          m,
		Versions:     []domain.ModVersion{},
		Dependencies: []ModDependencyView{},
		Dependents:   []ModDependentView{},
	}

	// ---- 发布版本 ----
	if err := s.db.WithContext(ctx).
		Where("mod_id = ?", m.ID).
		Order("sort_order ASC, published_at DESC NULLS LAST, version DESC").
		Limit(maxModVersions).Find(&detail.Versions).Error; err != nil {
		return nil, err
	}

	// ---- 依赖与冲突 ----
	var deps []domain.ModDependency
	if err := s.db.WithContext(ctx).
		Where("mod_id = ?", m.ID).
		Order(modDependencyOrder).
		Limit(maxModDependencies).Find(&deps).Error; err != nil {
		return nil, err
	}
	targetIDs := make([]uuid.UUID, 0, len(deps))
	for _, d := range deps {
		if d.TargetModID != nil {
			targetIDs = append(targetIDs, *d.TargetModID)
		}
	}
	targetRefs, err := s.ResolveModRefsByID(ctx, targetIDs)
	if err != nil {
		return nil, err
	}
	for _, d := range deps {
		view := ModDependencyView{ModDependency: d}
		if d.TargetModID != nil {
			if ref, ok := targetRefs[*d.TargetModID]; ok {
				refCopy := ref
				view.TargetMod = &refCopy
			}
		}
		detail.Dependencies = append(detail.Dependencies, view)
	}

	// ---- 反向依赖（谁依赖了当前 Mod）----
	var dependents []domain.ModDependency
	if err := s.db.WithContext(ctx).
		Where("target_mod_id = ?", m.ID).
		Order(modDependencyOrder).
		Limit(maxModDependencies).Find(&dependents).Error; err != nil {
		return nil, err
	}
	depModIDs := make([]uuid.UUID, 0, len(dependents))
	for _, d := range dependents {
		depModIDs = append(depModIDs, d.ModID)
	}
	depRefs, err := s.ResolveModRefsByID(ctx, depModIDs)
	if err != nil {
		return nil, err
	}
	for _, d := range dependents {
		ref, ok := depRefs[d.ModID]
		if !ok {
			continue // 依赖方 Mod 已删除，跳过，避免断链
		}
		detail.Dependents = append(detail.Dependents, ModDependentView{
			DepKind:     d.DepKind,
			Mod:         ref,
			VersionNote: d.VersionNote,
			Note:        d.Note,
			SourceURL:   d.SourceURL,
			Verified:    d.Verified,
		})
	}
	return detail, nil
}

// modDependencyOrder 依赖类型展示顺序（必需 → 可选 → 内嵌 → 不兼容）。
const modDependencyOrder = `CASE dep_kind
	WHEN 'required' THEN 0
	WHEN 'optional' THEN 1
	WHEN 'embedded' THEN 2
	WHEN 'incompatible' THEN 3
	ELSE 4 END`

// ResolveModRefsByID 按 id 批量解析 Mod 摘要（TODO §16 关联有效性检查）。
// **只返回 mods 表中真实存在的 Mod**：已删除时该键不会出现，调用方必须回退为纯文本名。
func (s *Service) ResolveModRefsByID(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ModRef, error) {
	out := make(map[uuid.UUID]ModRef)
	if len(ids) == 0 {
		return out, nil
	}
	uniq := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	var mods []domain.Mod
	if err := s.db.WithContext(ctx).Where("id IN ?", uniq).Find(&mods).Error; err != nil {
		return nil, err
	}
	for _, m := range mods {
		out[m.ID] = modRefOf(m)
	}
	return out, nil
}

// ResolveModRefsBySlug 按 slug 批量解析 Mod 摘要（导入器与整合包清单复用）。
// 只返回真实存在的 Mod；未收录时该键不会出现，调用方回退为纯文本名。
func (s *Service) ResolveModRefsBySlug(ctx context.Context, slugs []string) (map[string]ModRef, error) {
	out := make(map[string]ModRef)
	if len(slugs) == 0 {
		return out, nil
	}
	uniq := make([]string, 0, len(slugs))
	seen := make(map[string]struct{}, len(slugs))
	for _, sl := range slugs {
		sl = strings.TrimSpace(sl)
		if sl == "" {
			continue
		}
		if _, ok := seen[sl]; ok {
			continue
		}
		seen[sl] = struct{}{}
		uniq = append(uniq, sl)
	}
	if len(uniq) == 0 {
		return out, nil
	}
	var mods []domain.Mod
	if err := s.db.WithContext(ctx).Where("slug IN ?", uniq).Find(&mods).Error; err != nil {
		return nil, err
	}
	for _, m := range mods {
		out[m.Slug] = modRefOf(m)
	}
	return out, nil
}

// ModpackFilter 整合包查询筛选（TODO §16）。
type ModpackFilter struct {
	Query     string
	Loader    string
	MCVersion string
	Page      int
	PageSize  int
}

// ListModpacks 按名称/别名、加载器与游戏版本查询**已发布**的整合包。
func (s *Service) ListModpacks(ctx context.Context, f ModpackFilter) ([]domain.Modpack, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Modpack{}).
		Where("review_status = ?", domain.ModReviewPublished)
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where(
			"slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ? OR aliases::text ILIKE ? OR COALESCE(author,'') ILIKE ?",
			like, like, like, like, like,
		)
	}
	if loader := strings.TrimSpace(f.Loader); loader != "" {
		q = q.Where("loader = ?", loader)
	}
	if mv := strings.TrimSpace(f.MCVersion); mv != "" {
		q = q.Where("mc_version = ?", mv)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Modpack
	err := q.Order("sort_order ASC, name_zh ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ModpackModView 整合包内一条 Mod 清单项：附带**已收录** Mod 的摘要引用。
type ModpackModView struct {
	domain.ModpackMod
	// ModRef 已收录 Mod 的摘要；未收录（或已删除）时为 nil，前端回退为纯文本名。
	ModRef *ModRef `json:"mod_ref"`
}

// ModpackDetail 整合包详情：本体 + 包内 Mod 清单（TODO §16）。
type ModpackDetail struct {
	domain.Modpack
	// Mods 包内 Mod 清单（按排序号），已收录项解析为可跳转引用。
	Mods []ModpackModView `json:"mods"`
}

// GetModpackBySlug 按 slug 查询**已发布**的整合包详情（TODO §16）。
func (s *Service) GetModpackBySlug(ctx context.Context, slug string) (*ModpackDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, ErrNotFound
	}
	var pack domain.Modpack
	err := s.db.WithContext(ctx).
		Where("slug = ? AND review_status = ?", slug, domain.ModReviewPublished).
		First(&pack).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	detail := &ModpackDetail{Modpack: pack, Mods: []ModpackModView{}}
	var rows []domain.ModpackMod
	if err := s.db.WithContext(ctx).
		Where("modpack_id = ?", pack.ID).
		Order("sort_order ASC, mod_name ASC").
		Limit(maxModpackMods).Find(&rows).Error; err != nil {
		return nil, err
	}
	modIDs := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		if r.ModID != nil {
			modIDs = append(modIDs, *r.ModID)
		}
	}
	refs, err := s.ResolveModRefsByID(ctx, modIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		view := ModpackModView{ModpackMod: r}
		if r.ModID != nil {
			if ref, ok := refs[*r.ModID]; ok {
				refCopy := ref
				view.ModRef = &refCopy
			}
		}
		detail.Mods = append(detail.Mods, view)
	}
	return detail, nil
}

// ---- 管理端（内容审核，TODO §16）----

// ModAdminFilter 管理端 Mod 列表筛选（按审核状态）。
type ModAdminFilter struct {
	Query        string
	ReviewStatus string
	Loader       string
	Page         int
	PageSize     int
}

// ListModsForAdmin 管理端查询 Mod（不过滤发布态，供审核使用）。
func (s *Service) ListModsForAdmin(ctx context.Context, f ModAdminFilter) ([]domain.Mod, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Mod{})
	if status := strings.TrimSpace(f.ReviewStatus); status != "" {
		q = q.Where("review_status = ?", status)
	}
	if loader := strings.TrimSpace(f.Loader); loader != "" {
		q = q.Where("loaders @> jsonb_build_array(?::text)", loader)
	}
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(name_en,'') ILIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Mod
	err := q.Order("review_status ASC, sort_order ASC, name_zh ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ReviewMod 审核 Mod：更新审核状态并留痕（TODO §16 内容审核流程）。
// reviewer 取自登录会话；仅接受合法审核状态。
func (s *Service) ReviewMod(ctx context.Context, id, status, reviewer, note string) (*domain.Mod, error) {
	status = strings.TrimSpace(status)
	if !domain.IsValidModReviewStatus(status) {
		return nil, ErrInvalidReviewStatus
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrNotFound
	}
	var m domain.Mod
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	updates := map[string]any{
		"review_status": status,
		"reviewer":      strings.TrimSpace(reviewer),
		"reviewed_at":   now,
		"review_note":   strings.TrimSpace(note),
		"updated_at":    now,
	}
	if err := s.db.WithContext(ctx).Model(&domain.Mod{}).Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	m.ReviewStatus = status
	m.Reviewer = strings.TrimSpace(reviewer)
	m.ReviewedAt = &now
	m.ReviewNote = strings.TrimSpace(note)
	return &m, nil
}

// ModpackAdminFilter 管理端整合包列表筛选（按审核状态）。
type ModpackAdminFilter struct {
	Query        string
	ReviewStatus string
	Loader       string
	Page         int
	PageSize     int
}

// ListModpacksForAdmin 管理端查询整合包（不过滤发布态，供审核使用）。
func (s *Service) ListModpacksForAdmin(ctx context.Context, f ModpackAdminFilter) ([]domain.Modpack, int64, error) {
	q := s.db.WithContext(ctx).Model(&domain.Modpack{})
	if status := strings.TrimSpace(f.ReviewStatus); status != "" {
		q = q.Where("review_status = ?", status)
	}
	if loader := strings.TrimSpace(f.Loader); loader != "" {
		q = q.Where("loader = ?", loader)
	}
	if kw := strings.TrimSpace(f.Query); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("slug ILIKE ? OR name_zh ILIKE ? OR COALESCE(author,'') ILIKE ?", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(f.Page, f.PageSize)
	var items []domain.Modpack
	err := q.Order("review_status ASC, sort_order ASC, name_zh ASC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ReviewModpack 审核整合包：更新审核状态并留痕（TODO §16）。
func (s *Service) ReviewModpack(ctx context.Context, id, status, reviewer, note string) (*domain.Modpack, error) {
	status = strings.TrimSpace(status)
	if !domain.IsValidModReviewStatus(status) {
		return nil, ErrInvalidReviewStatus
	}
	if strings.TrimSpace(id) == "" {
		return nil, ErrNotFound
	}
	var pack domain.Modpack
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&pack).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	updates := map[string]any{
		"review_status": status,
		"reviewer":      strings.TrimSpace(reviewer),
		"reviewed_at":   now,
		"review_note":   strings.TrimSpace(note),
		"updated_at":    now,
	}
	if err := s.db.WithContext(ctx).Model(&domain.Modpack{}).Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	pack.ReviewStatus = status
	pack.Reviewer = strings.TrimSpace(reviewer)
	pack.ReviewedAt = &now
	pack.ReviewNote = strings.TrimSpace(note)
	return &pack, nil
}
