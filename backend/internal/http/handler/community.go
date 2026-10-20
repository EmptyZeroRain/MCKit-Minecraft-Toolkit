package handler

import (
	"errors"
	"net/http"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/community"
)

// CommunityHandler 社区帖子 / 留言 / 点赞处理器。
//
// 公开读接口通过 OptionalAuth 识别登录者（仅用于返回 liked 状态），
// 写接口在路由层叠加 RequireAuth + RequireAccount + CSRF + RequirePasswordChanged。
type CommunityHandler struct {
	svc    *community.Service
	growth domain.GrowthEventSink
	audit  *audit.Service
}

// NewCommunityHandler 创建社区处理器。
func NewCommunityHandler(svc *community.Service) *CommunityHandler {
	return &CommunityHandler{svc: svc}
}

// WithGrowthSink 注入成长事件接收端（main 装配；链式调用）。
func (h *CommunityHandler) WithGrowthSink(sink domain.GrowthEventSink) *CommunityHandler {
	h.growth = sink
	return h
}

// WithAudit 注入审计服务（后台审核操作留痕）。
func (h *CommunityHandler) WithAudit(svc *audit.Service) *CommunityHandler {
	h.audit = svc
	return h
}

// auditLog 写后台社区审核审计日志；未注入审计服务时跳过。
func (h *CommunityHandler) auditLog(r *http.Request, action, objectType, objectID string, after map[string]any) {
	if h.audit == nil {
		return
	}
	h.audit.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   objectType,
		ObjectID:     objectID,
		After:        after,
	})
}

// ---- 请求体 ----

type communityListRequest struct {
	Category string `json:"category"`
	Q        string `json:"q"`
	Sort     string `json:"sort"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type communityPostDetailRequest struct {
	ID string `json:"id"`
}

type communityPostCreateRequest struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type communityPostDeleteRequest struct {
	ID string `json:"id"`
}

type communityCommentListRequest struct {
	PostID   string `json:"post_id"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type communityCommentCreateRequest struct {
	PostID string `json:"post_id"`
	Body   string `json:"body"`
}

type communityCommentDeleteRequest struct {
	ID string `json:"id"`
}

type communityLikeRequest struct {
	PostID string `json:"post_id"`
}

type communityAdminListRequest struct {
	Status   string `json:"status"`
	Category string `json:"category"`
	Q        string `json:"q"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type communityAdminStatusRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type communityAdminCommentsRequest struct {
	PostID   string `json:"post_id"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

// ---- 公开 / 用户端 ----

// ListPosts POST /api/v1/community/posts
func (h *CommunityHandler) ListPosts(w http.ResponseWriter, r *http.Request) {
	var req communityListRequest
	_ = decodeOptionalBody(w, r, &req)
	viewer := ""
	if s := middleware.SessionFrom(r.Context()); s != nil {
		viewer = s.AccountID
	}
	items, total, err := h.svc.ListPosts(r.Context(), community.ListFilter{
		Category: req.Category,
		Q:        req.Q,
		Sort:     req.Sort,
		Page:     req.Page,
		PageSize: req.PageSize,
		ViewerID: viewer,
	})
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// PostDetail POST /api/v1/community/posts/detail
func (h *CommunityHandler) PostDetail(w http.ResponseWriter, r *http.Request) {
	var req communityPostDetailRequest
	if !decodeBody(w, r, &req) {
		return
	}
	viewer := ""
	if s := middleware.SessionFrom(r.Context()); s != nil {
		viewer = s.AccountID
	}
	post, err := h.svc.GetPost(r.Context(), req.ID, viewer)
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	response.OK(w, post)
}

// ListComments POST /api/v1/community/comments
func (h *CommunityHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	var req communityCommentListRequest
	_ = decodeOptionalBody(w, r, &req)
	items, total, err := h.svc.ListComments(r.Context(), req.PostID, req.Page, req.PageSize, false)
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// CreatePost POST /api/v1/community/posts/create（CSRF）
func (h *CommunityHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req communityPostCreateRequest
	if !decodeBody(w, r, &req) {
		return
	}
	post, err := h.svc.CreatePost(r.Context(), userID, req.Category, req.Title, req.Body)
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	response.Created(w, post)
}

// DeletePost POST /api/v1/community/posts/delete（CSRF）
func (h *CommunityHandler) DeletePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req communityPostDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeletePost(r.Context(), userID, req.ID); err != nil {
		writeCommunityError(w, r, err)
		return
	}
	response.NoContent(w)
}

// CreateComment POST /api/v1/community/comments/create（CSRF）
func (h *CommunityHandler) CreateComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req communityCommentCreateRequest
	if !decodeBody(w, r, &req) {
		return
	}
	comment, err := h.svc.CreateComment(r.Context(), userID, req.PostID, req.Body)
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	// 成长事件：有效留言（subject 为帖子，成长侧按规则做幂等）。
	emitGrowth(r.Context(), h.growth, "community", domain.EventPostCommentCreated, userID, "post", req.PostID)
	response.Created(w, comment)
}

// DeleteComment POST /api/v1/community/comments/delete（CSRF）
func (h *CommunityHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req communityCommentDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.DeleteComment(r.Context(), userID, req.ID); err != nil {
		writeCommunityError(w, r, err)
		return
	}
	// 成长事件：留言删除（成长侧无奖励规则，仅保留事件事实供后续冲正 / 审计）。
	emitGrowth(r.Context(), h.growth, "community", domain.EventPostCommentDeleted, userID, "comment", req.ID)
	response.NoContent(w)
}

// ToggleLike POST /api/v1/community/posts/like（CSRF）
func (h *CommunityHandler) ToggleLike(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	var req communityLikeRequest
	if !decodeBody(w, r, &req) {
		return
	}
	liked, count, err := h.svc.ToggleLike(r.Context(), userID, req.PostID)
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	// 成长事件：仅在「新增点赞」时投递；取消点赞不奖励。
	if liked {
		emitGrowth(r.Context(), h.growth, "community", domain.EventPostLiked, userID, "post", req.PostID)
	} else {
		emitGrowth(r.Context(), h.growth, "community", domain.EventPostUnliked, userID, "post", req.PostID)
	}
	response.OK(w, map[string]any{"liked": liked, "like_count": count})
}

// ---- 后台审核 ----

// AdminListPosts POST /api/v1/admin/community/posts/list
func (h *CommunityHandler) AdminListPosts(w http.ResponseWriter, r *http.Request) {
	var req communityAdminListRequest
	_ = decodeOptionalBody(w, r, &req)
	items, total, err := h.svc.AdminListPosts(r.Context(), community.AdminListFilter{
		Status:   req.Status,
		Category: req.Category,
		Q:        req.Q,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// AdminSetPostStatus POST /api/v1/admin/community/posts/status
func (h *CommunityHandler) AdminSetPostStatus(w http.ResponseWriter, r *http.Request) {
	var req communityAdminStatusRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.SetPostStatus(r.Context(), req.ID, req.Status); err != nil {
		writeCommunityError(w, r, err)
		return
	}
	h.auditLog(r, "community.post.status", "community_post", req.ID, map[string]any{"status": req.Status})
	response.NoContent(w)
}

// AdminDeletePost POST /api/v1/admin/community/posts/delete
func (h *CommunityHandler) AdminDeletePost(w http.ResponseWriter, r *http.Request) {
	var req communityPostDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.AdminDeletePost(r.Context(), req.ID); err != nil {
		writeCommunityError(w, r, err)
		return
	}
	h.auditLog(r, "community.post.delete", "community_post", req.ID, nil)
	response.NoContent(w)
}

// AdminListComments POST /api/v1/admin/community/comments/list
func (h *CommunityHandler) AdminListComments(w http.ResponseWriter, r *http.Request) {
	var req communityAdminCommentsRequest
	_ = decodeOptionalBody(w, r, &req)
	items, total, err := h.svc.AdminListComments(r.Context(), req.PostID, req.Page, req.PageSize)
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// AdminListAllComments POST /api/v1/admin/community/comments/all
// 跨帖子查看全部留言，支持状态与关键词筛选（正文 / 帖子标题 / 作者）。
func (h *CommunityHandler) AdminListAllComments(w http.ResponseWriter, r *http.Request) {
	var req communityAdminListRequest
	_ = decodeOptionalBody(w, r, &req)
	items, total, err := h.svc.AdminListAllComments(r.Context(), community.AdminCommentFilter{
		Status:   req.Status,
		Q:        req.Q,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		writeCommunityError(w, r, err)
		return
	}
	page, pageSize := normalizePage(req.Page, req.PageSize)
	writeGrowthPage(w, r, items, total, page, pageSize)
}

// AdminSetCommentStatus POST /api/v1/admin/community/comments/status
func (h *CommunityHandler) AdminSetCommentStatus(w http.ResponseWriter, r *http.Request) {
	var req communityAdminStatusRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.SetCommentStatus(r.Context(), req.ID, req.Status); err != nil {
		writeCommunityError(w, r, err)
		return
	}
	h.auditLog(r, "community.comment.status", "community_comment", req.ID, map[string]any{"status": req.Status})
	response.NoContent(w)
}

// AdminDeleteComment POST /api/v1/admin/community/comments/delete
func (h *CommunityHandler) AdminDeleteComment(w http.ResponseWriter, r *http.Request) {
	var req communityCommentDeleteRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if err := h.svc.AdminDeleteComment(r.Context(), req.ID); err != nil {
		writeCommunityError(w, r, err)
		return
	}
	h.auditLog(r, "community.comment.delete", "community_comment", req.ID, nil)
	response.NoContent(w)
}

// writeCommunityError 统一映射社区服务错误。
func writeCommunityError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, community.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, community.ErrForbidden):
		response.Forbidden(w, r)
	case errors.Is(err, community.ErrBadCategory),
		errors.Is(err, community.ErrBadTitle),
		errors.Is(err, community.ErrBadBody),
		errors.Is(err, community.ErrBadComment),
		errors.Is(err, community.ErrBadSort),
		errors.Is(err, community.ErrBadStatus),
		errors.Is(err, community.ErrBadRequest),
		errors.Is(err, community.ErrBlockedContent):
		response.Validation(w, r, []response.ErrorDetail{{Message: err.Error()}})
	default:
		response.HandleError(w, r, err)
	}
}
