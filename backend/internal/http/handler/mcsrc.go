// Package handler 提供 HTTP 处理器。
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"mckit/backend/internal/http/response"
	"mckit/backend/internal/mcsrc"
	"mckit/backend/internal/mcversion"
)

// McsrcHandler「MC 源码浏览器」后端接口。
//
// 契约与项目一致：查询接口统一 POST，参数放 JSON body。
// 公开接口只读缓存状态与索引；触发下载 / 建索引的接口为管理员专属
// （单次可达数百 MB，必须限制触发者），并由路由层叠加重接口限流。
type McsrcHandler struct {
	store  *mcsrc.Store
	client *mcversion.Client
}

// NewMcsrcHandler 创建处理器。store 或 client 为 nil 时接口返回 503。
func NewMcsrcHandler(store *mcsrc.Store, client *mcversion.Client) *McsrcHandler {
	return &McsrcHandler{store: store, client: client}
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

	st, err := h.store.Prepare(r.Context(), req.Version, side, dl)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	response.OK(w, st)
}

// Jar GET /api/v1/mcsrc/jar?version=&side=
// 公开只读：以流式响应返回已缓存的 jar（支持 Range 与条件请求，便于浏览器缓存 / 断点续传）。
//
// 这是本模块唯一的 GET 接口：二进制大文件下载必须借助 GET + Range / ETag 语义，
// 无法用项目惯例的「POST + JSON body」表达；其余接口仍保持 POST。
func (h *McsrcHandler) Jar(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w, r) {
		return
	}
	version := r.URL.Query().Get("version")
	side := r.URL.Query().Get("side")
	if strings.TrimSpace(version) == "" {
		response.Validation(w, r, []response.ErrorDetail{{Field: "version", Message: "version 必填"}})
		return
	}
	f, fi, err := h.store.OpenJar(version, side)
	if err != nil {
		writeMcsrcError(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()

	// 以 jar 摘要作为强 ETag：内容未变时浏览器 / 代理可复用缓存（304）。
	if st, serr := h.store.Status(version, side); serr == nil && st.SHA1 != "" {
		w.Header().Set("ETag", `"`+st.SHA1+`"`)
	}
	w.Header().Set("Content-Type", "application/java-archive")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
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
	case errors.Is(err, mcsrc.ErrNotIndexed):
		response.Error(w, r, http.StatusConflict, "NEED_PREPARE",
			"该版本尚未建立类索引，请先由管理员执行「准备该版本」", nil)
	case errors.Is(err, mcsrc.ErrClassNotFound):
		response.Error(w, r, http.StatusNotFound, "CLASS_NOT_FOUND",
			"该类不存在于该版本的 jar 中", nil)
	case errors.Is(err, mcsrc.ErrBadClass):
		response.Validation(w, r, []response.ErrorDetail{{Field: "class", Message: err.Error()}})
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
