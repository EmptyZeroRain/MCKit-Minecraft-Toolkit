// Package handler 提供 HTTP 处理器。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mckit/backend/internal/audit"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/mcsrc"
	"mckit/backend/internal/mcversion"
	"mckit/backend/internal/service/catalog"
	"mckit/backend/internal/service/jobs"
)

// McsrcMappingResolver 按 MC 版本 + jar 内类名查询映射（可选依赖）。
// 未注入时映射接口返回 available=false，前端据此隐藏映射名。
type McsrcMappingResolver interface {
	LookupMappingClassByName(ctx context.Context, mcVersion, className string) (*catalog.ClassMappingLookup, error)
}

// McsrcHandler「MC 源码浏览器」后端接口。
//
// 契约与项目一致：查询接口统一 POST，参数放 JSON body。
// 公开接口只读缓存状态与索引；触发下载 / 建索引的 /prepare 任何访客可用
// （前端选中版本即自动调用），但匿名访客受「自动准备额度」限制，
// 具备 ops.mcsrc 的管理员不受限；并由路由层叠加重接口限流与并发上限。
type McsrcHandler struct {
	store  *mcsrc.Store
	client *mcversion.Client

	// perms 用于识别具备 ops.mcsrc 的管理员（豁免自动准备额度）；可为 nil。
	perms middleware.PermissionResolver
	// autoPrepareMax 匿名访客自动准备的版本数上限；<=0 表示关闭自动准备。
	autoPrepareMax int

	// auditSvc 缓存清理等运维动作的审计留痕；可为 nil（此时不记录）。
	auditSvc *audit.Service

	// mappings 版本映射数据源（「映射重命名」展示用）；可为 nil。
	mappings McsrcMappingResolver

	// jobs 后台任务服务：缓存重建是耗时动作，走任务队列而不是长连接请求。
	jobs *jobs.Service
	// cacheAlertMB 缓存占用告警阈值（MB）；仅用于前端提示，0 表示未配置。
	cacheAlertMB int
}

// NewMcsrcHandler 创建处理器。store 或 client 为 nil 时接口返回 503。
func NewMcsrcHandler(store *mcsrc.Store, client *mcversion.Client) *McsrcHandler {
	return &McsrcHandler{store: store, client: client}
}

// WithAutoPrepare 配置自动准备策略：perms 用于识别管理员，max 为匿名额度
// （<=0 表示关闭自动准备，此时 /prepare 仅对具备 ops.mcsrc 的管理员开放）。
func (h *McsrcHandler) WithAutoPrepare(perms middleware.PermissionResolver, max int) *McsrcHandler {
	h.perms = perms
	h.autoPrepareMax = max
	return h
}

// WithAudit 注入审计服务：缓存清理属运维动作，必须留痕。
func (h *McsrcHandler) WithAudit(auditSvc *audit.Service) *McsrcHandler {
	h.auditSvc = auditSvc
	return h
}

// WithCacheAlertMB 设置缓存容量告警阈值（MB），随缓存列表返回给前端展示。
func (h *McsrcHandler) WithCacheAlertMB(mb int) *McsrcHandler {
	h.cacheAlertMB = mb
	return h
}

// WithJobs 注入后台任务服务，用于「重建缓存」（下载 + 建索引耗时较长）。
func (h *McsrcHandler) WithJobs(j *jobs.Service) *McsrcHandler {
	h.jobs = j
	return h
}

// WithMappings 注入版本映射数据源，用于「映射重命名」展示。
func (h *McsrcHandler) WithMappings(m McsrcMappingResolver) *McsrcHandler {
	h.mappings = m
	return h
}

// jarRequest 状态 / 准备请求体。
type jarRequest struct {
	Version string `json:"version"`
	Side    string `json:"side"`
}

// Status POST /api/v1/mcsrc/status
// body: {version, side?}；返回该版本是否已缓存 jar、是否已建索引。
func (h *McsrcHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req jarRequest
	if !decodeJSON(w, r, &req, 4096) {
		return
	}
	st, err := h.store.Status(req.Version, req.Side)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, st)
}

// Prepared POST /api/v1/mcsrc/prepared
// body: {}；列出全部已缓存（含已建索引）的版本。
func (h *McsrcHandler) Prepared(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	list, err := h.store.ListPrepared()
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, list)
}

// CacheList POST /api/v1/admin/mcsrc/cache/list
// body: {}；列出全部已缓存版本、各自的磁盘占用与汇总（需 ops.mcsrc 权限）。
func (h *McsrcHandler) CacheList(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	items, usage, err := h.store.ListCache()
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, map[string]any{
		"items": items,
		"usage": usage,
		// 阈值随列表返回，前端可即时提示「已超过告警线」，无需再查配置。
		"alert_threshold_bytes": int64(h.cacheAlertMB) << 20,
	})
}

// CacheRebuild POST /api/v1/admin/mcsrc/cache/rebuild
// body: {version, side?}；把重建作为后台任务入队（清理 → 重新下载 → 重建索引）。
//
// 为什么不在这里同步执行：下载 jar 与建索引通常数十秒，远超反向代理超时；
// 入队后由「后台任务」页展示进度、失败原因，并可一键重试。
func (h *McsrcHandler) CacheRebuild(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	if h.jobs == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "JOBS_UNAVAILABLE", "后台任务服务未启用", nil)
		return
	}
	var req jarRequest
	if !decodeJSON(w, r, &req, 4096) {
		return
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}
	side := strings.ToLower(strings.TrimSpace(req.Side))
	if side == "" {
		side = mcsrc.SideClient
	}
	if side != mcsrc.SideClient && side != mcsrc.SideServer {
		response.Validation(w, r, []response.ErrorDetail{{Field: "side", Message: "side 仅支持 client / server"}})
		return
	}

	payload, _ := json.Marshal(map[string]string{"version": version, "side": side})
	job, err := h.jobs.Enqueue(r.Context(), jobs.EnqueueInput{
		Type:      "mcsrc.rebuild",
		Payload:   payload,
		CreatedBy: actorName(r),
	})
	if err != nil {
		response.HandleError(w, r, err)
		return
	}
	h.audit(r, "mcsrc.cache_rebuild", version+"-"+side, map[string]any{
		"version": version,
		"side":    side,
		"job_id":  job.ID.String(),
	})
	response.Created(w, map[string]any{
		"job_id":  job.ID.String(),
		"type":    job.Type,
		"version": version,
		"side":    side,
	})
}

// CachePurge POST /api/v1/admin/mcsrc/cache/purge
// body: {version, side?}；删除该版本的 jar / 索引 / 元数据（均为可重建的派生数据）。
// 未缓存该版本返回 404；清理后前端再次访问会重新触发准备。
func (h *McsrcHandler) CachePurge(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req jarRequest
	if !decodeJSON(w, r, &req, 4096) {
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}
	st, err := h.store.Status(req.Version, req.Side)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	if !st.Cached {
		response.NotFound(w, r)
		return
	}
	if err := h.store.Purge(req.Version, req.Side); err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	h.audit(r, "mcsrc.cache_purge", st.Version+"-"+st.Side, map[string]any{
		"version":     st.Version,
		"side":        st.Side,
		"freed_bytes": st.Size,
	})
	response.OK(w, map[string]any{
		"version":     st.Version,
		"side":        st.Side,
		"freed_bytes": st.Size,
	})
}

// CachePreview POST /api/v1/admin/mcsrc/cache/preview
// body: {version, side?}；返回清理该版本的影响预览（预计释放空间、文件清单、
// 是否正在准备），供前端二次确认。不执行任何删除，故不写审计。
func (h *McsrcHandler) CachePreview(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req jarRequest
	if !decodeJSON(w, r, &req, 4096) {
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}
	impact, err := h.store.CacheImpact(req.Version, req.Side)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, impact)
}

// CachePurgeAll POST /api/v1/admin/mcsrc/cache/purge-all
// body: {}；清空全部本地缓存（破坏性运维动作，前端需二次确认）。
func (h *McsrcHandler) CachePurgeAll(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	_, usage, err := h.store.ListCache()
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	purged, err := h.store.PurgeAll()
	if err != nil {
		// 部分成功时也返回已清理数量，避免调用方误以为全部失败。
		h.audit(r, "mcsrc.cache_purge_all_failed", "", map[string]any{
			"purged": purged,
			"err":    err.Error(),
		})
		response.HandleError(w, r, err)
		return
	}
	h.audit(r, "mcsrc.cache_purge_all", "", map[string]any{
		"purged":      purged,
		"freed_bytes": usage.TotalBytes,
	})
	response.OK(w, map[string]any{"purged": purged, "freed_bytes": usage.TotalBytes})
}

// audit 写审计日志（审计服务缺失时静默跳过）。
func (h *McsrcHandler) audit(r *http.Request, action, objID string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	h.auditSvc.Write(r, audit.Entry{
		ActorAdminID: actorID(r),
		Action:       action,
		ObjectType:   "mcsrc_cache",
		ObjectID:     objID,
		After:        after,
	})
}

// Prepare POST /api/v1/mcsrc/prepare
// body: {version, side?}；管理员触发：下载 jar（校验 sha1）并建立类索引。
// 重复调用幂等：已缓存且摘要一致时直接返回。
func (h *McsrcHandler) Prepare(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req jarRequest
	if !decodeJSON(w, r, &req, 4096) {
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}

	side := strings.ToLower(strings.TrimSpace(req.Side))
	if side == "" {
		side = mcsrc.SideClient
	}
	if side != mcsrc.SideClient && side != mcsrc.SideServer {
		response.Validation(w, r, []response.ErrorDetail{{Field: "side", Message: "side 仅支持 client / server"}})
		return
	}

	// 入参校验（版本号白名单）先于额度判定：非法版本号稳定返回 422 而非 429。
	st, err := h.store.Status(req.Version, side)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	if !h.allowPrepare(w, r, st) {
		return
	}

	d, err := h.client.FetchVersion(r.Context(), req.Version)
	if err != nil {
		// 版本不存在与网络故障区分开，方便前端提示。
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "缺少详情地址") {
			response.NotFound(w, r)
			return
		}
		response.Error(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "获取版本详情失败", nil)
		return
	}
	dl, ok := d.Downloads[side]
	if !ok || strings.TrimSpace(dl.URL) == "" {
		writeMcsrcError(w, r, mcsrc.ErrNoDownload)
		return
	}

	st, err = h.store.Prepare(r.Context(), req.Version, side, dl)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, st)
}

// allowPrepare 判定本次 /prepare 是否放行。
//
// 具备 ops.mcsrc 的管理员（显式准备）始终放行；其余访客按「自动准备额度」约束，
// 避免任意访客逐个版本把磁盘与带宽吃满：
//   - 已缓存 jar（仅缺索引）不消耗额度：只是重建索引，不产生新的磁盘占用；
//   - 额度 <= 0 表示关闭自动准备（仅管理员可准备）；
//   - 已缓存条目数达到额度即拒绝（429）。
func (h *McsrcHandler) allowPrepare(w http.ResponseWriter, r *http.Request, st mcsrc.Status) bool {
	if h.hasMcsrcPermission(r) {
		return true
	}
	if st.Cached {
		return true
	}
	if h.autoPrepareMax <= 0 {
		response.Error(w, r, http.StatusTooManyRequests, "AUTO_PREPARE_DISABLED",
			"该版本尚未缓存，服务端已关闭自动准备，请联系管理员准备", nil)
		return false
	}
	list, err := h.store.ListPrepared()
	if err != nil {
		response.HandleError(w, r, err)
		return false
	}
	if len(list) >= h.autoPrepareMax {
		response.Error(w, r, http.StatusTooManyRequests, "AUTO_PREPARE_LIMIT",
			fmt.Sprintf("自动准备额度已用尽（最多自动缓存 %d 个版本），请联系管理员准备该版本", h.autoPrepareMax), nil)
		return false
	}
	return true
}

// hasMcsrcPermission 判断当前会话是否具备 ops.mcsrc。
// 未登录、未配置权限解析器、解析失败一律按「无权限」处理（退化为受额度约束的自动准备）。
func (h *McsrcHandler) hasMcsrcPermission(r *http.Request) bool {
	if h.perms == nil {
		return false
	}
	session := middleware.SessionFrom(r.Context())
	if session == nil {
		return false
	}
	perms, err := h.perms.PermissionsForRole(r.Context(), session.Role)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p == domain.PermOpsMCsrc {
			return true
		}
	}
	return false
}

// Jar POST /api/v1/mcsrc/jar
// body: {version, side?}；以二进制流返回已缓存的 jar（公开只读）。
//
// 项目约定「接口统一 POST + JSON body」，故这里不采用 GET。代价是失去浏览器对
// 响应的 HTTP 缓存与 Range 语义，因此响应头回传官方 sha1（X-Mcsrc-Jar-Sha1），
// 前端据此把几十 MB 的 jar 持久化到 IndexedDB，跨刷新复用
// （见 frontend/src/mcsrc/jarStore.ts），内容变化时自动失效。
func (h *McsrcHandler) Jar(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req struct {
		Version string `json:"version"`
		Side    string `json:"side"`
	}
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}
	f, fi, err := h.store.OpenJar(req.Version, req.Side)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()

	// 摘要供前端做缓存键：服务端重新准备（内容变化）后缓存自动失效。
	if st, serr := h.store.Status(req.Version, req.Side); serr == nil && st.SHA1 != "" {
		w.Header().Set("X-Mcsrc-Jar-Sha1", st.SHA1)
	}
	w.Header().Set("Content-Type", "application/java-archive")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
	// POST 响应本就不会被浏览器缓存，这里再显式声明，避免中间代理误缓存几十 MB。
	w.Header().Set("Cache-Control", "no-store")
	// 响应头已写出后无法再改状态码，写失败只能放弃（客户端多半已断开）。
	_, _ = io.Copy(w, f)
}

// Classes POST /api/v1/mcsrc/classes/list
// body: {version, side?, q?, pkg?, include_inner?, page?, page_size?}
// 在已建索引的版本中检索类（大小写不敏感，相关度优先）。
func (h *McsrcHandler) Classes(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req struct {
		Version      string `json:"version"`
		Side         string `json:"side"`
		Q            string `json:"q"`
		Pkg          string `json:"pkg"`
		IncludeInner bool   `json:"include_inner"`
		Page         int    `json:"page"`
		PageSize     int    `json:"page_size"`
	}
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}

	res, err := h.store.Search(req.Version, req.Side, mcsrc.SearchQuery{
		Q:            req.Q,
		Pkg:          req.Pkg,
		IncludeInner: req.IncludeInner,
		Page:         req.Page,
		PageSize:     req.PageSize,
	})
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, res)
}

// ClassDetail POST /api/v1/mcsrc/classes/detail
// body: {version, side?, class}；返回该类的结构视图（修饰符 / 父类 / 接口 / 字段 / 方法）。
// 只读单个 class 条目，与 jar 体积无关。
func (h *McsrcHandler) ClassDetail(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req classRequest
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if !h.validClassRequest(w, r, req) {
		return
	}
	st, err := h.store.ClassStructure(req.Version, req.Side, req.Class)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, st)
}

// ClassHierarchy POST /api/v1/mcsrc/classes/hierarchy
// body: {version, side?, class}；返回祖先链与直接子类（含实现本接口的类）。
// 首次调用需扫描本 jar 内全部 class 建立关系索引，之后命中内存缓存。
func (h *McsrcHandler) ClassHierarchy(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req classRequest
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if !h.validClassRequest(w, r, req) {
		return
	}
	hier, err := h.store.Hierarchy(r.Context(), req.Version, req.Side, req.Class)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, hier)
}

// ClassBytecode POST /api/v1/mcsrc/classes/bytecode
// body: {version, side?, class}；返回该类的字节码反汇编文本
// （ASM Textifier 风格：类头 + 字段 + 方法 + Code 内的指令 / 标签 / 行号 / 局部变量表 / 异常表）。
// 纯 Go 解析 class 字节码，不反编译、不依赖 JVM，只读单个 class 条目。
func (h *McsrcHandler) ClassBytecode(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req classRequest
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if !h.validClassRequest(w, r, req) {
		return
	}
	res, err := h.store.ClassBytecode(req.Version, req.Side, req.Class)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, res)
}

// ClassReferences POST /api/v1/mcsrc/classes/references
// body: {version, side?, kind?, class, name?, descriptor?}；查找对某个类 / 方法 / 字段的引用。
//
// kind 取 class（默认）/ method / field；method / field 需提供 name 与 descriptor。
// 首次调用需扫描本 jar 内全部 class 的常量池与指令建立反向引用索引，之后命中内存缓存。
// 只返回**目标位于本 jar 内**的引用（jar 外类无法跳转）。
func (h *McsrcHandler) ClassReferences(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req struct {
		Version    string `json:"version"`
		Side       string `json:"side"`
		Kind       string `json:"kind"`
		Class      string `json:"class"`
		Name       string `json:"name"`
		Descriptor string `json:"descriptor"`
	}
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if !h.validClassRequest(w, r, classRequest{Version: req.Version, Side: req.Side, Class: req.Class}) {
		return
	}
	res, err := h.store.References(r.Context(), req.Version, req.Side, req.Kind, req.Class, req.Name, req.Descriptor)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, res)
}

// ClassMapping POST /api/v1/mcsrc/classes/mapping
// body: {version, side?, class}；返回该类在已导入映射数据集中的重命名。
//
// 与其它 mcsrc 接口不同，本接口只读**数据库中的映射数据**（不读 jar、不建索引），
// 因此不受「是否已建类索引」约束；该版本没有导入映射、或没有数据集覆盖该类时
// 返回 `available=false`（正常状态，前端据此隐藏映射名），而非 404。
func (h *McsrcHandler) ClassMapping(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	var req classRequest
	if !decodeJSON(w, r, &req, 8192) {
		return
	}
	if !h.validClassRequest(w, r, req) {
		return
	}
	if h.mappings == nil {
		response.OK(w, catalog.ClassMappingLookup{Available: false, Members: []catalog.MappingMemberView{}})
		return
	}
	res, err := h.mappings.LookupMappingClassByName(r.Context(), req.Version, req.Class)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, res)
}

// classRequest 类详情 / 继承关系请求体。
type classRequest struct {
	Version string `json:"version"`
	Side    string `json:"side"`
	Class   string `json:"class"`
}

// validClassRequest 校验 version 与 class 必填。
func (h *McsrcHandler) validClassRequest(w http.ResponseWriter, r *http.Request, req classRequest) bool {
	details := make([]response.ErrorDetail, 0, 2)
	if strings.TrimSpace(req.Version) == "" {
		details = append(details, response.ErrorDetail{Field: "version", Message: "version 必填"})
	}
	if strings.TrimSpace(req.Class) == "" {
		details = append(details, response.ErrorDetail{Field: "class", Message: "class 必填"})
	}
	if len(details) > 0 {
		response.Validation(w, r, details)
		return false
	}
	return true
}

// ready 检查处理器是否可用。
func (h *McsrcHandler) ready(w http.ResponseWriter, r *http.Request) bool {
	if h.store == nil || h.client == nil {
		response.Error(w, r, http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE", "源码库未启用", nil)
		return false
	}
	return true
}

// decodeJSON 解析请求体；失败时已写出响应。
// 空 body 视为全默认值（与既有查询接口一致）。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	if r.Body == nil {
		return true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(dst); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, http.ErrBodyNotAllowed) {
			return true
		}
		response.BadRequest(w, r, "请求体格式错误")
		return false
	}
	return true
}

// writeMcsrcError 将源码库服务错误映射为 HTTP 响应。
func writeMcsrcError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, mcsrc.ErrDisabled):
		response.Error(w, r, http.StatusServiceUnavailable, "MCSRC_DISABLED", "源码库未启用", nil)
	case errors.Is(err, mcsrc.ErrBadVersion):
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: err.Error()}})
	case errors.Is(err, mcsrc.ErrBadSide):
		response.Validation(w, r, []response.ErrorDetail{{Field: "side", Message: err.Error()}})
	case errors.Is(err, mcsrc.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, mcsrc.ErrCacheBusy):
		response.Error(w, r, http.StatusConflict, "CACHE_BUSY", err.Error(), nil)
	case errors.Is(err, mcsrc.ErrNotIndexed):
		response.Error(w, r, http.StatusConflict, "NEED_PREPARE",
			"该版本尚未建立类索引，请先由管理员执行「准备该版本」", nil)
	case errors.Is(err, mcsrc.ErrClassNotFound):
		response.Error(w, r, http.StatusNotFound, "CLASS_NOT_FOUND",
			"该类不存在于该版本的 jar 中", nil)
	case errors.Is(err, mcsrc.ErrBadClass):
		response.Validation(w, r, []response.ErrorDetail{{Field: "class", Message: err.Error()}})
	case errors.Is(err, mcsrc.ErrBadReference):
		response.Validation(w, r, []response.ErrorDetail{{Field: "kind", Message: err.Error()}})
	case errors.Is(err, mcsrc.ErrClassTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, "CLASS_TOO_LARGE", err.Error(), nil)
	case errors.Is(err, mcsrc.ErrNoDownload):
		response.Error(w, r, http.StatusNotFound, "NO_DOWNLOAD", err.Error(), nil)
	case errors.Is(err, mcsrc.ErrTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, "JAR_TOO_LARGE", err.Error(), nil)
	case errors.Is(err, mcsrc.ErrChecksum):
		response.Error(w, r, http.StatusBadGateway, "JAR_CHECKSUM_MISMATCH", err.Error(), nil)
	default:
		response.HandleError(w, r, err)
	}
}
