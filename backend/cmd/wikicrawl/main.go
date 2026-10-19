// main 程序：从中文 Minecraft Wiki 抓取条目并导入本站百科。
//
// 合规说明：抓取内容采用 CC BY-NC-SA 3.0 授权，仅用于本站非商业展示，
// 每条记录保留来源 URL 与许可信息（需求文档 §0.3 与 Wiki 许可要求）。
//
// 用法：
//   go run ./cmd/wikicrawl --titles "交易,附魔,合成,命令"
//   go run ./cmd/wikicrawl --titles-file ./titles.txt
//   go run ./cmd/wikicrawl --titles "村庄" --dry-run
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/envutil"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/wikicrawl"
)

func main() {
	_ = envutil.LoadDotEnv()

	var (
		titlesStr    string
		titlesFile   string
		dryRun       bool
		interval     int
		contentType  string
		gameEdition  string
		userAgent    string
	)
	flag.StringVar(&titlesStr, "titles", "", "要抓取的页面标题，逗号分隔（如：交易,附魔）")
	flag.StringVar(&titlesFile, "titles-file", "", "从文件读取标题，每行一个（# 开头为注释）")
	flag.BoolVar(&dryRun, "dry-run", false, "仅抓取并打印，不写入数据库")
	flag.IntVar(&interval, "interval", 1, "请求间隔秒数（遵守 robots.txt，建议 ≥1）")
	flag.StringVar(&contentType, "content-type", "reference", "百科内容类型：guide|concept|tutorial|reference|other")
	flag.StringVar(&gameEdition, "game-edition", "java", "游戏版本归属：java|bedrock")
	flag.StringVar(&userAgent, "user-agent", "", "自定义 User-Agent（默认内置标识）")
	flag.Parse()

	titles := splitTitles(titlesStr)
	if titlesFile != "" {
		fromFile, err := readTitlesFile(titlesFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取标题文件失败: %v\n", err)
			os.Exit(1)
		}
		titles = append(titles, fromFile...)
	}
	if len(titles) == 0 {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/wikicrawl --titles \"交易,附魔\" [--dry-run]")
		os.Exit(1)
	}

	crawler := wikicrawl.NewCrawler(time.Duration(interval) * time.Second)
	crawler.SetUserAgent(userAgent)

	if dryRun {
		runDryRun(crawler, titles)
		return
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "缺少 DATABASE_URL")
		os.Exit(1)
	}
	gormDB, err := db.Open(databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库失败: %v\n", err)
		os.Exit(1)
	}
	sqlDB, _ := gormDB.DB()
	defer sqlDB.Close()

	ctx := context.Background()
	var created, failed int
	for _, t := range titles {
		art, err := crawler.Fetch(ctx, t)
		if err != nil {
			fmt.Printf("[失败] %s: %v\n", t, err)
			failed++
			continue
		}

		now := time.Now()
		entry := &domain.WikiEntry{
			Slug:          "wiki-" + art.Slug,
			Title:         art.Title,
			Summary:       art.Summary,
			Content:       art.Content,
			ContentFormat: "markdown", // 前端安全渲染，禁用原始 HTML
			ContentType:   contentType,
			GameEdition:   gameEdition,
			GameVersions:  types.JSONSlice{},
			Tags:          types.JSONSlice{"wiki", "crawled"},
			SourceReferences: mustJSONB([]map[string]string{
				// 前端 SourceReference 契约字段为 title/url；额外保留 source/license 便于审计。
				{
					"title":   art.Source + "：" + art.Title,
					"url":     art.URL,
					"source":  art.Source,
					"license": art.License,
				},
			}),
			Status:      domain.StatusPublished,
			PublishedAt: &now,
		}

		res := gormDB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{"title", "summary", "content", "content_type", "game_edition", "source_references", "status", "updated_at"}),
		}).Create(entry)
		if res.Error != nil {
			fmt.Printf("[失败] %s: %v\n", t, res.Error)
			failed++
			continue
		}
		created++
		mark := ""
		if art.Redirected {
			mark = "（重定向）"
		}
		fmt.Printf("[成功] %s%s -> slug=%s | 正文 %d 字\n", art.Title, mark, entry.Slug, len([]rune(art.Content)))
	}
	fmt.Printf("完成: 成功 %d，失败 %d\n", created, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func runDryRun(crawler *wikicrawl.Crawler, titles []string) {
	ctx := context.Background()
	for _, t := range titles {
		art, err := crawler.Fetch(ctx, t)
		if err != nil {
			fmt.Printf("[跳过] %s: %v\n", t, err)
			continue
		}
		mark := ""
		if art.Redirected {
			mark = "（重定向自 " + art.Requested + "）"
		}
		fmt.Printf("[dry-run] %s%s | slug=%s | 正文 %d 字 | 摘要: %.50s | %s\n",
			art.Title, mark, art.Slug, len([]rune(art.Content)), art.Summary, art.URL)
	}
}

func splitTitles(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// readTitlesFile 读取每行一个标题的文件，忽略空行与 # 注释。
func readTitlesFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}

func mustJSONB(v any) types.JSONB {
	b, _ := json.Marshal(v)
	return types.JSONB(b)
}
