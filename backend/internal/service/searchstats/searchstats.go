// Package searchstats 提供搜索词的匿名化聚合统计（TODO §5 搜索体验）。
//
// 隐私立场：只保存「归一化关键词 + 计数」，**不保存 IP、会话、用户标识，也不保存
// 逐次查询的时间序列**。归一化会折叠空白、转小写并限制长度；纯符号 / 纯 emoji 等
// 无检索意义的输入直接丢弃。因此本表无法还原「谁在什么时候搜了什么」。
package searchstats

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"mckit/backend/internal/domain"
)

// MaxTermLength 记录的最长关键词（以字符数计，与 search_terms.term 的 64 上限一致）。
const MaxTermLength = 64

// Normalize 归一化搜索词。
//
// 返回空串表示「不记录」：空输入、纯空白、纯符号（无字母/数字/汉字）、超过长度上限。
func Normalize(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Fields 按任意空白切分，可顺带去除内部的换行 / 制表符等控制字符。
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ToLower(s)
	if utf8.RuneCountInString(s) > MaxTermLength {
		return ""
	}
	if !hasWordChar(s) {
		return ""
	}
	return s
}

// hasWordChar 是否包含字母、数字或汉字等有检索意义的字符。
func hasWordChar(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// DailyRecorder 按天聚合搜索统计（可选注入，TODO §13）。
//
// 与词级聚合互补：词级回答「搜了什么、多少无结果」，日聚合回答「某段时间搜了多少、
// 多少无结果、多少失败」，供运营看板按时间范围筛选。实现须为并发安全。
type DailyRecorder interface {
	// RecordSearch 记录一次搜索：zeroResult 表示查询成功但无结果，failed 表示查询链路失败。
	RecordSearch(zeroResult, failed bool)
}

// Service 搜索词统计服务。
type Service struct {
	db    *gorm.DB
	daily DailyRecorder
}

// NewService 创建搜索词统计服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// WithDailyRecorder 注入日聚合记录器（可为 nil，表示不做日聚合）。
func (s *Service) WithDailyRecorder(r DailyRecorder) *Service {
	s.daily = r
	return s
}

// Record 记录一次搜索。resultCount 为本次结果数，0 计入「无结果」。
//
// 设计为 best-effort：调用方不应因统计失败而影响搜索本身（错误仅由调用方记录）。
func (s *Service) Record(ctx context.Context, rawTerm string, resultCount int) error {
	term := Normalize(rawTerm)
	if term == "" {
		return nil
	}
	noResult := 0
	if resultCount <= 0 {
		noResult = 1
	}
	// 单条 UPSERT 完成累加，避免「读-改-写」竞态。
	err := s.db.WithContext(ctx).Exec(`
		INSERT INTO search_terms (term, hits, no_result_hits, last_result_count, first_seen_at, last_seen_at)
		VALUES (?, 1, ?, ?, now(), now())
		ON CONFLICT (term) DO UPDATE SET
			hits              = search_terms.hits + 1,
			no_result_hits    = search_terms.no_result_hits + EXCLUDED.no_result_hits,
			last_result_count = EXCLUDED.last_result_count,
			last_seen_at      = now()`, term, noResult, resultCount).Error
	if err == nil && s.daily != nil {
		s.daily.RecordSearch(resultCount <= 0, false)
	}
	return err
}

// RecordFailure 记录一次「查询失败」的搜索（超时 / 数据库错误等，TODO §5）。
//
// 与无结果区分：无结果说明资料缺失，失败说明查询链路有问题，两者的处置完全不同。
// 失败时不改动 last_result_count，避免把「查询失败」误报为「无结果」。
func (s *Service) RecordFailure(ctx context.Context, rawTerm string) error {
	term := Normalize(rawTerm)
	if term == "" {
		return nil
	}
	err := s.db.WithContext(ctx).Exec(`
		INSERT INTO search_terms (term, hits, failed_hits, first_seen_at, last_seen_at)
		VALUES (?, 1, 1, now(), now())
		ON CONFLICT (term) DO UPDATE SET
			hits        = search_terms.hits + 1,
			failed_hits = search_terms.failed_hits + 1,
			last_seen_at = now()`, term).Error
	if err == nil && s.daily != nil {
		s.daily.RecordSearch(false, true)
	}
	return err
}

// Hot 返回热门搜索词（按搜索次数降序）。
func (s *Service) Hot(ctx context.Context, limit int) ([]domain.SearchTerm, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	out := make([]domain.SearchTerm, 0, limit)
	err := s.db.WithContext(ctx).
		Order("hits DESC, last_seen_at DESC").
		Limit(limit).Find(&out).Error
	return out, err
}

// NoResult 返回高频「无结果」搜索词（按无结果次数降序）——用于发现资料缺口。
func (s *Service) NoResult(ctx context.Context, limit int) ([]domain.SearchTerm, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	out := make([]domain.SearchTerm, 0, limit)
	err := s.db.WithContext(ctx).
		Where("no_result_hits > 0").
		Order("no_result_hits DESC, hits DESC").
		Limit(limit).Find(&out).Error
	return out, err
}

// Suggestions 为「无结果」搜索给出可尝试的关键词。
//
// 关联依据是**已记录搜索词的字面子串关系**（可解释、可验证），不引入语义猜测：
// 先找包含该词的其它词，再找被该词包含的其它词，最后用热门词兜底。
func (s *Service) Suggestions(ctx context.Context, rawTerm string, limit int) ([]string, error) {
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	term := Normalize(rawTerm)
	if term == "" {
		return []string{}, nil
	}

	kw := "%" + escapeLike(term) + "%"
	var rows []domain.SearchTerm
	if err := s.db.WithContext(ctx).
		Where("term <> ?", term).
		Where("term ILIKE ? ESCAPE '\\' OR ? ILIKE '%' || term || '%'", kw, term).
		Order("hits DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Term)
	}
	if len(out) >= limit {
		return out, nil
	}

	// 兜底：补充热门搜索词。
	hot, err := s.Hot(ctx, limit)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(out))
	for _, t := range out {
		seen[t] = struct{}{}
	}
	for _, h := range hot {
		if h.Term == term {
			continue
		}
		if _, dup := seen[h.Term]; dup {
			continue
		}
		out = append(out, h.Term)
		seen[h.Term] = struct{}{}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// List 管理端分页查询搜索词统计。
// kind：hot（按搜索次数）/ no_result（按无结果次数）/ failed（按查询失败次数，仅含有失败记录的词）。
func (s *Service) List(ctx context.Context, kind, q string, page, pageSize int) ([]domain.SearchTerm, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tx := s.db.WithContext(ctx).Model(&domain.SearchTerm{})
	switch kind {
	case "no_result":
		tx = tx.Where("no_result_hits > 0").Order("no_result_hits DESC, hits DESC")
	case "failed":
		tx = tx.Where("failed_hits > 0").Order("failed_hits DESC, hits DESC")
	default:
		tx = tx.Order("hits DESC, last_seen_at DESC")
	}
	if kw := strings.TrimSpace(q); kw != "" {
		tx = tx.Where("term ILIKE ? ESCAPE '\\'", "%"+escapeLike(kw)+"%")
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.SearchTerm, 0, pageSize)
	if err := tx.Offset((page - 1) * pageSize).Limit(pageSize).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// escapeLike 转义 LIKE 通配符，避免用户输入的 % / _ 改变匹配语义。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
