package mcsrc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mckit/backend/internal/mcversion"
)

// TestLivePrepareRealJar 用真实上游（Mojang 清单 + 官方 jar）验证「下载 → 校验 sha1 → 建索引」全链路。
//
// 默认跳过（需要外网且会下载约 10 MB），仅在显式开启时运行：
//
//	MCSRC_LIVE_TEST=1 go test ./internal/mcsrc/ -run TestLivePrepareRealJar -v
//
// 该用例的价值：单元测试用的是合成 jar 与假上游，无法证明
// 「mcversion 解析出的 DownloadInfo 字段与真实清单一致」以及「真实 jar 的条目布局可被索引」。
func TestLivePrepareRealJar(t *testing.T) {
	if os.Getenv("MCSRC_LIVE_TEST") != "1" {
		t.Skip("未设置 MCSRC_LIVE_TEST=1，跳过真实上游验证")
	}

	// 选体积较小、类数量适中且长期存在的正式版，避免动辄下载数百 MB。
	const version = "1.8.9"

	dir := t.TempDir()
	client := mcversion.New(filepath.Join(dir, "meta"), time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	detail, err := client.FetchVersion(ctx, version)
	if err != nil {
		t.Fatalf("获取版本详情失败: %v", err)
	}
	dl, ok := detail.Downloads[SideClient]
	if !ok || strings.TrimSpace(dl.URL) == "" || dl.SHA1 == "" {
		t.Fatalf("版本 %s 缺少客户端下载信息：%+v", version, detail.Downloads)
	}
	t.Logf("官方客户端 jar：size=%d sha1=%s", dl.Size, dl.SHA1)

	store := NewStore(filepath.Join(dir, "mcsrc"), 512<<20, 60000)
	st, err := store.Prepare(ctx, version, SideClient, dl)
	if err != nil {
		t.Fatalf("准备失败（sha1 校验不符会在此暴露）: %v", err)
	}
	if !st.Cached || !st.Indexed {
		t.Fatalf("准备结果异常：%+v", st)
	}
	// 真实客户端 jar 的类数量远大于 1000；数量过小说明索引解析有问题。
	if st.ClassCount < 1000 {
		t.Fatalf("类数量异常偏小：%d", st.ClassCount)
	}
	if st.Truncated {
		t.Fatalf("不应触发截断：%+v", st)
	}

	// 真实 jar 中必然存在这两个类，且包名拆分正确。
	res, err := store.Search(version, SideClient, SearchQuery{Q: "MinecraftServer"})
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	var found bool
	for _, c := range res.Items {
		if c.Name == "net.minecraft.server.MinecraftServer" {
			found = true
			if c.Pkg != "net.minecraft.server" || c.Simple != "MinecraftServer" || c.Inner {
				t.Fatalf("类字段拆分异常：%+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("未检索到 net.minecraft.server.MinecraftServer：%+v", res.Items)
	}

	// 幂等：重复准备不应重新下载（缓存命中后摘要与体积一致）。
	st2, err := store.Prepare(ctx, version, SideClient, dl)
	if err != nil {
		t.Fatalf("二次准备失败: %v", err)
	}
	if st2.SHA1 != st.SHA1 || st2.Size != st.Size || st2.ClassCount != st.ClassCount {
		t.Fatalf("二次准备结果不一致：%+v vs %+v", st, st2)
	}

	list, err := store.ListPrepared()
	if err != nil || len(list) != 1 {
		t.Fatalf("已准备列表异常：%+v err=%v", list, err)
	}
	t.Logf("真实链路验证通过：%d 个类，jar %d 字节", st.ClassCount, st.Size)

	// ---- 类结构视图（真实 class 字节）----
	// 断言不写死具体成员，只验证「结构自洽」：
	// 本类名一致、父类非空、字段 / 方法描述符可被渲染、修饰符非空。
	detailOfServer, err := store.ClassStructure(version, SideClient, "net.minecraft.server.MinecraftServer")
	if err != nil {
		t.Fatalf("真实 jar 类结构解析失败: %v", err)
	}
	if detailOfServer.Name != "net.minecraft.server.MinecraftServer" || detailOfServer.Kind != "class" {
		t.Fatalf("类结构基本信息异常：%+v", detailOfServer)
	}
	if detailOfServer.Super == "" || detailOfServer.Super == detailOfServer.Name {
		t.Fatalf("父类解析异常：%+v", detailOfServer)
	}
	if detailOfServer.Major < 45 || detailOfServer.Major > 70 {
		t.Fatalf("class 文件版本异常：%d", detailOfServer.Major)
	}
	if len(detailOfServer.Methods) < 10 {
		t.Fatalf("方法数量异常偏少：%d", len(detailOfServer.Methods))
	}
	var typed, voided bool
	for _, m := range detailOfServer.Methods {
		if m.Type == "" || m.Descriptor == "" || len(m.Modifiers) == 0 {
			t.Fatalf("方法渲染缺失信息：%+v", m)
		}
		if m.Type == "void" {
			voided = true
		}
		if len(m.Params) > 0 {
			typed = true
		}
	}
	if !voided || !typed {
		t.Fatalf("应同时存在无参 void 方法与带参方法：voided=%v typed=%v", voided, typed)
	}
	t.Logf("MinecraftServer：super=%s 字段=%d 方法=%d java=%d",
		detailOfServer.Super, len(detailOfServer.Fields), len(detailOfServer.Methods), detailOfServer.Major)

	// ---- 继承关系（真实 jar 全量扫描）----
	// 真实版本里类与父类的分布随版本变化（例如 1.8.9 的 MinecraftServer 直接继承 java.lang.Object），
	// 因此不写死类名：先在本 jar 中找出一个「父类也在 jar 内」的类，再验证双向关系。
	rels, err := store.relationsFor(ctx, version, SideClient, st.SHA1)
	if err != nil {
		t.Fatalf("构建真实 jar 关系索引失败: %v", err)
	}
	var child, parent string
	for name, sup := range rels.sup {
		if rels.inner[name] || sup == "" || !rels.known(sup) {
			continue
		}
		child, parent = name, sup
		break
	}
	if child == "" {
		t.Fatalf("未能在真实 jar（%d 个类）中找到父类位于 jar 内的类", len(rels.sup))
	}
	t.Logf("选中继承样本：%s extends %s", child, parent)

	// 该类的结构视图也应标记父类在 jar 内。
	childStruct, err := store.ClassStructure(version, SideClient, child)
	if err != nil {
		t.Fatalf("继承样本类结构解析失败: %v", err)
	}
	if !childStruct.InJarSuper || childStruct.Super != parent {
		t.Fatalf("继承样本父类信息异常：%+v", childStruct)
	}

	hier, err := store.Hierarchy(ctx, version, SideClient, child)
	if err != nil {
		t.Fatalf("真实 jar 继承关系解析失败: %v", err)
	}
	if len(hier.Ancestors) == 0 || hier.Ancestors[0] != parent {
		t.Fatalf("祖先链首项应为直接父类 %s：%+v", parent, hier.Ancestors)
	}
	last := hier.Ancestors[len(hier.Ancestors)-1]
	if last != "java.lang.Object" {
		t.Fatalf("祖先链终点应为 java.lang.Object，实际 %q（链=%v）", last, hier.Ancestors)
	}
	if hier.AncestorsInJar {
		t.Fatalf("祖先链终点在 jar 外，AncestorsInJar 应为 false：%+v", hier.Ancestors)
	}
	t.Logf("%s 祖先链：%v", child, hier.Ancestors)

	// 反向验证：父类视角下 child 必须出现在子类列表中。
	parentHier, err := store.Hierarchy(ctx, version, SideClient, parent)
	if err != nil {
		t.Fatalf("父类继承关系解析失败: %v", err)
	}
	var listed bool
	for _, d := range parentHier.Descendants {
		if d.Name == child {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("%s 的子类列表应包含 %s（共 %d 个）：%+v",
			parent, child, parentHier.DescendantCount, parentHier.Descendants)
	}

	// MinecraftServer 自身：父类在 jar 外，计数与列表应自洽。
	msHier, err := store.Hierarchy(ctx, version, SideClient, "net.minecraft.server.MinecraftServer")
	if err != nil {
		t.Fatalf("MinecraftServer 继承关系解析失败: %v", err)
	}
	if msHier.DescendantCount < len(msHier.Descendants) {
		t.Fatalf("子类计数不应小于列表长度：%+v", msHier)
	}
	if msHier.Super != "java.lang.Object" || msHier.AncestorsInJar {
		t.Fatalf("MinecraftServer 父类信息异常：%+v", msHier)
	}

	// java.lang.Object 不在 jar 内 → 类不存在。
	if _, err := store.Hierarchy(ctx, version, SideClient, "java.lang.Object"); !errors.Is(err, ErrClassNotFound) {
		t.Fatalf("jar 外类期望 ErrClassNotFound，实际 %v", err)
	}

	// 关系索引已缓存：二次调用应为毫秒级（此处只验证结果一致）。
	hier2, err := store.Hierarchy(ctx, version, SideClient, child)
	if err != nil {
		t.Fatalf("二次继承关系解析失败: %v", err)
	}
	if len(hier2.Ancestors) != len(hier.Ancestors) || hier2.DescendantCount != hier.DescendantCount {
		t.Fatalf("二次结果不一致：%+v vs %+v", hier, hier2)
	}
}
