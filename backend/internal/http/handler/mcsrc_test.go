package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/mcsrc"
	"mckit/backend/internal/mcversion"
)

// testJar 生成最小 jar（zip）字节流。
// 两个类条目使用**真实可解析**的 class 字节（含 1 字段 1 方法），
// 以便同时覆盖类结构视图与继承关系；清单条目仍为占位内容。
func testJar(t *testing.T) []byte {
	t.Helper()
	entries := []struct {
		name string
		data []byte
	}{
		{"META-INF/MANIFEST.MF", []byte("cafebabe")},
		{"net/minecraft/server/MinecraftServer.class", fakeClassBytes("net.minecraft.server.MinecraftServer", "net.minecraft.world.level.Level")},
		{"net/minecraft/world/level/Level.class", fakeClassBytes("net.minecraft.world.level.Level", "java.lang.Object")},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatalf("写入 zip 条目失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

// fakeClassBytes 手工拼装最小合法 class 文件（结构视图所需字段齐全）：
// public class <name> extends <super> { private int value; public int get(); }
func fakeClassBytes(name, super string) []byte {
	var cp bytes.Buffer
	cpCount := uint16(1)
	utf8 := func(s string) uint16 {
		idx := cpCount
		cpCount++
		cp.WriteByte(1)
		_ = binary.Write(&cp, binary.BigEndian, uint16(len(s)))
		cp.WriteString(s)
		return idx
	}
	class := func(s string) uint16 {
		nameIdx := utf8(strings.ReplaceAll(s, ".", "/"))
		idx := cpCount
		cpCount++
		cp.WriteByte(7)
		_ = binary.Write(&cp, binary.BigEndian, nameIdx)
		return idx
	}
	thisIdx := class(name)
	superIdx := class(super)
	fieldName, fieldDesc := utf8("value"), utf8("I")
	methodName, methodDesc := utf8("get"), utf8("()I")

	var out bytes.Buffer
	write := func(v any) { _ = binary.Write(&out, binary.BigEndian, v) }
	write(uint32(0xCAFEBABE))
	write(uint16(0))  // minor
	write(uint16(52)) // major = Java 8
	write(cpCount)
	out.Write(cp.Bytes())
	write(uint16(0x0001)) // public
	write(thisIdx)
	write(superIdx)
	write(uint16(0)) // interfaces
	write(uint16(1)) // fields
	write(uint16(0x0002))
	write(fieldName)
	write(fieldDesc)
	write(uint16(0)) // 字段属性表
	write(uint16(1)) // methods
	write(uint16(0x0001))
	write(methodName)
	write(methodDesc)
	write(uint16(0)) // 方法属性表
	write(uint16(0)) // 类属性表
	return out.Bytes()
}

// newMcsrcTestHandler 搭建「假 Mojang 上游 + 真实 mcversion/mcsrc 实现」的处理器。
func newMcsrcTestHandler(t *testing.T) *McsrcHandler {
	t.Helper()
	jar := testJar(t)
	sum := sha1.Sum(jar)

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"latest": map[string]string{"release": "1.21.4", "snapshot": "24w45a"},
			"versions": []map[string]any{{
				"id":   "1.21.4",
				"type": "release",
				"url":  srv.URL + "/versions/1.21.4.json",
			}},
		})
	})
	mux.HandleFunc("/versions/1.21.4.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "1.21.4",
			"type": "release",
			"downloads": map[string]any{
				"client": map[string]any{
					"url":  srv.URL + "/1.21.4-client.jar",
					"sha1": hex.EncodeToString(sum[:]),
					"size": len(jar),
				},
			},
		})
	})
	mux.HandleFunc("/1.21.4-client.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jar)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := mcversion.New(t.TempDir(), time.Hour).WithHTTP(srv.Client()).WithManifestURL(srv.URL + "/manifest.json")
	store := mcsrc.NewStore(t.TempDir(), 1<<20, 100).WithHTTP(srv.Client())
	return NewMcsrcHandler(store, client)
}

// doMcsrc 发起 POST 并返回响应。
func doMcsrc(h *McsrcHandler, fn http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcsrc/x", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

// dataOf 解析 {data: ...} 响应体。
func dataOf(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("解析响应失败: %v（body=%s）", err, rec.Body.String())
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		t.Fatalf("解析 data 失败: %v（body=%s）", err, rec.Body.String())
	}
}

func TestMcsrcStatusAndValidation(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 未准备的版本：200 且 cached/indexed 均为 false。
	rec := doMcsrc(h, h.Status, `{"version":"1.21.4","side":"client"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var st mcsrc.Status
	dataOf(t, rec, &st)
	if st.Cached || st.Indexed || st.Version != "1.21.4" || st.Side != "client" {
		t.Fatalf("未准备版本状态异常：%+v", st)
	}

	// 非法版本号（路径穿越）→ 422。
	rec = doMcsrc(h, h.Status, `{"version":"../etc/passwd"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法版本号期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 非法侧别 → 422。
	rec = doMcsrc(h, h.Status, `{"version":"1.21.4","side":"both"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法侧别期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 空 body 按默认值处理（版本为空 → 422）。
	rec = doMcsrc(h, h.Status, ``)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("空版本期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

func TestMcsrcClassesNeedsPrepare(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 尚未建索引 → 409 NEED_PREPARE。
	rec := doMcsrc(h, h.Classes, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("未建索引期望 409，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("解析错误体失败: %v", err)
	}
	if errBody.Error.Code != "NEED_PREPARE" {
		t.Fatalf("错误码期望 NEED_PREPARE，实际 %s", errBody.Error.Code)
	}
}

func TestMcsrcPrepareThenSearch(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 准备：下载 jar + 建索引（幂等）。
	rec := doMcsrc(h, h.Prepare, `{"version":"1.21.4","side":"client"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var st mcsrc.Status
	dataOf(t, rec, &st)
	if !st.Cached || !st.Indexed || st.ClassCount != 2 {
		t.Fatalf("准备结果异常：%+v", st)
	}

	rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("重复 prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 已准备列表。
	rec = doMcsrc(h, h.Prepared, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepared 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var list []mcsrc.Status
	dataOf(t, rec, &list)
	if len(list) != 1 || !list[0].Indexed {
		t.Fatalf("已准备列表异常：%+v", list)
	}

	// 类检索：大小写不敏感。
	rec = doMcsrc(h, h.Classes, `{"version":"1.21.4","q":"minecraftserver","page_size":10}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("classes 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var res mcsrc.SearchResult
	dataOf(t, rec, &res)
	if res.Total != 1 || res.Items[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("类检索结果异常：%+v", res)
	}
	if res.Items[0].Pkg != "net.minecraft.server" || res.Items[0].Simple != "MinecraftServer" {
		t.Fatalf("类字段拆分异常：%+v", res.Items[0])
	}

	// 包过滤 + 分页参数。
	rec = doMcsrc(h, h.Classes, `{"version":"1.21.4","pkg":"net.minecraft","page":1,"page_size":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("classes 包过滤期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	dataOf(t, rec, &res)
	if res.Total != 2 || len(res.Items) != 1 {
		t.Fatalf("包过滤/分页异常：%+v", res)
	}
}

func TestMcsrcPrepareErrors(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 未知版本 → 404（上游清单里没有）。
	rec := doMcsrc(h, h.Prepare, `{"version":"9.9.9"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知版本期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 版本存在但没有 server 侧下载 → 404 NO_DOWNLOAD。
	rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4","side":"server"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("缺少 server 下载期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 缺 version → 422。
	rec = doMcsrc(h, h.Prepare, `{}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 version 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 非法 side → 422。
	rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4","side":"nope"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法 side 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

func TestMcsrcClassDetailAndHierarchy(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 版本未准备（jar 未缓存）→ 404，且不会触发下载。
	rec := doMcsrc(h, h.ClassDetail, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的类详情期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = doMcsrc(h, h.ClassHierarchy, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的继承关系期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 入参校验：缺 class / 缺 version → 422。
	rec = doMcsrc(h, h.ClassDetail, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 class 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = doMcsrc(h, h.ClassHierarchy, `{"class":"a.A"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 version 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 准备 jar + 索引。
	if rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4","side":"client"}`); rec.Code != http.StatusOK {
		t.Fatalf("prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 类结构视图。
	rec = doMcsrc(h, h.ClassDetail, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("类详情期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var st mcsrc.ClassStructure
	dataOf(t, rec, &st)
	if st.Name != "net.minecraft.server.MinecraftServer" || st.Kind != "class" || st.Major != 52 {
		t.Fatalf("类结构异常：%+v", st)
	}
	if st.Super != "net.minecraft.world.level.Level" || !st.InJarSuper {
		t.Fatalf("父类信息异常：%+v", st)
	}
	if len(st.Fields) != 1 || st.Fields[0].Name != "value" || st.Fields[0].Type != "int" {
		t.Fatalf("字段渲染异常：%+v", st.Fields)
	}
	if len(st.Methods) != 1 || st.Methods[0].Name != "get" || st.Methods[0].Type != "int" {
		t.Fatalf("方法渲染异常：%+v", st.Methods)
	}

	// 斜杠写法等价。
	rec = doMcsrc(h, h.ClassDetail, `{"version":"1.21.4","class":"net/minecraft/world/level/Level"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("斜杠写法期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var lvl mcsrc.ClassStructure
	dataOf(t, rec, &lvl)
	if lvl.Name != "net.minecraft.world.level.Level" || lvl.InJarSuper {
		t.Fatalf("Level 结构异常：%+v", lvl)
	}

	// 不存在的类 → 404 CLASS_NOT_FOUND。
	rec = doMcsrc(h, h.ClassDetail, `{"version":"1.21.4","class":"net.minecraft.Nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在类期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("解析错误体失败: %v", err)
	}
	if errBody.Error.Code != "CLASS_NOT_FOUND" {
		t.Fatalf("错误码期望 CLASS_NOT_FOUND，实际 %s", errBody.Error.Code)
	}

	// 类名非法 → 422。
	rec = doMcsrc(h, h.ClassDetail, `{"version":"1.21.4","class":"a..B"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法类名期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 继承关系：祖先链与子类。
	rec = doMcsrc(h, h.ClassHierarchy, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("继承关系期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var hier mcsrc.Hierarchy
	dataOf(t, rec, &hier)
	if len(hier.Ancestors) != 2 || hier.Ancestors[0] != "net.minecraft.world.level.Level" ||
		hier.Ancestors[1] != "java.lang.Object" || hier.AncestorsInJar {
		t.Fatalf("祖先链异常：%+v", hier)
	}

	rec = doMcsrc(h, h.ClassHierarchy, `{"version":"1.21.4","class":"net.minecraft.world.level.Level"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Level 继承关系期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	dataOf(t, rec, &hier)
	if hier.DescendantCount != 1 || len(hier.Descendants) != 1 ||
		hier.Descendants[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("子类异常：%+v", hier)
	}
}

// doMcsrcGet 发起 GET 请求（仅 jar 下载用）。
func doMcsrcGet(h *McsrcHandler, fn http.HandlerFunc, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func TestMcsrcJarDownload(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 未准备 → 404（不触发下载）。
	rec := doMcsrcGet(h, h.Jar, "/api/v1/mcsrc/jar?version=1.21.4&side=client")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的 jar 期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 缺 version → 422；非法版本号（路径穿越）→ 422。
	rec = doMcsrcGet(h, h.Jar, "/api/v1/mcsrc/jar")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 version 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = doMcsrcGet(h, h.Jar, "/api/v1/mcsrc/jar?version=../etc/passwd")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法版本号期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 准备 jar 后下载：内容须与缓存摘要（sha1 / size）一致。
	rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4","side":"client"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var st mcsrc.Status
	dataOf(t, rec, &st)

	rec = doMcsrcGet(h, h.Jar, "/api/v1/mcsrc/jar?version=1.21.4&side=client")
	if rec.Code != http.StatusOK {
		t.Fatalf("jar 下载期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	sum := sha1.Sum(body)
	if int64(len(body)) != st.Size || hex.EncodeToString(sum[:]) != st.SHA1 {
		t.Fatalf("jar 内容与缓存摘要不一致：len=%d sha1=%s（期望 %d / %s）",
			len(body), hex.EncodeToString(sum[:]), st.Size, st.SHA1)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/java-archive" {
		t.Fatalf("Content-Type 期望 application/java-archive，实际 %q", ct)
	}
	if etag := rec.Header().Get("ETag"); etag != `"`+st.SHA1+`"` {
		t.Fatalf("ETag 期望 jar 摘要，实际 %q", etag)
	}

	// Range 请求 → 206 + 指定区间内容（前端按需读取 / 断点续传依赖）。
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcsrc/jar?version=1.21.4&side=client", nil)
	req.Header.Set("Range", "bytes=0-3")
	rr := httptest.NewRecorder()
	h.Jar(rr, req)
	if rr.Code != http.StatusPartialContent {
		t.Fatalf("Range 请求期望 206，实际 %d（%s）", rr.Code, rr.Body.String())
	}
	if !bytes.Equal(rr.Body.Bytes(), body[:4]) {
		t.Fatalf("Range 内容不一致：%v", rr.Body.Bytes())
	}
}

func TestMcsrcHandlerUnavailable(t *testing.T) {
	h := NewMcsrcHandler(nil, nil)
	for name, fn := range map[string]http.HandlerFunc{
		"status":    h.Status,
		"prepared":  h.Prepared,
		"prepare":   h.Prepare,
		"jar":       h.Jar,
		"classes":   h.Classes,
		"detail":    h.ClassDetail,
		"hierarchy": h.ClassHierarchy,
	} {
		rec := doMcsrc(h, fn, `{"version":"1.21.4","class":"a.A"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s 未配置时应返回 503，实际 %d（%s）", name, rec.Code, rec.Body.String())
		}
	}
}
