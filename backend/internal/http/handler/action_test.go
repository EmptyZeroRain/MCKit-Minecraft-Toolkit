package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestWithBodyIDInjectsParam 验证动作子路径契约：id 从 body 注入 chi URL 参数。
func TestWithBodyIDInjectsParam(t *testing.T) {
	r := chi.NewRouter()
	var gotID string
	var decoded string
	r.Post("/x/update", WithBodyID(func(w http.ResponseWriter, req *http.Request) {
		gotID = chi.URLParam(req, "id")
		// body 注入后仍可被后续处理器重新解码。
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		decoded = body.Name
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/x/update", strings.NewReader(`{"id":"abc-123","name":"工具"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("期望 204，实际 %d", rec.Code)
	}
	if gotID != "abc-123" {
		t.Fatalf("URL 参数 id 应为 abc-123，实际 %q", gotID)
	}
	if decoded != "工具" {
		t.Fatalf("body 应可再次解码，name=%q", decoded)
	}
}

// TestWithBodyIDNoID body 无 id 时不注入，处理器仍正常执行。
func TestWithBodyIDNoID(t *testing.T) {
	r := chi.NewRouter()
	var gotID string
	r.Post("/x/list", WithBodyID(func(w http.ResponseWriter, req *http.Request) {
		gotID = chi.URLParam(req, "id")
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/x/list", strings.NewReader(`{"page":1}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	if gotID != "" {
		t.Fatalf("无 id 时不应注入，实际 %q", gotID)
	}
}

func TestDecodeID(t *testing.T) {
	// 正常
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"id":"z"}`))
	rec := httptest.NewRecorder()
	id, ok := decodeID(rec, req)
	if !ok || id != "z" {
		t.Fatalf("应解析出 id=z: ok=%v id=%q", ok, id)
	}
	// 缺失
	req2 := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	rec2 := httptest.NewRecorder()
	if _, ok := decodeID(rec2, req2); ok {
		t.Fatal("缺少 id 应返回失败")
	}
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺少 id 应返回 422，实际 %d", rec2.Code)
	}
}
