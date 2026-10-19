package catalog

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/versioning"
)

// 映射查询的扫描上限：命中结果先在内存中归并排序再分页，
// 因此对超大命中集做截断保护（并在响应中显式标记 Truncated）。
const (
	mappingClassScanLimit  = 5000
	mappingMemberScanLimit = 5000
	mappingMemberPreview   = 8
)

// mappingClassColumns 类命中查询字段（含所属数据集信息，便于跨数据集展示）。
const mappingClassColumns = `c.id, c.set_id, c.obf_name, c.mid_name, c.named_name,
	c.package, c.member_count, ms.set_key, ms.mc_version, ms.mapping_type`

// MappingSetFilter 映射数据集筛选。
type MappingSetFilter struct {
	Q           string
	MCVersion   string
	MappingType string
}

// ListMappingSets 查询已导入的映射数据集。
//
// 排序在内存中完成：mc_version 需要按版本语义降序（字符串序会把 1.9 排在 1.21 之后），
// 数据集数量为个位到数十，代价可忽略。
func (s *Service) ListMappingSets(ctx context.Context, f MappingSetFilter) ([]domain.MappingSet, error) {
	q := s.db.WithContext(ctx).Model(&domain.MappingSet{})
	if kw := strings.TrimSpace(f.Q); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("set_key ILIKE ? OR mc_version ILIKE ? OR build_label ILIKE ? OR channel ILIKE ?",
			like, like, like, like)
	}
	if v := strings.TrimSpace(f.MCVersion); v != "" {
		q = q.Where("mc_version = ?", v)
	}
	if t := strings.TrimSpace(f.MappingType); t != "" {
		q = q.Where("mapping_type = ?", t)
	}

	sets := make([]domain.MappingSet, 0, 16)
	if err := q.Limit(200).Find(&sets).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(sets, func(i, j int) bool {
		a, b := sets[i], sets[j]
		if c := compareVersionDesc(a.MCVersion, b.MCVersion); c != 0 {
			return c < 0
		}
		if a.MappingType != b.MappingType {
			return a.MappingType < b.MappingType
		}
		return a.SetKey < b.SetKey
	})
	return sets, nil
}

// compareVersionDesc 按版本语义比较两个版本字符串，返回 -1 / 0 / 1。
// 无法解析的版本退化为字符串比较，保证排序稳定不 panic。
func compareVersionDesc(a, b string) int {
	va, errA := versioning.Parse(a)
	vb, errB := versioning.Parse(b)
	if errA != nil || errB != nil {
		return strings.Compare(b, a)
	}
	return versioning.Compare(vb, va)
}

// MappingSearchFilter 类 / 成员查询条件。
type MappingSearchFilter struct {
	// Q 名称关键词，可为 obf / mid / named 任意一列的子串；为空表示按范围浏览。
	Q string
	// SetID 数据集 ID；为空表示跨数据集查询。
	SetID string
	// MCVersion / MappingType 在不指定 SetID 时用于缩小范围。
	MCVersion   string
	MappingType string
	// Kind 取值 all | class | method | field。
	Kind     string
	Page     int
	PageSize int
}

// MappingMemberView 成员视图（不含仅用于去重的 desc_key）。
type MappingMemberView struct {
	Kind      string `json:"kind"`
	ObfName   string `json:"obf_name"`
	MidName   string `json:"mid_name"`
	NamedName string `json:"named_name"`
	ObfDesc   string `json:"obf_desc"`
	NamedDesc string `json:"named_desc"`
	Side      string `json:"side"`
}

// MappingClassHit 类命中结果。
type MappingClassHit struct {
	ID          uuid.UUID `json:"id"`
	SetID       uuid.UUID `json:"set_id"`
	SetKey      string    `json:"set_key"`
	MCVersion   string    `json:"mc_version"`
	MappingType string    `json:"mapping_type"`
	ObfName     string    `json:"obf_name"`
	MidName     string    `json:"mid_name"`
	NamedName   string    `json:"named_name"`
	Package     string    `json:"package"`
	MemberCount int       `json:"member_count"`
	// MatchedMembers 命中的成员预览（最多 mappingMemberPreview 条）。
	MatchedMembers []MappingMemberView `json:"matched_members"`
	// MatchedTotal 命中的成员总数（可能大于预览条数）。
	MatchedTotal int `json:"matched_total"`
}

// MappingSearchResult 映射查询结果。
type MappingSearchResult struct {
	Hits []MappingClassHit `json:"hits"`
	// Total 命中类总数（分页前）。
	Total int64 `json:"total"`
	// Truncated 命中集被扫描上限截断时为 true，提示收窄查询范围。
	Truncated bool `json:"truncated"`
}

// memberHit 成员命中中间结构。
type memberHit struct {
	ClassID   uuid.UUID
	Kind      string
	ObfName   string
	MidName   string
	NamedName string
	ObfDesc   string
	NamedDesc string
	Side      string
}

// SearchMappings 按类名或成员名查询映射，返回类命中及命中的成员预览。
//
// 设计说明：
//   - 类名命中与成员名命中分别查询后在内存归并：类名命中优先（rank 0），
//     仅成员命中的类排在其后（rank 1），再按可读名排序，结果可预期；
//   - 同一查询同时覆盖 obf / mid / named 三列，使用者不必知道自己在查哪套命名空间；
//   - 不做模糊排序打分，避免"看起来智能但不可解释"的结果顺序。
func (s *Service) SearchMappings(ctx context.Context, f MappingSearchFilter) (*MappingSearchResult, error) {
	kw := strings.TrimSpace(f.Q)
	kind := strings.TrimSpace(f.Kind)
	if kind == "" {
		kind = "all"
	}

	scope := s.scopeMappingClasses(ctx, f)
	like := "%" + kw + "%"

	var (
		classHits  = make([]MappingClassHit, 0, 32)
		memberHits = make([]memberHit, 0, 32)
		truncated  bool
	)

	// 1) 类名命中（kind = class / all）
	if kind == "all" || kind == "class" {
		q := scope.Session(&gorm.Session{}).
			Select(mappingClassColumns).
			Order("c.named_name ASC, c.obf_name ASC").
			Limit(mappingClassScanLimit + 1)
		if kw != "" {
			q = q.Where("c.obf_name ILIKE ? OR c.mid_name ILIKE ? OR c.named_name ILIKE ?", like, like, like)
		}
		if err := q.Scan(&classHits).Error; err != nil {
			return nil, err
		}
		if len(classHits) > mappingClassScanLimit {
			classHits = classHits[:mappingClassScanLimit]
			truncated = true
		}
	}

	// 2) 成员名命中（kind = method / field / all）
	if kind != "class" {
		q := scope.Session(&gorm.Session{}).
			Select("m.class_id, m.kind, m.obf_name, m.mid_name, m.named_name, m.obf_desc, m.named_desc, m.side").
			Joins("JOIN mapping_members AS m ON m.class_id = c.id").
			Limit(mappingMemberScanLimit + 1)
		if kw == "" {
			// 无关键词时不做成员匹配，避免退化为全表扫描。
			q = q.Where("1 = 0")
		} else {
			q = q.Where("m.obf_name ILIKE ? OR m.mid_name ILIKE ? OR m.named_name ILIKE ?", like, like, like)
		}
		if kind == "method" || kind == "field" {
			q = q.Where("m.kind = ?", kind)
		}
		if err := q.Scan(&memberHits).Error; err != nil {
			return nil, err
		}
		if len(memberHits) > mappingMemberScanLimit {
			memberHits = memberHits[:mappingMemberScanLimit]
			truncated = true
		}
	}

	// 3) 归并：类名命中优先，仅成员命中的类补查类信息
	index := make(map[uuid.UUID]int, len(classHits))
	for i := range classHits {
		classHits[i].MatchedMembers = []MappingMemberView{}
		index[classHits[i].ID] = i
	}
	ordered := classHits
	rank := make(map[uuid.UUID]int, len(classHits))
	for _, h := range classHits {
		rank[h.ID] = 0
	}

	// 成员命中按类分组，待类信息齐备后统一挂载（避免遗漏同类的多条命中）。
	byClass := make(map[uuid.UUID][]memberHit, len(memberHits))
	missing := make([]uuid.UUID, 0, 16)
	for _, m := range memberHits {
		byClass[m.ClassID] = append(byClass[m.ClassID], m)
		if _, ok := index[m.ClassID]; !ok {
			missing = append(missing, m.ClassID)
		}
	}

	if len(missing) > 0 {
		var extra []MappingClassHit
		err := scope.Session(&gorm.Session{}).
			Where("c.id IN ?", missing).
			Select(mappingClassColumns).
			Scan(&extra).Error
		if err != nil {
			return nil, err
		}
		for i := range extra {
			extra[i].MatchedMembers = []MappingMemberView{}
			index[extra[i].ID] = len(ordered)
			rank[extra[i].ID] = 1
			ordered = append(ordered, extra[i])
		}
	}

	for classID, hits := range byClass {
		idx, ok := index[classID]
		if !ok {
			continue
		}
		for _, m := range hits {
			attachMember(&ordered[idx], m)
		}
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		if rank[ordered[i].ID] != rank[ordered[j].ID] {
			return rank[ordered[i].ID] < rank[ordered[j].ID]
		}
		a, b := ordered[i], ordered[j]
		if a.NamedName != b.NamedName {
			return a.NamedName < b.NamedName
		}
		return a.ObfName < b.ObfName
	})

	total := int64(len(ordered))
	page, pageSize := normalizePage(f.Page, f.PageSize)
	start := (page - 1) * pageSize
	if start >= len(ordered) {
		return &MappingSearchResult{Hits: []MappingClassHit{}, Total: total, Truncated: truncated}, nil
	}
	end := start + pageSize
	if end > len(ordered) {
		end = len(ordered)
	}
	return &MappingSearchResult{Hits: ordered[start:end], Total: total, Truncated: truncated}, nil
}

// attachMember 追加一条成员预览并维护命中计数。
func attachMember(hit *MappingClassHit, m memberHit) {
	hit.MatchedTotal++
	if len(hit.MatchedMembers) >= mappingMemberPreview {
		return
	}
	hit.MatchedMembers = append(hit.MatchedMembers, MappingMemberView{
		Kind:      m.Kind,
		ObfName:   m.ObfName,
		MidName:   m.MidName,
		NamedName: m.NamedName,
		ObfDesc:   m.ObfDesc,
		NamedDesc: m.NamedDesc,
		Side:      m.Side,
	})
}

// scopeMappingClasses 构造类级查询范围（数据集过滤 + 关键词无关条件）。
func (s *Service) scopeMappingClasses(ctx context.Context, f MappingSearchFilter) *gorm.DB {
	q := s.db.WithContext(ctx).
		Table("mapping_classes AS c").
		Joins("JOIN mapping_sets AS ms ON ms.id = c.set_id")
	if id := strings.TrimSpace(f.SetID); id != "" {
		q = q.Where("c.set_id = ?", id)
	}
	if v := strings.TrimSpace(f.MCVersion); v != "" {
		q = q.Where("ms.mc_version = ?", v)
	}
	if t := strings.TrimSpace(f.MappingType); t != "" {
		q = q.Where("ms.mapping_type = ?", t)
	}
	return q
}

// MappingClassDetail 类详情：类映射 + 成员列表（可分页、可按名称过滤）。
type MappingClassDetail struct {
	Class   MappingClassHit     `json:"class"`
	Members []MappingMemberView `json:"members"`
	Total   int64               `json:"member_total"`
	Set     *domain.MappingSet  `json:"set"`
}

// GetMappingClass 按 ID 获取类映射及其成员。
func (s *Service) GetMappingClass(ctx context.Context, classID string, kind, q string, page, pageSize int) (*MappingClassDetail, error) {
	id, err := uuid.Parse(strings.TrimSpace(classID))
	if err != nil {
		return nil, ErrNotFound
	}

	var cls MappingClassHit
	err = s.db.WithContext(ctx).
		Table("mapping_classes AS c").
		Joins("JOIN mapping_sets AS ms ON ms.id = c.set_id").
		Where("c.id = ?", id).
		Select(mappingClassColumns).
		Take(&cls).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cls.MatchedMembers = []MappingMemberView{}

	var set domain.MappingSet
	if err := s.db.WithContext(ctx).Where("id = ?", cls.SetID).First(&set).Error; err != nil {
		// 数据集缺失不影响类信息展示，仅省略。
		set = domain.MappingSet{}
	}

	mq := s.db.WithContext(ctx).Model(&domain.MappingMember{}).Where("class_id = ?", cls.ID)
	if k := strings.TrimSpace(kind); k != "" && k != "all" {
		mq = mq.Where("kind = ?", k)
	}
	if kw := strings.TrimSpace(q); kw != "" {
		like := "%" + kw + "%"
		mq = mq.Where("obf_name ILIKE ? OR mid_name ILIKE ? OR named_name ILIKE ?", like, like, like)
	}

	var total int64
	if err := mq.Count(&total).Error; err != nil {
		return nil, err
	}
	p, ps := normalizePage(page, pageSize)
	members := make([]MappingMemberView, 0, ps)
	err = mq.Select("kind, obf_name, mid_name, named_name, obf_desc, named_desc, side").
		Order("kind ASC, named_name ASC, obf_name ASC").
		Offset((p - 1) * ps).Limit(ps).
		Scan(&members).Error
	if err != nil {
		return nil, err
	}

	out := &MappingClassDetail{Class: cls, Members: members, Total: total}
	if set.ID != uuid.Nil {
		out.Set = &set
	}
	return out, nil
}
