package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/submission"
)

// SubmissionHandler 普通用户资源投稿处理器（登录用户专属）。
//
// 安全边界：路由层已加 RequireAuth + RequireAccount + CSRF，
// 提交者账号一律取自会话，不接受请求体传入，避免越权代他人投稿。
type SubmissionHandler struct {
	svc *submission.Service
	// submitLimiter 投稿限流（按账号）：与「待审核上限 + 来源链接去重」共同构成反垃圾。
	submitLimiter *keyedLimiter
	// growth 成长事件接收端（可选注入；未注入时为空操作）。
	growth domain.GrowthEventSink
}

// NewSubmissionHandler 创建用户投稿处理器。
func NewSubmissionHandler(svc *submission.Service) *SubmissionHandler {
	return &SubmissionHandler{
		svc:           svc,
		submitLimiter: newKeyedLimiter(rate.Every(30*time.Second), 5, 30*time.Minute),
	}
}

// WithGrowthSink 注入成长事件接收端（main 装配；链式调用）。
func (h *SubmissionHandler) WithGrowthSink(sink domain.GrowthEventSink) *SubmissionHandler {
	h.growth = sink
	return h
}

// maxSubmissionBody 投稿请求体上限（结构化字段 + 长文本，小于上传类接口）。
const maxSubmissionBody = 64 << 10

type submissionCreateRequest struct {
	Name              string   `json:"name"`
	Summary           string   `json:"summary"`
	Description       string   `json:"description"`
	ResourceType      string   `json:"resource_type"`
	AuthorName        string   `json:"author_name"`
	SourcePlatform    string   `json:"source_platform"`
	SourceURL         string   `json:"source_url"`
	License           string   `json:"license"`
	GameEdition       string   `json:"game_edition"`
	SupportedVersions []string `json:"supported_versions"`
	LoaderTypes       []string `json:"loader_types"`
	InstallGuide      string   `json:"install_guide"`
	Note              string   `json:"note"`
	// LicenseVersion 投稿授权条款版本（须与后端当前版本一致，否则 422）。
	LicenseVersion string `json:"license_version"`
}

type submissionListRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// Submit POST /api/v1/user/submissions/submit
// 提交一条资源投稿，落库为待审核。
func (h *SubmissionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	accountID, err := uuid.Parse(session.AccountID)
	if err != nil {
		response.Unauthorized(w, r)
		return
	}
	if !h.submitLimiter.allow(session.AccountID) {
		response.TooManyRequests(w, r)
		return
	}

	var req submissionCreateRequest
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSubmissionBody)).Decode(&req); err != nil {
			response.BadRequest(w, r, "请求体格式错误")
			return
		}
	}

	sub, err := h.svc.Create(r.Context(), &submission.CreateInput{
		AccountID:         accountID,
		SubmitterName:     session.Username,
		Name:              req.Name,
		Summary:           req.Summary,
		Description:       req.Description,
		ResourceType:      req.ResourceType,
		AuthorName:        req.AuthorName,
		SourcePlatform:    req.SourcePlatform,
		SourceURL:         req.SourceURL,
		License:           req.License,
		GameEdition:       req.GameEdition,
		SupportedVersions: req.SupportedVersions,
		LoaderTypes:       req.LoaderTypes,
		InstallGuide:      req.InstallGuide,
		Note:              req.Note,
		LicenseVersion:    req.LicenseVersion,
	})
	if err != nil {
		writeSubmissionError(w, r, err)
		return
	}
	// 成长事件：有效投稿（subject 为投稿 ID，同一投稿只奖励一次）。
	emitGrowth(r.Context(), h.growth, "submission", domain.EventResourceSubmission, session.AccountID, "submission", sub.ID.String())
	response.Created(w, sub)
}

// ListMine POST /api/v1/user/submissions/list
// 返回当前账号的投稿列表（含审核状态）。
func (h *SubmissionHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		response.Unauthorized(w, r)
		return
	}
	accountID, err := uuid.Parse(session.AccountID)
	if err != nil {
		response.Unauthorized(w, r)
		return
	}
	var req submissionListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.ListMine(r.Context(), accountID, page, pageSize)
	if err != nil {
		writeSubmissionError(w, r, err)
		return
	}
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

// SubmissionAdminHandler 资源投稿审核处理器（后台，需 content.resources 权限）。
//
// 权限选择说明：审核通过会真正创建 resources 记录，与「资源管理」是同一写入权能，
// 因此复用 content.resources，不新增权限点。
type SubmissionAdminHandler struct {
	svc      *submission.Service
	auditSvc *audit.Service
	// growth 成长事件接收端（可选注入）：审核通过时给投稿人发放「投稿审核通过」经验。
	growth domain.GrowthEventSink
}

// NewSubmissionAdminHandler 创建审核处理器。auditSvc 可为 nil（不写审计）。
func NewSubmissionAdminHandler(svc *submission.Service, auditSvc *audit.Service) *SubmissionAdminHandler {
	return &SubmissionAdminHandler{svc: svc, auditSvc: auditSvc}
}

// WithGrowthSink 注入成长事件接收端（main 装配；链式调用）。
func (h *SubmissionAdminHandler) WithGrowthSink(sink domain.GrowthEventSink) *SubmissionAdminHandler {
	h.growth = sink
	return h
}

type submissionAdminListRequest struct {
	Status       string `json:"status"`
	ResourceType string `json:"resource_type"`
	Q            string `json:"q"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
}

type submissionIDRequest struct {
	ID string `json:"id"`
}

type submissionReviewRequest struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Note    string `json:"note"`
	Publish bool   `json:"publish"`
}

// List POST /api/v1/admin/submissions/list
// 按状态 / 类型 / 关键词筛选投稿（不过滤状态，供审核使用）。
func (h *SubmissionAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	var req submissionAdminListRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	items, total, err := h.svc.List(r.Context(), submission.Filter{
		Status:       req.Status,
		ResourceType: req.ResourceType,
		Q:            req.Q,
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeSubmissionError(w, r, err)
		return
	}
	response.Page(w, r, items, response.ResourceList{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages(total, pageSize),
	})
}

// Detail POST /api/v1/admin/submissions/detail
func (h *SubmissionAdminHandler) Detail(w http.ResponseWriter, r *http.Request) {
	var req submissionIDRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		response.BadRequest(w, r, "缺少投稿 ID")
		return
	}
	sub, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeSubmissionError(w, r, err)
		return
	}
	response.OK(w, sub)
}

// Review POST /api/v1/admin/submissions/review
// 审核投稿：status 为 approved（通过并生成资源）或 rejected（驳回）。
func (h *SubmissionAdminHandler) Review(w http.ResponseWriter, r *http.Request) {
	var req submissionReviewRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		response.BadRequest(w, r, "缺少投稿 ID")
		return
	}
	reviewerID, _ := uuid.Parse(actorID(r))
	sub, res, err := h.svc.Review(r.Context(), submission.ReviewInput{
		ID:         id,
		Status:     req.Status,
		Reviewer:   actorName(r),
		ReviewerID: reviewerID,
		Note:       req.Note,
		Publish:    req.Publish,
	})
	if err != nil {
		writeSubmissionError(w, r, err)
		return
	}
	if h.auditSvc != nil {
		after := map[string]any{
			"status":   sub.Status,
			"reviewer": sub.Reviewer,
			"note":     sub.ReviewNote,
		}
		if res != nil {
			after["resource_id"] = res.ID.String()
			after["resource_status"] = res.Status
		}
		h.auditSvc.Write(r, audit.Entry{
			ActorAdminID: actorID(r),
			Action:       "resource_submission.review",
			ObjectType:   "resource_submission",
			ObjectID:     sub.ID.String(),
			After:        after,
		})
	}
	out := map[string]any{"submission": sub}
	if res != nil {
		out["resource"] = res
	}
	// 成长事件：投稿审核通过（奖励投稿人；subject 为生成的资源 ID，同一资源只奖励一次）。
	if res != nil {
		emitGrowth(r.Context(), h.growth, "submission", domain.EventResourceApproved, sub.AccountID.String(), "resource", res.ID.String())
	}
	response.OK(w, out)
}

// submissionBatchReviewRequest 批量审核请求（最多 domain.MaxBatchReview 条）。
type submissionBatchReviewRequest struct {
	IDs     []string `json:"ids"`
	Status  string   `json:"status"`
	Note    string   `json:"note"`
	Publish bool     `json:"publish"`
}

// BatchReview POST /api/v1/admin/submissions/batch-review
//
// 批量通过 / 驳回投稿：逐条独立事务，返回逐条结果（部分成功是正常情况）。
// 每条成功记录都写一条审计，与被单条审核时的粒度保持一致。
func (h *SubmissionAdminHandler) BatchReview(w http.ResponseWriter, r *http.Request) {
	var req submissionBatchReviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "请求体格式错误")
		return
	}
	if len(req.IDs) == 0 {
		response.Validation(w, r, []response.ErrorDetail{{Field: "ids", Message: "请至少选择一条投稿"}})
		return
	}
	if len(req.IDs) > domain.MaxBatchReview {
		response.Validation(w, r, []response.ErrorDetail{{
			Field:   "ids",
			Message: fmt.Sprintf("单次最多 %d 条，请分批提交", domain.MaxBatchReview),
		}})
		return
	}

	ids := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		// 全零 UUID 虽能解析，但服务层会按「空 ID」丢弃，最终表现为 500；
		// 在入口直接拒绝，保持 422 语义。
		if err != nil || id == uuid.Nil {
			response.Validation(w, r, []response.ErrorDetail{{Field: "ids", Message: "存在非法的投稿 ID"}})
			return
		}
		ids = append(ids, id)
	}

	reviewerID, _ := uuid.Parse(actorID(r))
	result, err := h.svc.BatchReview(r.Context(), submission.BatchReviewInput{
		IDs:        ids,
		Status:     req.Status,
		Reviewer:   actorName(r),
		ReviewerID: reviewerID,
		Note:       req.Note,
		Publish:    req.Publish,
		OnApproved: func(sub *domain.ResourceSubmission, res *domain.Resource) {
			// 成长事件：同一资源只奖励一次（emitGrowth 内部按 subject 去重）。
			emitGrowth(r.Context(), h.growth, "submission", domain.EventResourceApproved, sub.AccountID.String(), "resource", res.ID.String())
		},
	})
	if err != nil {
		writeSubmissionError(w, r, err)
		return
	}

	if h.auditSvc != nil {
		for _, item := range result.Items {
			if !item.OK {
				continue
			}
			h.auditSvc.Write(r, audit.Entry{
				ActorAdminID: actorID(r),
				Action:       "resource_submission.review",
				ObjectType:   "resource_submission",
				ObjectID:     item.ID,
				After: map[string]any{
					"status":   req.Status,
					"reviewer": actorName(r),
					"note":     req.Note,
					"batch":    true,
				},
			})
		}
	}
	response.OK(w, result)
}

// writeSubmissionError 统一映射投稿相关错误。
func writeSubmissionError(w http.ResponseWriter, r *http.Request, err error) {
	var fe *submission.FieldError
	switch {
	case errors.As(err, &fe):
		response.Validation(w, r, []response.ErrorDetail{{Field: fe.Field, Message: fe.Message}})
	case errors.Is(err, submission.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, submission.ErrDuplicate):
		response.Error(w, r, http.StatusConflict, "SUBMISSION_DUPLICATE", err.Error(), nil)
	case errors.Is(err, submission.ErrAlreadyReviewed):
		response.Error(w, r, http.StatusConflict, "SUBMISSION_REVIEWED", err.Error(), nil)
	case errors.Is(err, submission.ErrTooManyPending):
		response.Error(w, r, http.StatusTooManyRequests, "SUBMISSION_LIMIT", err.Error(), nil)
	case errors.Is(err, submission.ErrBadStatus):
		response.Validation(w, r, []response.ErrorDetail{{Field: "status", Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
