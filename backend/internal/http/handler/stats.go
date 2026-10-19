package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"mckit/backend/internal/builtintools"
	"mckit/backend/internal/http/response"
	"mckit/backend/internal/service/metrics"
)

// StatsRecorder 使用统计采集能力（便于测试替换）。
type StatsRecorder interface {
	RecordTool(slug, action string)
	RecordResourceView(slug string)
}

// StatsSlugResolver 校验统计对象是否存在，避免伪造 slug 污染统计。
type StatsSlugResolver interface {
	ToolExists(ctx context.Context, slug string) (bool, error)
	ResourceExists(ctx context.Context, slug string) (bool, error)
}

// StatsHandler 公开的匿名使用统计上报处理器（TODO §13）。
//
// 隐私设计：只接收「工具/资源 slug + 动作」，不接收也不落库 IP、会话、用户标识、
// User-Agent 或自由文本；对象必须通过白名单校验（内置工具 / 工具表 / 资源表），
// 避免任意字符串污染统计。上报为 best-effort：失败不影响相关工具的正常使用。
type StatsHandler struct {
	recorder StatsRecorder
	resolver StatsSlugResolver
}

// NewStatsHandler 创建处理器。recorder 为 nil 时接口静默成功（统计关闭）。
func NewStatsHandler(recorder StatsRecorder, resolver StatsSlugResolver) *StatsHandler {
	return &StatsHandler{recorder: recorder, resolver: resolver}
}

// statsIDPattern 统计对象标识的合法字符与长度（slug / item_id / 工具 slug）。
var statsIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:+-]{1,180}$`)

// Event POST /api/v1/stats/event
// 请求体：{"scope":"tool|resource","id":"<slug>","action":"visit|use|fail"}
// 成功返回 204；非法或未知对象返回 422。
func (h *StatsHandler) Event(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope  string `json:"scope"`
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req)
	}
	scope := strings.TrimSpace(req.Scope)
	id := strings.TrimSpace(req.ID)
	action := strings.TrimSpace(req.Action)

	if !statsIDPattern.MatchString(id) {
		response.Validation(w, r, []response.ErrorDetail{{Field: "id", Message: "统计对象标识不合法"}})
		return
	}

	switch scope {
	case "tool":
		if action != metrics.ActionVisit && action != metrics.ActionUse && action != metrics.ActionFail {
			response.Validation(w, r, []response.ErrorDetail{{Field: "action", Message: "工具动作只允许 visit / use / fail"}})
			return
		}
		ok, err := h.toolAllowed(r.Context(), id)
		if err != nil {
			response.HandleError(w, r, err)
			return
		}
		if !ok {
			response.Error(w, r, http.StatusUnprocessableEntity, "STATS_TARGET_NOT_FOUND", "统计对象不存在", nil)
			return
		}
		if h.recorder != nil {
			h.recorder.RecordTool(id, action)
		}
	case "resource":
		if action != metrics.ActionVisit {
			response.Validation(w, r, []response.ErrorDetail{{Field: "action", Message: "资源动作只允许 visit"}})
			return
		}
		ok := false
		if h.resolver != nil {
			var err error
			ok, err = h.resolver.ResourceExists(r.Context(), id)
			if err != nil {
				response.HandleError(w, r, err)
				return
			}
		}
		if !ok {
			response.Error(w, r, http.StatusUnprocessableEntity, "STATS_TARGET_NOT_FOUND", "统计对象不存在", nil)
			return
		}
		if h.recorder != nil {
			h.recorder.RecordResourceView(id)
		}
	default:
		response.Validation(w, r, []response.ErrorDetail{{Field: "scope", Message: "scope 只允许 tool / resource"}})
		return
	}
	response.NoContent(w)
}

// toolAllowed 内置工具直接放行，其余查工具表。
func (h *StatsHandler) toolAllowed(ctx context.Context, slug string) (bool, error) {
	if builtintools.IsBuiltin(slug) {
		return true, nil
	}
	if h.resolver == nil {
		return false, nil
	}
	return h.resolver.ToolExists(ctx, slug)
}
