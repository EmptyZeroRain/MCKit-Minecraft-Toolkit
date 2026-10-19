package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStatsRecorder 记录被接受的上报事件。
type fakeStatsRecorder struct {
	tools     []string
	resources []string
}

func (f *fakeStatsRecorder) RecordTool(slug, action string) {
	f.tools = append(f.tools, slug+":"+action)
}
func (f *fakeStatsRecorder) RecordResourceView(slug string) { f.resources = append(f.resources, slug) }

// fakeSlugResolver 仅认为指定 slug 存在。
type fakeSlugResolver struct {
	tools     map[string]bool
	resources map[string]bool
	err       error
}

func (f *fakeSlugResolver) ToolExists(_ context.Context, slug string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.tools[slug], nil
}

func (f *fakeSlugResolver) ResourceExists(_ context.Context, slug string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.resources[slug], nil
}

func postStats(t *testing.T, h *StatsHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stats/event", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Event(rec, req)
	return rec
}

func TestStatsEventAcceptsBuiltinAndKnownSlugs(t *testing.T) {
	recorder := &fakeStatsRecorder{}
	resolver := &fakeSlugResolver{
		tools:     map[string]bool{"custom-tool": true},
		resources: map[string]bool{"jei": true},
	}
	h := NewStatsHandler(recorder, resolver)

	cases := []string{
		`{"scope":"tool","id":"region-inspect","action":"visit"}`, // 内置工具，无需查库
		`{"scope":"tool","id":"custom-tool","action":"use"}`,      // 工具表
		`{"scope":"tool","id":"custom-tool","action":"fail"}`,
		`{"scope":"resource","id":"jei","action":"visit"}`,
	}
	for _, body := range cases {
		if rec := postStats(t, h, body); rec.Code != http.StatusNoContent {
			t.Fatalf("%s 期望 204，实际 %d（%s）", body, rec.Code, rec.Body.String())
		}
	}
	if len(recorder.tools) != 3 {
		t.Errorf("工具事件应为 3 条，实际 %v", recorder.tools)
	}
	if len(recorder.resources) != 1 || recorder.resources[0] != "jei" {
		t.Errorf("资源访问应为 1 条，实际 %v", recorder.resources)
	}
}

func TestStatsEventRejectsInvalidInput(t *testing.T) {
	h := NewStatsHandler(&fakeStatsRecorder{}, &fakeSlugResolver{
		tools:     map[string]bool{"known": true},
		resources: map[string]bool{"known-res": true},
	})

	cases := []string{
		`{"scope":"bogus","id":"x","action":"visit"}`,
		`{"scope":"tool","id":"known","action":"bogus"}`,
		`{"scope":"resource","id":"known-res","action":"use"}`,
		`{"scope":"tool","id":"has space","action":"visit"}`,
		`{"scope":"tool","id":"","action":"visit"}`,
		`{"scope":"tool","id":"unknown-tool","action":"visit"}`,
		`{"scope":"resource","id":"unknown-res","action":"visit"}`,
		`not json`,
	}
	for _, body := range cases {
		if rec := postStats(t, h, body); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s 期望 422，实际 %d（%s）", body, rec.Code, rec.Body.String())
		}
	}
}

func TestStatsEventResolverErrorIs500(t *testing.T) {
	h := NewStatsHandler(&fakeStatsRecorder{}, &fakeSlugResolver{err: errors.New("db down")})
	rec := postStats(t, h, `{"scope":"tool","id":"custom","action":"visit"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("查询失败期望 500，实际 %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("错误响应不得泄漏内部细节: %s", rec.Body.String())
	}
}

func TestStatsEventNilRecorderStillValidates(t *testing.T) {
	h := NewStatsHandler(nil, &fakeSlugResolver{tools: map[string]bool{"known": true}})
	if rec := postStats(t, h, `{"scope":"tool","id":"region-inspect","action":"visit"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("统计关闭时合法上报仍应 204，实际 %d", rec.Code)
	}
	if rec := postStats(t, h, `{"scope":"tool","id":"unknown","action":"visit"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法对象仍应 422，实际 %d", rec.Code)
	}
}

func TestResolveAnalyticsRange(t *testing.T) {
	// days 默认 30
	from, to, err := resolveRange(analyticsRangeRequest{})
	if err != nil {
		t.Fatalf("默认范围出错: %v", err)
	}
	if days := int(to.Sub(from).Hours()/24) + 1; days != defaultAnalyticsDays {
		t.Fatalf("默认天数应为 %d，实际 %d", defaultAnalyticsDays, days)
	}
	// 显式 from/to
	from, to, err = resolveRange(analyticsRangeRequest{From: "2026-01-01", To: "2026-01-31"})
	if err != nil {
		t.Fatalf("显式范围出错: %v", err)
	}
	if from.Format("2006-01-02") != "2026-01-01" || to.Format("2006-01-02") != "2026-01-31" {
		t.Fatalf("范围解析错误: %v ~ %v", from, to)
	}
	// 顺序错误
	if _, _, err := resolveRange(analyticsRangeRequest{From: "2026-02-01", To: "2026-01-01"}); err == nil {
		t.Fatal("from 晚于 to 应报错")
	}
	// 非法日期
	if _, _, err := resolveRange(analyticsRangeRequest{From: "2026-13-01"}); err == nil {
		t.Fatal("非法日期应报错")
	}
	// 超长跨度被收敛到上限
	from, to, err = resolveRange(analyticsRangeRequest{From: "2000-01-01", To: "2026-01-01"})
	if err != nil {
		t.Fatalf("超长范围出错: %v", err)
	}
	if days := int(to.Sub(from).Hours()/24) + 1; days != maxAnalyticsDays {
		t.Fatalf("超长跨度应收敛为 %d 天，实际 %d", maxAnalyticsDays, days)
	}
}
