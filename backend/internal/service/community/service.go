// Package community 提供社区帖子 / 留言 / 点赞服务。
//
// 安全与边界：
//   - 公开读只返回 status = 'published' 的帖子与留言；
//   - 写操作一律由路由层保证「普通用户会话 + CSRF + 已完成改密」；
//   - 删除只允许作者本人（留言额外允许帖子作者清理自己帖子下的留言）；
//   - 后台审核通过 content.community 权限点控制，可隐藏 / 删除任意内容。
//
// 计数策略：点赞数与留言数在读取时实时聚合，不维护冗余计数列，
// 避免删除 / 隐藏 / 并发导致计数漂移。
package community

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/service/contentfilter"
)

// 内容长度上限（按 Unicode 字符计）。
const (
	MaxTitleLength   = 120
	MaxBodyLength    = 10000
	MaxCommentLength = 2000
	// 列表分页上限。
	MaxPageSize = 50
)

// 服务级错误（处理层据此映射状态码）。
var (
	ErrNotFound       = errors.New("内容不存在或已下线")
	ErrForbidden      = errors.New("没有权限操作该内容")
	ErrBadCategory    = errors.New("帖子分类不合法")
	ErrBadTitle       = errors.New("标题需为 1-120 个字符")
	ErrBadBody        = errors.New("正文需为 1-10000 个字符")
	ErrBadComment     = errors.New("留言需为 1-2000 个字符")
	ErrBadSort        = errors.New("排序方式不合法")
	ErrBadStatus      = errors.New("审核状态不合法")
	ErrBadRequest     = errors.New("请求参数不合法")
	ErrBlockedContent = errors.New("内容包含被屏蔽关键字，请修改后重试")
)

// Service 社区服务。
type Service struct {
	db     *gorm.DB
	filter *contentfilter.Service
}

// NewService 创建社区服务。
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// WithContentFilter 注入内容关键字屏蔽服务；未注入时跳过屏蔽校验。
func (s *Service) WithContentFilter(filter *contentfilter.Service) *Service {
	s.filter = filter
	return s
}

// AuthorView 作者公开信息（不含内部 UUID、邮箱等隐私字段）。
type AuthorView struct {
	UserNo   string `json:"user_no"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
}

// PostView 帖子视图（列表与详情共用）。
type PostView struct {
	ID           string     `json:"id"`
	Category     string     `json:"category"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Status       string     `json:"status"`
	Author       AuthorView `json:"author"`
	LikeCount    int64      `json:"like_count"`
	CommentCount int64      `json:"comment_count"`
	Liked        bool       `json:"liked"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CommentView 留言视图。
type CommentView struct {
	ID        string     `json:"id"`
	PostID    string     `json:"post_id"`
	PostTitle string     `json:"post_title,omitempty"`
	Body      string     `json:"body"`
	Status    string     `json:"status"`
	Author    AuthorView `json:"author"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ListFilter 帖子列表筛选。
type ListFilter struct {
	Category string
	Q        string
	Sort     string // latest（默认）/ hot
	Page     int
	PageSize int
	// ViewerID 当前登录账号（匿名传空），仅用于计算 liked。
	ViewerID string
}

// AdminListFilter 后台帖子列表筛选（可查看隐藏内容）。
type AdminListFilter struct {
	Status   string
	Category string
	Q        string
	Page     int
	PageSize int
}

// AdminCommentFilter 后台全站留言筛选。
type AdminCommentFilter struct {
	Status   string
	Q        string
	Page     int
	PageSize int
}

// postRow 列表 / 详情查询的中间扫描结构。
type postRow struct {
	ID             uuid.UUID
	Category       string
	Title          string
	Body           string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AuthorUserNo   string
	AuthorUsername string
	AuthorNickname string
	LikeCount      int64
	CommentCount   int64
	Liked          bool
}

// commentRow 留言查询的中间扫描结构。
type commentRow struct {
	ID             uuid.UUID
	PostID         uuid.UUID
	PostTitle      string
	Body           string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	AuthorUserNo   string
	AuthorUsername string
	AuthorNickname string
}

// postColumns 公开帖子查询列（含作者与实时计数）。
const postColumns = `p.id, p.category, p.title, p.body, p.status, p.created_at, p.updated_at,
	a.user_no AS author_user_no, a.username AS author_username,
	a.nickname AS author_nickname,
	(SELECT count(*) FROM community_likes l WHERE l.post_id = p.id) AS like_count,
	(SELECT count(*) FROM community_comments c WHERE c.post_id = p.id AND c.status = 'published') AS comment_count,
	EXISTS(SELECT 1 FROM community_likes l2 WHERE l2.post_id = p.id AND l2.account_id = ?) AS liked`

// ListPosts 公开帖子列表（仅 published）。
func (s *Service) ListPosts(ctx context.Context, f ListFilter) ([]PostView, int64, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)
	base := s.db.WithContext(ctx).Table("community_posts p").
		Joins("JOIN accounts a ON a.id = p.author_id").
		Where("p.status = ?", domain.CommunityStatusPublished)

	if category := strings.TrimSpace(f.Category); category != "" {
		if !domain.IsValidCommunityCategory(category) {
			return nil, 0, ErrBadCategory
		}
		base = base.Where("p.category = ?", category)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		base = base.Where("p.title ILIKE ? OR p.body ILIKE ?", "%"+q+"%", "%"+q+"%")
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := "p.created_at DESC"
	switch strings.TrimSpace(f.Sort) {
	case "", "latest":
	case "hot":
		order = `((SELECT count(*) FROM community_likes l WHERE l.post_id = p.id)
			+ (SELECT count(*) FROM community_comments c WHERE c.post_id = p.id AND c.status = 'published')) DESC,
			p.created_at DESC`
	default:
		return nil, 0, ErrBadSort
	}

	var rows []postRow
	if err := base.Select(postColumns, viewerID(f.ViewerID)).
		Order(order).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toPostViews(rows), total, nil
}

// GetPost 公开帖子详情（仅 published）。
func (s *Service) GetPost(ctx context.Context, id, viewer string) (*PostView, error) {
	pid, err := parseID(id)
	if err != nil {
		return nil, ErrNotFound
	}
	var rows []postRow
	if err := s.db.WithContext(ctx).Table("community_posts p").
		Select(postColumns, viewerID(viewer)).
		Joins("JOIN accounts a ON a.id = p.author_id").
		Where("p.id = ? AND p.status = ?", pid, domain.CommunityStatusPublished).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	views := toPostViews(rows)
	return &views[0], nil
}

// CreatePost 创建帖子；分类 / 标题 / 正文均由服务端校验。
func (s *Service) CreatePost(ctx context.Context, authorID, category, title, body string) (*PostView, error) {
	author, err := parseID(authorID)
	if err != nil {
		return nil, ErrBadRequest
	}
	category = strings.TrimSpace(category)
	if !domain.IsValidCommunityCategory(category) {
		return nil, ErrBadCategory
	}
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > MaxTitleLength {
		return nil, ErrBadTitle
	}
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > MaxBodyLength {
		return nil, ErrBadBody
	}
	if err := s.checkBlocked(ctx, title+"\n"+body); err != nil {
		return nil, err
	}
	post := &domain.CommunityPost{
		AuthorID: author,
		Category: category,
		Title:    title,
		Body:     body,
		Status:   domain.CommunityStatusPublished,
	}
	if err := s.db.WithContext(ctx).Create(post).Error; err != nil {
		return nil, err
	}
	return s.GetPost(ctx, post.ID.String(), authorID)
}

// DeletePost 作者删除自己的帖子（物理删除，级联清除留言与点赞）。
func (s *Service) DeletePost(ctx context.Context, userID, postID string) error {
	uid, err := parseID(userID)
	if err != nil {
		return ErrBadRequest
	}
	pid, err := parseID(postID)
	if err != nil {
		return ErrNotFound
	}
	var post domain.CommunityPost
	if err := s.db.WithContext(ctx).Select("id", "author_id").
		Where("id = ?", pid).First(&post).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if post.AuthorID != uid {
		return ErrForbidden
	}
	return s.db.WithContext(ctx).Where("id = ?", pid).Delete(&domain.CommunityPost{}).Error
}

// ListComments 公开留言列表（按时间正序）。
func (s *Service) ListComments(ctx context.Context, postID string, page, pageSize int, includeHidden bool) ([]CommentView, int64, error) {
	pid, err := parseID(postID)
	if err != nil {
		return nil, 0, ErrNotFound
	}
	// 公开读必须先确认帖子本身可见，避免通过留言接口探测隐藏帖子。
	if !includeHidden {
		var status string
		if err := s.db.WithContext(ctx).Model(&domain.CommunityPost{}).
			Select("status").Where("id = ?", pid).Scan(&status).Error; err != nil {
			return nil, 0, err
		}
		if status != domain.CommunityStatusPublished {
			return nil, 0, ErrNotFound
		}
	}

	page, pageSize = normalizePage(page, pageSize)
	base := s.db.WithContext(ctx).Table("community_comments c").
		Joins("JOIN accounts a ON a.id = c.author_id").
		Joins("JOIN community_posts p ON p.id = c.post_id").
		Where("c.post_id = ?", pid)
	if !includeHidden {
		base = base.Where("c.status = ?", domain.CommunityStatusPublished)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []commentRow
	if err := base.Select(`c.id, c.post_id, p.title AS post_title, c.body, c.status,
		c.created_at, c.updated_at,
		a.user_no AS author_user_no, a.username AS author_username,
		a.nickname AS author_nickname`).
		Order("c.created_at ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toCommentViews(rows), total, nil
}

// CreateComment 创建留言；帖子必须存在且处于 published。
func (s *Service) CreateComment(ctx context.Context, authorID, postID, body string) (*CommentView, error) {
	uid, err := parseID(authorID)
	if err != nil {
		return nil, ErrBadRequest
	}
	pid, err := parseID(postID)
	if err != nil {
		return nil, ErrNotFound
	}
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > MaxCommentLength {
		return nil, ErrBadComment
	}
	if err := s.checkBlocked(ctx, body); err != nil {
		return nil, err
	}
	// 只允许在公开帖下留言：隐藏 / 已删除帖子一律视为不存在。
	var status string
	if err := s.db.WithContext(ctx).Model(&domain.CommunityPost{}).
		Select("status").Where("id = ?", pid).Scan(&status).Error; err != nil {
		return nil, err
	}
	if status != domain.CommunityStatusPublished {
		return nil, ErrNotFound
	}

	c := &domain.CommunityComment{
		PostID:   pid,
		AuthorID: uid,
		Body:     body,
		Status:   domain.CommunityStatusPublished,
	}
	if err := s.db.WithContext(ctx).Create(c).Error; err != nil {
		return nil, err
	}
	var rows []commentRow
	if err := s.db.WithContext(ctx).Table("community_comments c").
		Select(`c.id, c.post_id, p.title AS post_title, c.body, c.status,
			c.created_at, c.updated_at,
			a.user_no AS author_user_no, a.username AS author_username,
			a.nickname AS author_nickname`).
		Joins("JOIN accounts a ON a.id = c.author_id").
		Joins("JOIN community_posts p ON p.id = c.post_id").
		Where("c.id = ?", c.ID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	views := toCommentViews(rows)
	if len(views) == 0 {
		return nil, ErrNotFound
	}
	return &views[0], nil
}

// DeleteComment 删除留言：留言作者本人，或该帖作者（清理自己帖子下的留言）。
func (s *Service) DeleteComment(ctx context.Context, userID, commentID string) error {
	uid, err := parseID(userID)
	if err != nil {
		return ErrBadRequest
	}
	cid, err := parseID(commentID)
	if err != nil {
		return ErrNotFound
	}
	var c domain.CommunityComment
	if err := s.db.WithContext(ctx).Select("id", "post_id", "author_id").
		Where("id = ?", cid).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if c.AuthorID != uid {
		var post domain.CommunityPost
		if err := s.db.WithContext(ctx).Select("author_id").Where("id = ?", c.PostID).First(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if post.AuthorID != uid {
			return ErrForbidden
		}
	}
	return s.db.WithContext(ctx).Where("id = ?", cid).Delete(&domain.CommunityComment{}).Error
}

// ToggleLike 点赞 / 取消点赞，返回操作后的状态与最新点赞数。
func (s *Service) ToggleLike(ctx context.Context, userID, postID string) (bool, int64, error) {
	uid, err := parseID(userID)
	if err != nil {
		return false, 0, ErrBadRequest
	}
	pid, err := parseID(postID)
	if err != nil {
		return false, 0, ErrNotFound
	}
	var status string
	if err := s.db.WithContext(ctx).Model(&domain.CommunityPost{}).
		Select("status").Where("id = ?", pid).Scan(&status).Error; err != nil {
		return false, 0, err
	}
	if status != domain.CommunityStatusPublished {
		return false, 0, ErrNotFound
	}

	liked := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&domain.CommunityLike{PostID: pid, AccountID: uid})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			liked = true
			return nil
		}
		// 已点赞 → 取消。
		return tx.Where("post_id = ? AND account_id = ?", pid, uid).
			Delete(&domain.CommunityLike{}).Error
	})
	if err != nil {
		return false, 0, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&domain.CommunityLike{}).
		Where("post_id = ?", pid).Count(&count).Error; err != nil {
		return liked, 0, err
	}
	return liked, count, nil
}

// ---- 后台审核 ----

// AdminListPosts 后台帖子列表：可按状态 / 分类 / 关键词筛选，含隐藏内容。
func (s *Service) AdminListPosts(ctx context.Context, f AdminListFilter) ([]PostView, int64, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)
	base := s.db.WithContext(ctx).Table("community_posts p").
		Joins("JOIN accounts a ON a.id = p.author_id")
	if status := strings.TrimSpace(f.Status); status != "" {
		if !domain.IsValidCommunityStatus(status) {
			return nil, 0, ErrBadStatus
		}
		base = base.Where("p.status = ?", status)
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		if !domain.IsValidCommunityCategory(category) {
			return nil, 0, ErrBadCategory
		}
		base = base.Where("p.category = ?", category)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		base = base.Where("p.title ILIKE ? OR p.body ILIKE ?", "%"+q+"%", "%"+q+"%")
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []postRow
	// 后台视角不计算 liked（viewer 固定为 uuid.Nil）。
	if err := base.Select(postColumns, uuid.Nil).
		Order("p.created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toPostViews(rows), total, nil
}

// SetPostStatus 后台设置帖子审核状态（published / hidden）。
func (s *Service) SetPostStatus(ctx context.Context, postID, status string) error {
	pid, err := parseID(postID)
	if err != nil {
		return ErrNotFound
	}
	if !domain.IsValidCommunityStatus(status) {
		return ErrBadStatus
	}
	res := s.db.WithContext(ctx).Model(&domain.CommunityPost{}).
		Where("id = ?", pid).
		Updates(map[string]any{"status": status, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminDeletePost 后台物理删除帖子（级联留言与点赞）。
func (s *Service) AdminDeletePost(ctx context.Context, postID string) error {
	pid, err := parseID(postID)
	if err != nil {
		return ErrNotFound
	}
	res := s.db.WithContext(ctx).Where("id = ?", pid).Delete(&domain.CommunityPost{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminListComments 后台查看帖子全部留言（含隐藏）。
func (s *Service) AdminListComments(ctx context.Context, postID string, page, pageSize int) ([]CommentView, int64, error) {
	return s.ListComments(ctx, postID, page, pageSize, true)
}

// AdminListAllComments 后台全站留言列表：跨帖子检索，含隐藏内容与所属帖子标题。
func (s *Service) AdminListAllComments(ctx context.Context, f AdminCommentFilter) ([]CommentView, int64, error) {
	page, pageSize := normalizePage(f.Page, f.PageSize)
	base := s.db.WithContext(ctx).Table("community_comments c").
		Joins("JOIN accounts a ON a.id = c.author_id").
		Joins("JOIN community_posts p ON p.id = c.post_id")
	if status := strings.TrimSpace(f.Status); status != "" {
		if !domain.IsValidCommunityStatus(status) {
			return nil, 0, ErrBadStatus
		}
		base = base.Where("c.status = ?", status)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + q + "%"
		base = base.Where(
			"c.body ILIKE ? OR p.title ILIKE ? OR a.username ILIKE ? OR a.nickname ILIKE ? OR a.user_no ILIKE ?",
			like, like, like, like, like,
		)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []commentRow
	if err := base.Select(`c.id, c.post_id, p.title AS post_title, c.body, c.status,
		c.created_at, c.updated_at,
		a.user_no AS author_user_no, a.username AS author_username,
		a.nickname AS author_nickname`).
		Order("c.created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toCommentViews(rows), total, nil
}

// SetCommentStatus 后台设置留言审核状态（published / hidden）。
func (s *Service) SetCommentStatus(ctx context.Context, commentID, status string) error {
	cid, err := parseID(commentID)
	if err != nil {
		return ErrNotFound
	}
	if !domain.IsValidCommunityStatus(status) {
		return ErrBadStatus
	}
	res := s.db.WithContext(ctx).Model(&domain.CommunityComment{}).
		Where("id = ?", cid).
		Updates(map[string]any{"status": status, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminDeleteComment 后台物理删除留言。
func (s *Service) AdminDeleteComment(ctx context.Context, commentID string) error {
	cid, err := parseID(commentID)
	if err != nil {
		return ErrNotFound
	}
	res := s.db.WithContext(ctx).Where("id = ?", cid).Delete(&domain.CommunityComment{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- 内部辅助 ----

// checkBlocked 在写入数据库前同步校验关键字屏蔽规则。
func (s *Service) checkBlocked(ctx context.Context, text string) error {
	if s.filter == nil {
		return nil
	}
	matched, err := s.filter.CheckText(ctx, text)
	if err != nil {
		return err
	}
	if matched != "" {
		return ErrBlockedContent
	}
	return nil
}

func toPostViews(rows []postRow) []PostView {
	out := make([]PostView, 0, len(rows))
	for _, r := range rows {
		out = append(out, PostView{
			ID:       r.ID.String(),
			Category: r.Category,
			Title:    r.Title,
			Body:     r.Body,
			Status:   r.Status,
			Author: AuthorView{
				UserNo:   r.AuthorUserNo,
				Username: r.AuthorUsername,
				Nickname: r.AuthorNickname,
			},
			LikeCount:    r.LikeCount,
			CommentCount: r.CommentCount,
			Liked:        r.Liked,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
		})
	}
	return out
}

func toCommentViews(rows []commentRow) []CommentView {
	out := make([]CommentView, 0, len(rows))
	for _, r := range rows {
		out = append(out, CommentView{
			ID:        r.ID.String(),
			PostID:    r.PostID.String(),
			PostTitle: r.PostTitle,
			Body:      r.Body,
			Status:    r.Status,
			Author: AuthorView{
				UserNo:   r.AuthorUserNo,
				Username: r.AuthorUsername,
				Nickname: r.AuthorNickname,
			},
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return out
}

// viewerID 解析当前查看者；匿名或非法值统一返回 uuid.Nil。
func viewerID(s string) uuid.UUID {
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return uuid.Nil
	}
	return id
}

func parseID(s string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(s))
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = 20
	}
	return page, pageSize
}
