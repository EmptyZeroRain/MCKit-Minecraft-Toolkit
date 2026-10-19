package mcsrc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mckit/backend/internal/mcversion"
)

// makeJar 生成一个最小可用的 jar（zip）字节流，含类、内部类、清单与模块描述符。
func makeJar(t *testing.T, classNames ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("创建 zip 条目失败: %v", err)
		}
		if _, err := w.Write([]byte("cafebabe")); err != nil {
			t.Fatalf("写入 zip 条目失败: %v", err)
		}
	}
	write("META-INF/MANIFEST.MF")
	write("module-info.class")
	for _, n := range classNames {
		write(n)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

func sha1Hex(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

func TestBuildIndexFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	jar := makeJar(t,
		"net/minecraft/world/level/Level.class",
		"net/minecraft/server/MinecraftServer.class",
		"net/minecraft/server/MinecraftServer$1.class",
	)
	path := filepath.Join(dir, "t.jar")
	if err := os.WriteFile(path, jar, 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(context.Background(), path, "1.21.4", SideClient, sha1Hex(jar), 100)
	if err != nil {
		t.Fatalf("BuildIndex 失败: %v", err)
	}
	if idx.Truncated {
		t.Fatal("不应被截断")
	}
	if len(idx.Classes) != 3 {
		t.Fatalf("期望 3 个类，实际 %d：%+v", len(idx.Classes), idx.Classes)
	}
	// 按全限定名升序：net.minecraft.server.* 在 net.minecraft.world.* 之前。
	if idx.Classes[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("排序异常：%+v", idx.Classes)
	}
	inner := idx.Classes[1]
	if !inner.Inner || inner.Simple != "MinecraftServer$1" {
		t.Fatalf("内部类标记异常：%+v", inner)
	}
	if idx.Classes[2].Pkg != "net.minecraft.world.level" || idx.Classes[2].Simple != "Level" {
		t.Fatalf("包名/简单名拆分异常：%+v", idx.Classes[2])
	}
}

func TestIndexSearchRankingAndPaging(t *testing.T) {
	idx := &Index{
		Classes: []ClassEntry{
			{Name: "net.minecraft.world.level.Level", Simple: "Level", Pkg: "net.minecraft.world.level"},
			{Name: "net.minecraft.server.MinecraftServer", Simple: "MinecraftServer", Pkg: "net.minecraft.server"},
			{Name: "net.minecraft.server.MinecraftServer$1", Simple: "MinecraftServer$1", Pkg: "net.minecraft.server", Inner: true},
			{Name: "net.minecraft.server.dedicated.DedicatedServer", Simple: "DedicatedServer", Pkg: "net.minecraft.server.dedicated"},
		},
	}

	// 精确匹配排最前；默认排除内部类。
	res := idx.Search(SearchQuery{Q: "MinecraftServer"})
	if res.Total != 1 {
		t.Fatalf("默认应排除内部类，实际 total=%d items=%+v", res.Total, res.Items)
	}
	if res.Items[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("精确匹配应排首位：%+v", res.Items)
	}

	// 包含内部类时总数 +1。
	res = idx.Search(SearchQuery{Q: "MinecraftServer", IncludeInner: true})
	if res.Total != 2 {
		t.Fatalf("包含内部类时期望 2，实际 %d", res.Total)
	}
	if res.Items[0].Inner || res.Items[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("包含内部类时精确匹配仍应排首位：%+v", res.Items)
	}

	// 简单名包含匹配（默认排除内部类，故为 2 个）。
	res = idx.Search(SearchQuery{Q: "server"})
	if res.Total != 2 {
		t.Fatalf("server 应匹配 2 个类，实际 %d：%+v", res.Total, res.Items)
	}

	// 包名前缀过滤（同样默认排除内部类）。
	res = idx.Search(SearchQuery{Pkg: "net.minecraft.server", PageSize: 2})
	if res.Total != 2 {
		t.Fatalf("包过滤期望 2，实际 %d", res.Total)
	}
	if len(res.Items) != 2 || res.PageSize != 2 {
		t.Fatalf("分页异常：%+v", res)
	}

	// 空关键词返回全部（按全限定名升序，默认排除内部类）。
	res = idx.Search(SearchQuery{PageSize: 200})
	if res.Total != 3 || res.Items[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("空关键词应返回全部且有序：%+v", res)
	}

	// 非法页码/页大小回落默认值。
	res = idx.Search(SearchQuery{Page: -3, PageSize: 100000})
	if res.Page != 1 || res.PageSize != defaultPageSize {
		t.Fatalf("分页参数未回落默认值：%+v", res)
	}
}

func TestNormalizeRejectsTraversal(t *testing.T) {
	bad := []string{"", "../etc/passwd", "1.21/../../x", "a b", strings.Repeat("x", 65)}
	for _, v := range bad {
		if _, _, err := normalize(v, SideClient); !errors.Is(err, ErrBadVersion) {
			t.Fatalf("版本 %q 应被拒绝，实际 err=%v", v, err)
		}
	}
	if _, _, err := normalize("1.21.4", "unknown"); !errors.Is(err, ErrBadSide) {
		t.Fatalf("非法侧别应被拒绝，实际 err=%v", err)
	}
	if v, sd, err := normalize(" 1.21.4 ", "SERVER"); err != nil || v != "1.21.4" || sd != SideServer {
		t.Fatalf("归一化异常：%q %q %v", v, sd, err)
	}
}

// newTestStore 返回使用指定 jar 字节与下载信息的 Store 及其 httptest 服务器。
func newTestStore(t *testing.T, jar []byte, opts ...func(*Store)) (*Store, mcversion.DownloadInfo, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(jar)
	}))
	t.Cleanup(srv.Close)

	store := NewStore(t.TempDir(), 1<<20, 100).WithHTTP(srv.Client())
	for _, opt := range opts {
		opt(store)
	}
	dl := mcversion.DownloadInfo{URL: srv.URL + "/client.jar", SHA1: sha1Hex(jar), Size: int64(len(jar))}
	return store, dl, &hits
}

func TestPrepareDownloadsIndexesAndCaches(t *testing.T) {
	jar := makeJar(t, "net/minecraft/server/MinecraftServer.class")
	store, dl, hits := newTestStore(t, jar)

	st, err := store.Prepare(context.Background(), "1.21.4", SideClient, dl)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if !st.Cached || !st.Indexed || st.ClassCount != 1 {
		t.Fatalf("准备状态异常：%+v", st)
	}
	if st.SHA1 != dl.SHA1 || st.Size != dl.Size {
		t.Fatalf("摘要/体积不一致：%+v", st)
	}

	// 再次 Prepare 应命中缓存，不再请求上游。
	if _, err := store.Prepare(context.Background(), "1.21.4", SideClient, dl); err != nil {
		t.Fatalf("二次 Prepare 失败: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("二次 Prepare 不应重新下载，实际请求 %d 次", hits.Load())
	}

	// 索引可读且可检索。
	res, err := store.Search("1.21.4", SideClient, SearchQuery{Q: "minecraftserver"})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if res.Total != 1 || res.Items[0].Name != "net.minecraft.server.MinecraftServer" {
		t.Fatalf("检索结果异常：%+v", res)
	}

	// 已准备列表。
	list, err := store.ListPrepared()
	if err != nil {
		t.Fatalf("ListPrepared 失败: %v", err)
	}
	if len(list) != 1 || list[0].Version != "1.21.4" {
		t.Fatalf("已准备列表异常：%+v", list)
	}

	// 未准备的版本：状态为未缓存，检索报未建索引。
	other, err := store.Status("1.20.1", SideClient)
	if err != nil || other.Cached || other.Indexed {
		t.Fatalf("未准备版本状态异常：%+v err=%v", other, err)
	}
	if _, err := store.Search("1.20.1", SideClient, SearchQuery{}); !errors.Is(err, ErrNotIndexed) {
		t.Fatalf("未建索引应返回 ErrNotIndexed，实际 %v", err)
	}
}

// TestProgressWriterAccumulates 覆盖进度累加器：只对已跟踪的目标计数，
// 避免为未在准备的版本凭空造出进度条目。
func TestProgressWriterAccumulates(t *testing.T) {
	store := NewStore(t.TempDir(), 1<<20, 100)
	key := progKey("1.21.4", SideClient)
	store.setProgress(key, progress{phase: PhaseDownloading, total: 10})

	w := progressWriter{store: store, key: key}
	if n, err := w.Write([]byte("12345")); n != 5 || err != nil {
		t.Fatalf("Write 应原样返回长度，实际 n=%d err=%v", n, err)
	}
	if _, err := w.Write([]byte("678")); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	if p, _ := store.progressOf(key); p.downloaded != 8 {
		t.Fatalf("已下载字节数期望 8，实际 %d", p.downloaded)
	}

	// 未跟踪的目标不产生进度条目。
	other := progressWriter{store: store, key: progKey("1.20.1", SideClient)}
	if _, err := other.Write([]byte("x")); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	if _, ok := store.progressOf(progKey("1.20.1", SideClient)); ok {
		t.Fatal("未跟踪的目标不应产生进度条目")
	}

	// 清除后不再上报。
	store.clearProgress(key)
	if _, ok := store.progressOf(key); ok {
		t.Fatal("clearProgress 后不应再读到进度")
	}
}

// TestPrepareReportsProgress 覆盖准备进度上报：下载期间 Status 暴露
// preparing / phase / downloaded / total（前端进度条的数据来源），
// 准备结束后进度被清除。
func TestPrepareReportsProgress(t *testing.T) {
	jar := makeJar(t, "net/minecraft/server/MinecraftServer.class")
	// 分两段下发并插入延迟，制造可观测的下载窗口。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		half := len(jar) / 2
		_, _ = w.Write(jar[:half])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write(jar[half:])
	}))
	defer srv.Close()

	store := NewStore(t.TempDir(), 1<<20, 100).WithHTTP(srv.Client())
	dl := mcversion.DownloadInfo{URL: srv.URL + "/client.jar", SHA1: sha1Hex(jar), Size: int64(len(jar))}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := store.Prepare(context.Background(), "1.21.4", SideClient, dl); err != nil {
			t.Errorf("Prepare 失败: %v", err)
		}
	}()

	// 轮询直到观察到已下载字节数（最多 2 秒，避免偶发调度延迟导致假失败）。
	seen := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !seen {
		st, err := store.Status("1.21.4", SideClient)
		if err != nil {
			t.Fatalf("Status 失败: %v", err)
		}
		if st.Preparing {
			if st.Phase != PhaseDownloading && st.Phase != PhaseIndexing {
				t.Fatalf("未知准备阶段：%q", st.Phase)
			}
			if st.Total != int64(len(jar)) {
				t.Fatalf("进度总量期望 %d，实际 %d", len(jar), st.Total)
			}
			if st.Downloaded > 0 {
				seen = true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	<-done
	if !seen {
		t.Fatal("下载期间未观察到进度上报")
	}

	// 结束后不再上报进度，且已完成建索引。
	st, err := store.Status("1.21.4", SideClient)
	if err != nil {
		t.Fatalf("Status 失败: %v", err)
	}
	if st.Preparing || st.Phase != "" || st.Downloaded != 0 || st.Total != 0 {
		t.Fatalf("准备结束后应清除进度：%+v", st)
	}
	if !st.Indexed {
		t.Fatalf("准备应完成建索引：%+v", st)
	}
}

func TestPrepareIsSingleFlight(t *testing.T) {
	jar := makeJar(t, "net/minecraft/server/MinecraftServer.class")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond) // 拉长窗口，制造并发重叠
		_, _ = w.Write(jar)
	}))
	defer srv.Close()

	store := NewStore(t.TempDir(), 1<<20, 100).WithHTTP(srv.Client())
	dl := mcversion.DownloadInfo{URL: srv.URL + "/client.jar", SHA1: sha1Hex(jar), Size: int64(len(jar))}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Prepare(context.Background(), "1.21.4", SideClient, dl); err != nil {
				t.Errorf("并发 Prepare 失败: %v", err)
			}
		}()
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("并发 Prepare 应只下载一次，实际 %d 次", hits.Load())
	}
}

func TestPrepareChecksumAndSizeLimits(t *testing.T) {
	jar := makeJar(t, "net/minecraft/server/MinecraftServer.class")

	// sha1 不匹配。
	store, dl, _ := newTestStore(t, jar)
	bad := dl
	bad.SHA1 = strings.Repeat("0", 40)
	if _, err := store.Prepare(context.Background(), "1.21.4", SideClient, bad); !errors.Is(err, ErrChecksum) {
		t.Fatalf("摘要不符应返回 ErrChecksum，实际 %v", err)
	}
	// 校验失败不应留下半成品。
	if st, _ := store.Status("1.21.4", SideClient); st.Cached {
		t.Fatalf("校验失败后不应标记为已缓存：%+v", st)
	}

	// 声明体积超限（在下载前即拒绝）。
	store2, dl2, _ := newTestStore(t, jar)
	dl2.Size = 1 << 40
	if _, err := store2.Prepare(context.Background(), "1.21.4", SideClient, dl2); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("体积超限应返回 ErrTooLarge，实际 %v", err)
	}

	// 实际体积超限（上游比声明的大）。
	store3 := NewStore(t.TempDir(), 4, 100)
	dl3 := mcversion.DownloadInfo{URL: dl.URL, Size: 0}
	store3.WithHTTP(func() *http.Client {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(jar)
		}))
		t.Cleanup(srv.Close)
		return srv.Client()
	}())
	if _, err := store3.Prepare(context.Background(), "1.21.4", SideClient, dl3); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("实际体积超限应返回 ErrTooLarge，实际 %v", err)
	}

	// 缺少下载地址。
	store4, _, _ := newTestStore(t, jar)
	if _, err := store4.Prepare(context.Background(), "1.21.4", SideClient, mcversion.DownloadInfo{}); !errors.Is(err, ErrNoDownload) {
		t.Fatalf("缺少下载地址应返回 ErrNoDownload，实际 %v", err)
	}
}

func TestPrepareUpstreamErrorAndContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	store := NewStore(t.TempDir(), 1<<20, 100).WithHTTP(srv.Client())
	dl := mcversion.DownloadInfo{URL: srv.URL + "/client.jar"}
	if _, err := store.Prepare(context.Background(), "1.21.4", SideClient, dl); err == nil {
		t.Fatal("上游 403 应返回错误")
	}

	// 上下文取消应中止下载。
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()
	store2 := NewStore(t.TempDir(), 1<<20, 100).WithHTTP(slow.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := store2.Prepare(ctx, "1.21.4", SideClient, mcversion.DownloadInfo{URL: slow.URL + "/c.jar"}); err == nil {
		t.Fatal("上下文超时应返回错误")
	}
}

func TestStatusJSONOmitsZeroTimes(t *testing.T) {
	store, dl, _ := newTestStore(t, makeJar(t, "net/minecraft/server/MinecraftServer.class"))

	// 未缓存时不应出现零值时间（前端会渲染成 0001/1/1）。
	pending, err := store.Status("1.21.4", SideClient)
	if err != nil {
		t.Fatalf("Status 失败: %v", err)
	}
	raw, err := json.Marshal(pending)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if strings.Contains(string(raw), "0001-01-01") {
		t.Fatalf("未缓存状态不应含零值时间：%s", raw)
	}
	if strings.Contains(string(raw), "fetched_at") || strings.Contains(string(raw), "index_built_at") {
		t.Fatalf("未缓存状态不应含时间字段：%s", raw)
	}

	// 准备完成后两个时间字段都应存在。
	ready, err := store.Prepare(context.Background(), "1.21.4", SideClient, dl)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if ready.FetchedAt == nil || ready.IndexBuiltAt == nil {
		t.Fatalf("已准备状态应含时间字段：%+v", ready)
	}
	raw, _ = json.Marshal(ready)
	if !strings.Contains(string(raw), "fetched_at") || !strings.Contains(string(raw), "index_built_at") {
		t.Fatalf("已准备状态应含时间字段：%s", raw)
	}
}

func TestBuildIndexTruncatesAtLimit(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		names = append(names, fmt.Sprintf("net/minecraft/gen/C%d.class", i))
	}
	jar := makeJar(t, names...)
	path := filepath.Join(dir, "t.jar")
	if err := os.WriteFile(path, jar, 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path, "1.21.4", SideServer, "", 4)
	if err != nil {
		t.Fatalf("BuildIndex 失败: %v", err)
	}
	if !idx.Truncated || len(idx.Classes) != 4 {
		t.Fatalf("截断异常：truncated=%v len=%d", idx.Truncated, len(idx.Classes))
	}
}
