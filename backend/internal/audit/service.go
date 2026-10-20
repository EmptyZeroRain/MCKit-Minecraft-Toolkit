// Package audit 提供审计日志写入与查询。
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
)

// Service 审计日志服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建审计服务。
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Entry 一次审计事件。
type Entry struct {
	ActorAdminID string
	Action       string
	ObjectType   string
	ObjectID     string
	Before       map[string]any
	After        map[string]any
	SourceIP     string
	Result       string
}

// Write 从请求上下文写入审计日志。
// r 用于提取请求 ID 与来源 IP；可传 nil（如命令初始化场景）。
func (s *Service) Write(r *http.Request, e Entry) {
	var requestID, sourceIP string
	if r != nil {
		requestID = middleware.RequestIDFromCtx(r.Context())
		sourceIP = middleware.ClientIP(r)
	}

	result := e.Result
	if result == "" {
		result = domain.AuditSuccess
	}

	log := domain.AuditLog{
		ActorAdminID:  parseUUIDPtr(e.ActorAdminID),
		Action:        e.Action,
		ObjectType:    e.ObjectType,
		ObjectID:      parseUUIDPtr(e.ObjectID),
		BeforeSummary: marshalJSON(e.Before),
		AfterSummary:  marshalJSON(e.After),
		RequestID:     requestID,
		SourceIP:      sourceIP,
		Result:        result,
	}
	// 审计写入失败不阻断业务，仅降级为日志输出。
	if err := s.db.Create(&log).Error; err != nil {
		slog.Warn("审计日志写入失败", "err", err)
	}
}

// SecurityActionPrefixes 与账号安全直接相关的动作前缀。
//
// 「安全事件视图」只筛这些动作（登录、改密、MFA、会话撤销），
// 避免把内容审核等业务操作混进安全审计。session.admin_revoke 为后台强制踢下线。
var SecurityActionPrefixes = []string{
	"admin.login",
	"admin.password",
	"admin.mfa",
	"admin.revoke_sessions",
	"session.admin_revoke",
}

// ListFilter 审计日志查询筛选。
type ListFilter struct {
	ActorID string
	Action  string
	// ObjectType 对象类型精确匹配（如 resource / admin_job / ops_alert）。
	ObjectType string
	// Result 结果精确匹配：success / failure。
	Result string
	// ActionPrefixes 动作前缀（OR 关系），用于「安全事件」这类聚合视图。
	ActionPrefixes []string
	// SourceIP 来源 IP 精确匹配。
	SourceIP string
	Page     int
	PageSize int
}

// ExportMax 单次导出上限：审计日志增长快，避免一次导出把内存打满。
const ExportMax = 5000

// Export 按筛选条件导出审计日志（不分页，最多 ExportMax 条）。
// truncated 表示结果被上限截断，调用方应在响应头中提示。
func (s *Service) Export(ctx context.Context, f ListFilter) (items []LogItem, truncated bool, err error) {
	f.Page = 1
	f.PageSize = ExportMax
	// 直接复用 List 的筛选逻辑，避免两套 SQL 漂移。
	items, total, err := s.List(f)
	if err != nil {
		return nil, false, err
	}
	return items, total > int64(len(items)), nil
}

// LogItem 返回给管理端的审计日志条目。
type LogItem struct {
	ID            string         `json:"id"`
	ActorAdminID  *string        `json:"actor_admin_id"`
	Action        string         `json:"action"`
	ObjectType    string         `json:"object_type"`
	ObjectID      *string        `json:"object_id"`
	BeforeSummary map[string]any `json:"before_summary"`
	AfterSummary  map[string]any `json:"after_summary"`
	RequestID     string         `json:"request_id"`
	SourceIP      string         `json:"source_ip"`
	Result        string         `json:"result"`
	CreatedAt     string         `json:"created_at"`
}

// List 分页查询审计日志。
func (s *Service) List(f ListFilter) ([]LogItem, int64, error) {
	q := s.db.Model(&domain.AuditLog{})
	if f.ActorID != "" {
		q = q.Where("actor_admin_id = ?", f.ActorID)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	// 以下三项是 gj.md 阶段三 ⑨ 的修复点：前端一直在传，后端此前直接忽略，
	// 造成「筛选条件看起来生效、结果却没变」的静默错误。
	if f.ObjectType != "" {
		q = q.Where("object_type = ?", f.ObjectType)
	}
	if f.Result != "" {
		q = q.Where("result = ?", f.Result)
	}
	if f.SourceIP != "" {
		q = q.Where("source_ip = ?", f.SourceIP)
	}
	if len(f.ActionPrefixes) > 0 {
		conds := make([]string, 0, len(f.ActionPrefixes))
		args := make([]any, 0, len(f.ActionPrefixes))
		for _, p := range f.ActionPrefixes {
			if p == "" {
				continue
			}
			conds = append(conds, "action LIKE ?")
			args = append(args, p+"%")
		}
		if len(conds) > 0 {
			q = q.Where("("+strings.Join(conds, " OR ")+")", args...)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	page := f.Page
	if page < 1 {
		page = 1
	}

	var logs []domain.AuditLog
	err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]LogItem, 0, len(logs))
	for _, l := range logs {
		items = append(items, LogItem{
			ID:            l.ID.String(),
			ActorAdminID:  uuidPtrToString(l.ActorAdminID),
			Action:        l.Action,
			ObjectType:    l.ObjectType,
			ObjectID:      uuidPtrToString(l.ObjectID),
			BeforeSummary: unmarshalJSON(l.BeforeSummary),
			AfterSummary:  unmarshalJSON(l.AfterSummary),
			RequestID:     l.RequestID,
			SourceIP:      l.SourceIP,
			Result:        l.Result,
			CreatedAt:     l.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return items, total, nil
}

func parseUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func uuidPtrToString(p *uuid.UUID) *string {
	if p == nil {
		return nil
	}
	s := p.String()
	return &s
}

func marshalJSON(m map[string]any) string {
	if len(m) == 0 {
		return "null"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "null"
	}
	return string(b)
}

func unmarshalJSON(s string) map[string]any {
	if s == "" || s == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}
