package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/auth"
	"mckit/backend/internal/domain"
	"mckit/backend/internal/http/middleware"
	"mckit/backend/internal/mcsrc"
	"mckit/backend/internal/mcversion"
	"mckit/backend/internal/service/catalog"
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
//
// 上游清单提供两个版本（1.21.4 / 1.21.3），且**只有 client 侧**下载信息
// （用于覆盖「该版本缺少 server 下载」的错误分支）。
// 处理器默认开启自动准备（额度 12，无权限解析器），即按匿名访客语义测试。
func newMcsrcTestHandler(t *testing.T) *McsrcHandler {
	t.Helper()
	jar := testJar(t)
	sum := sha1.Sum(jar)
	known := map[string]bool{"1.21.4": true, "1.21.3": true}

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"latest": map[string]string{"release": "1.21.4", "snapshot": "24w45a"},
			"versions": []map[string]any{
				{"id": "1.21.4", "type": "release", "url": srv.URL + "/versions/1.21.4.json"},
				{"id": "1.21.3", "type": "release", "url": srv.URL + "/versions/1.21.3.json"},
			},
		})
	})
	mux.HandleFunc("/versions/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/versions/"), ".json")
		if !known[id] {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   id,
			"type": "release",
			"downloads": map[string]any{
				"client": map[string]any{
					"url":  srv.URL + "/" + id + "-client.jar",
					"sha1": hex.EncodeToString(sum[:]),
					"size": len(jar),
				},
			},
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "-client.jar") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(jar)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := mcversion.New(t.TempDir(), time.Hour).WithHTTP(srv.Client()).WithManifestURL(srv.URL + "/manifest.json")
	store := mcsrc.NewStore(t.TempDir(), 1<<20, 100).WithHTTP(srv.Client())
	return NewMcsrcHandler(store, client).WithAutoPrepare(nil, 12)
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

// TestMcsrcClassBytecodeAndReferences 覆盖字节码反汇编与查找引用两个只读接口的
// 路由 / 校验 / 错误映射（具体反汇编与引用语义在 internal/mcsrc 层单测）。
func TestMcsrcClassBytecodeAndReferences(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 版本未准备（jar 未缓存）→ 404。
	rec := doMcsrc(h, h.ClassBytecode, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的字节码期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = doMcsrc(h, h.ClassReferences, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的查找引用期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 缺 class → 422。
	rec = doMcsrc(h, h.ClassBytecode, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("字节码缺 class 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = doMcsrc(h, h.ClassReferences, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("查找引用缺 class 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 准备 jar + 索引。
	if rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4","side":"client"}`); rec.Code != http.StatusOK {
		t.Fatalf("prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 字节码：200 + 元信息。
	rec = doMcsrc(h, h.ClassBytecode, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("字节码期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var bc mcsrc.ClassBytecode
	dataOf(t, rec, &bc)
	if bc.Name != "net.minecraft.server.MinecraftServer" || bc.Kind != "class" || bc.Major != 52 {
		t.Fatalf("字节码元信息异常：%+v", bc)
	}

	// 字节码：类不存在 → 404 CLASS_NOT_FOUND。
	rec = doMcsrc(h, h.ClassBytecode, `{"version":"1.21.4","class":"net.minecraft.Nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("字节码类不存在期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 查找引用：类级 200（测试 jar 无指令引用 → 空列表）。
	rec = doMcsrc(h, h.ClassReferences, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("查找引用期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var ref mcsrc.ReferenceResult
	dataOf(t, rec, &ref)
	if ref.Kind != "class" || ref.Key != "c:net.minecraft.server.MinecraftServer" || len(ref.Sites) != 0 {
		t.Fatalf("类引用结果异常：%+v", ref)
	}

	// 方法引用缺 name → 422（ErrBadReference）。
	rec = doMcsrc(h, h.ClassReferences, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer","kind":"method"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("方法引用缺 name 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 非法 kind → 422。
	rec = doMcsrc(h, h.ClassReferences, `{"version":"1.21.4","class":"net.minecraft.server.MinecraftServer","kind":"bogus"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法 kind 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	// 类不存在 → 404。
	rec = doMcsrc(h, h.ClassReferences, `{"version":"1.21.4","class":"net.minecraft.Nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("查找引用类不存在期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// fakeMappings 返回预置映射结果，用于验证「映射重命名」接口的接线与参数透传。
type fakeMappings struct {
	res                  *catalog.ClassMappingLookup
	err                  error
	gotVersion, gotClass string
}

func (f *fakeMappings) LookupMappingClassByName(_ context.Context, mcVersion, className string) (*catalog.ClassMappingLookup, error) {
	f.gotVersion, f.gotClass = mcVersion, className
	return f.res, f.err
}

// TestMcsrcClassMapping 覆盖「映射重命名」接口：未注入映射数据时返回 available=false
// （正常状态，非错误），注入后透传版本与类名并返回映射名。
func TestMcsrcClassMapping(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 缺 class → 422。
	rec := doMcsrc(h, h.ClassMapping, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 class 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 未注入映射数据源 → 200 + available=false。
	rec = doMcsrc(h, h.ClassMapping, `{"version":"1.21.4","class":"aab"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("未注入映射期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var empty catalog.ClassMappingLookup
	dataOf(t, rec, &empty)
	if empty.Available {
		t.Fatalf("未注入映射应返回 available=false，实际 %+v", empty)
	}

	// 注入映射数据源 → 透传参数并返回映射名（不依赖 jar / 索引）。
	fake := &fakeMappings{res: &catalog.ClassMappingLookup{
		Available:   true,
		SetKey:      "mojang-1.21.4",
		MCVersion:   "1.21.4",
		MappingType: "mojang",
		MatchedBy:   "obf",
		Class: &catalog.MappingClassNames{
			ObfName:   "aab",
			NamedName: "net.minecraft.server.MinecraftServer",
		},
		Members: []catalog.MappingMemberView{
			{Kind: "method", ObfName: "a", NamedName: "tick"},
		},
	}}
	h = h.WithMappings(fake)
	rec = doMcsrc(h, h.ClassMapping, `{"version":"1.21.4","class":"aab"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("映射查询期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var got catalog.ClassMappingLookup
	dataOf(t, rec, &got)
	if !got.Available || got.Class == nil || got.Class.NamedName != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("映射结果异常：%+v", got)
	}
	if len(got.Members) != 1 || got.Members[0].NamedName != "tick" {
		t.Fatalf("映射成员异常：%+v", got.Members)
	}
	if fake.gotVersion != "1.21.4" || fake.gotClass != "aab" {
		t.Fatalf("参数透传异常：version=%q class=%q", fake.gotVersion, fake.gotClass)
	}
}

// errCodeOf 取出错误响应里的 error.code。
func errCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析错误体失败: %v（body=%s）", err, rec.Body.String())
	}
	return body.Error.Code
}

// fakePerms 按角色返回权限点，用于验证管理员豁免自动准备额度。
type fakePerms map[string][]string

func (f fakePerms) PermissionsForRole(_ context.Context, role string) ([]string, error) {
	return f[role], nil
}

// doMcsrcAs 以指定角色发起 POST（模拟 OptionalAuth 已注入会话）。
func doMcsrcAs(t *testing.T, h *McsrcHandler, fn http.HandlerFunc, role, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcsrc/prepare", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithSession(req.Context(), &auth.Session{AccountID: "a1", Role: role}))
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

// TestMcsrcPrepareAutoQuota 覆盖匿名访客的「自动准备额度」：
// 额度内的版本可自动下载，超出后拒绝（429），但已缓存的版本不消耗额度。
func TestMcsrcPrepareAutoQuota(t *testing.T) {
	// 额度 1：只允许自动缓存 1 个版本。
	h := newMcsrcTestHandler(t).WithAutoPrepare(nil, 1)

	rec := doMcsrc(h, h.Prepare, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("额度内 prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 已缓存（重复请求）不消耗额度：额度已满仍放行。
	rec = doMcsrc(h, h.Prepare, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("已缓存版本重复 prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 新版本超出额度 → 429 AUTO_PREPARE_LIMIT。
	rec = doMcsrc(h, h.Prepare, `{"version":"1.21.3"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超额 prepare 期望 429，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if code := errCodeOf(t, rec); code != "AUTO_PREPARE_LIMIT" {
		t.Fatalf("错误码期望 AUTO_PREPARE_LIMIT，实际 %s", code)
	}

	// 额度判定在入参校验之后：非法版本号仍返回 422 而非 429。
	rec = doMcsrc(h, h.Prepare, `{"version":"../etc/passwd"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("非法版本号期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
}

// TestMcsrcPrepareAutoDisabled 覆盖额度 <= 0（关闭自动准备）：匿名访客一律 429。
func TestMcsrcPrepareAutoDisabled(t *testing.T) {
	h := newMcsrcTestHandler(t).WithAutoPrepare(nil, 0)

	rec := doMcsrc(h, h.Prepare, `{"version":"1.21.4"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("关闭自动准备时期望 429，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if code := errCodeOf(t, rec); code != "AUTO_PREPARE_DISABLED" {
		t.Fatalf("错误码期望 AUTO_PREPARE_DISABLED，实际 %s", code)
	}
}

// TestMcsrcPrepareAdminBypassQuota 覆盖管理员豁免：
// 具备 ops.mcsrc 的角色不受额度限制，其他角色仍受限制。
func TestMcsrcPrepareAdminBypassQuota(t *testing.T) {
	perms := fakePerms{
		"admin":  {domain.PermOpsMCsrc},
		"editor": {domain.PermContentResources},
	}

	// 额度为 0（关闭自动准备）时，管理员仍可准备。
	admin := newMcsrcTestHandler(t).WithAutoPrepare(perms, 0)
	rec := doMcsrcAs(t, admin, admin.Prepare, "admin", `{"version":"1.21.4"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员 prepare 期望 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 无 ops.mcsrc 的角色（editor）仍受额度限制。
	editor := newMcsrcTestHandler(t).WithAutoPrepare(perms, 0)
	rec = doMcsrcAs(t, editor, editor.Prepare, "editor", `{"version":"1.21.4"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("editor prepare 期望 429，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	if code := errCodeOf(t, rec); code != "AUTO_PREPARE_DISABLED" {
		t.Fatalf("错误码期望 AUTO_PREPARE_DISABLED，实际 %s", code)
	}
}

// TestMcsrcJarDownload 覆盖 jar 下发（POST + JSON body）。
//
// 项目约定接口统一 POST，因此不再有浏览器 HTTP 缓存 / Range / ETag 语义：
// 前端改为依据响应头 X-Mcsrc-Jar-Sha1 把 jar 持久化到 IndexedDB。
func TestMcsrcJarDownload(t *testing.T) {
	h := newMcsrcTestHandler(t)

	// 未准备 → 404（不触发下载）。
	rec := doMcsrc(h, h.Jar, `{"version":"1.21.4","side":"client"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未准备版本的 jar 期望 404，实际 %d（%s）", rec.Code, rec.Body.String())
	}

	// 缺 version → 422；非法版本号（路径穿越）→ 422。
	rec = doMcsrc(h, h.Jar, `{}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("缺 version 期望 422，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	rec = doMcsrc(h, h.Jar, `{"version":"../etc/passwd"}`)
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

	rec = doMcsrc(h, h.Jar, `{"version":"1.21.4","side":"client"}`)
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
	// POST 下载靠摘要头 + no-store 替代 ETag / Range，供前端 IndexedDB 缓存。
	if got := rec.Header().Get("X-Mcsrc-Jar-Sha1"); got != st.SHA1 {
		t.Fatalf("X-Mcsrc-Jar-Sha1 期望 %q，实际 %q", st.SHA1, got)
	}
	if got := rec.Header().Get("Content-Length"); got != strconv.FormatInt(st.Size, 10) {
		t.Fatalf("Content-Length 期望 %d，实际 %q", st.Size, got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control 期望 no-store，实际 %q", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("POST 下载不应再返回 ETag，实际 %q", got)
	}
}

func TestMcsrcHandlerUnavailable(t *testing.T) {
	h := NewMcsrcHandler(nil, nil)
	for name, fn := range map[string]http.HandlerFunc{
		"status":     h.Status,
		"prepared":   h.Prepared,
		"prepare":    h.Prepare,
		"jar":        h.Jar,
		"classes":    h.Classes,
		"detail":     h.ClassDetail,
		"hierarchy":  h.ClassHierarchy,
		"bytecode":   h.ClassBytecode,
		"references": h.ClassReferences,
		"mapping":    h.ClassMapping,
	} {
		rec := doMcsrc(h, fn, `{"version":"1.21.4","class":"a.A"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s 未配置时应返回 503，实际 %d（%s）", name, rec.Code, rec.Body.String())
		}
	}
}
