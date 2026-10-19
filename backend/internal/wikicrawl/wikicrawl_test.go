package wikicrawl

import (
	"strings"
	"testing"
)

// 构造一个贴近 MediaWiki 现行结构的页面片段：
// 正文容器为 <div class="mw-content-ltr mw-parser-output">，
// 内含标题、段落、编辑链接、目录、导航框、引用、列表、表格与链接。
const fixtureHTML = `<!DOCTYPE html>
<html><head><title>附魔 - 中文 Minecraft Wiki</title></head>
<body>
<div id="mw-content-text" class="mw-body-content">
<div class="mw-content-ltr mw-parser-output" lang="zh-Hans-CN" dir="ltr">
<h1 id="firstHeading" class="firstHeading mw-first-heading">魔咒</h1>
<div class="hatnote searchaux">本条目介绍的是附魔机制。</div>
<div id="toc" class="toc"><ul><li class="toclevel-1"><a href="#a"><span class="tocnumber">1</span><span class="toctext">机制</span></a></li></ul></div>
<h2><span class="mw-heading mw-heading2">机制</span><span class="mw-editsection"><span class="mw-editsection-bracket">[</span><a href="/w/%E9%99%84%E9%AD%94?action=edit">编辑</a><span class="mw-editsection-bracket">]</span></span></h2>
<figure class="mw-default-size pixel-image" typeof="mw:File/Thumb"><a href="/w/File:Foo.png"><img src="/x.png" /></a><figcaption>图片说明不应进入正文或摘要</figcaption></figure>
<p>魔咒（Enchantment）是能赋予<strong>盔甲</strong>与<em>工具</em>特殊能力的机制，可通过<a href="/w/%E9%99%84%E9%AD%94%E5%8F%B0">附魔台</a>实现。<sup class="reference"><a href="#cite_note-1">[1]</a></sup></p>
<ul><li>锋利</li><li>保护</li></ul>
<table class="wikitable">
<caption>示例表</caption>
<tbody>
<tr><th>名称</th><th>等级</th></tr>
<tr><td>锋利</td><td>V</td></tr>
<tr><td>保护</td><td></td></tr>
</tbody>
</table>
<div class="navbox">导航框内容不应出现</div>
<div class="mw-references-wrap"><ol class="references"><li id="cite_note-1">参考文献不应出现</li></ol></div>
<p>外部链接：<a href="https://www.minecraft.net/">官网</a>；红链：<a class="new" href="/w/%E4%B8%8D%E5%AD%98%E5%9C%A8">不存在</a>。</p>
</div>
</div>
<div class="printfooter">检索自“魔咒”</div>
</body></html>`

func TestExtractArticle(t *testing.T) {
	art := extractArticle(fixtureHTML, "附魔", "https://zh.minecraft.wiki/w/%E9%99%84%E9%AD%94")

	if art.Title != "魔咒" {
		t.Errorf("标题提取错误: got %q, want %q", art.Title, "魔咒")
	}
	if art.Slug != "魔咒" {
		t.Errorf("slug 生成错误: got %q, want %q", art.Slug, "魔咒")
	}

	checks := []struct {
		name   string
		want   bool
		substr string
	}{
		{"包含二级标题", true, "## 机制"},
		{"包含粗体", true, "**盔甲**"},
		{"包含斜体", true, "*工具*"},
		{"内部链接转为绝对 Markdown", true, "[附魔台](https://zh.minecraft.wiki/w/%E9%99%84%E9%AD%94%E5%8F%B0)"},
		{"包含无序列表", true, "- 锋利"},
		{"包含 Markdown 表格", true, "| 名称 | 等级 |"},
		{"表格分隔行", true, "| --- | --- |"},
		{"保留空单元格占位", true, "| 保护 |  |"},
		{"外部链接降级为纯文本", true, "官网"},
		{"编辑链接被过滤", false, "action=edit"},
		{"目录被过滤", false, "机制</a>"},
		{"导航框被过滤", false, "导航框内容不应出现"},
		{"引用列表被过滤", false, "参考文献不应出现"},
		{"printfooter 被过滤", false, "检索自"},
		{"图片说明被过滤", false, "图片说明不应进入正文或摘要"},
	}
	for _, c := range checks {
		got := strings.Contains(art.Content, c.substr)
		if got != c.want {
			t.Errorf("%s: 包含 %q = %v, want %v\n---内容---\n%s", c.name, c.substr, got, c.want, art.Content)
		}
	}

	// 外部链接不应生成 Markdown 链接
	if strings.Contains(art.Content, "(https://www.minecraft.net/") {
		t.Errorf("外链不应转为 Markdown 链接:\n%s", art.Content)
	}
	// 红链不应生成 Markdown 链接
	if strings.Contains(art.Content, "不存在](http") {
		t.Errorf("红链不应转为 Markdown 链接:\n%s", art.Content)
	}
	// 摘要应来自正文首段，而非 <title>
	if art.Summary == "" || strings.Contains(art.Summary, "中文 Minecraft Wiki") {
		t.Errorf("摘要提取错误: %q", art.Summary)
	}
	if !strings.HasPrefix(art.Summary, "魔咒") {
		t.Errorf("摘要应取自首段: %q", art.Summary)
	}
}

func TestExtractArticleTitleFallback(t *testing.T) {
	// 缺少 h1#firstHeading 时回退 <title>，并去掉站点后缀
	raw := `<html><head><title>交易 - 中文 Minecraft Wiki</title></head>
<body><div id="mw-content-text"><div class="mw-parser-output"><p>交易是村民与玩家之间的物品交换机制。</p></div></div></body></html>`
	art := extractArticle(raw, "交易", "https://zh.minecraft.wiki/w/%E4%BA%A4%E6%98%93")
	if art.Title != "交易" {
		t.Errorf("标题回退错误: got %q", art.Title)
	}
	if art.Summary == "" {
		t.Errorf("摘要为空")
	}
}

func TestSlugifyTitle(t *testing.T) {
	cases := map[string]string{
		"交易":               "交易",
		"生物群系":             "生物群系",
		"China Edition:Wiki": "china-edition-wiki",
		"Java 版":            "java-版",
		"  Hello World  ":    "hello-world",
	}
	for in, want := range cases {
		if got := slugifyTitle(in); got != want {
			t.Errorf("slugifyTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsDisallowed(t *testing.T) {
	for _, t2 := range []string{"File:Abc.png", "Special:RecentChanges", "User:Foo", "Template:Bar"} {
		if !isDisallowed(t2) {
			t.Errorf("%q 应被禁止", t2)
		}
	}
	for _, t2 := range []string{"交易", "附魔", "命令", "China_Edition:Wiki"} {
		if isDisallowed(t2) {
			t.Errorf("%q 不应被禁止", t2)
		}
	}
}
