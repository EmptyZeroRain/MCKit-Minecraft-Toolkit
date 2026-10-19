// main 程序：映射数据导入器（TODO §21「版本映射与类 / 方法查询」）。
//
// 从官方映射文件导入类 / 成员映射，供站内「版本映射查询」检索：
//
//	Mojang 官方映射（ProGuard）：
//	  go run ./cmd/import-mappings --file client.txt \
//	    --mc-version 1.21.1 --mapping-type mojang --channel mojang \
//	    --source-url https://piston-data.mojang.com/.../client.txt \
//	    --license "Minecraft EULA（不得原样再分发完整映射）"
//
//	Fabric Yarn（Tiny v2，需先从 jar 解出）：
//	  unzip -p yarn-1.21.1+build.3-v2.jar mappings/mappings.tiny > mappings.tiny
//	  go run ./cmd/import-mappings --file mappings.tiny \
//	    --mc-version 1.21.1 --mapping-type yarn --channel fabric --build-label 1.21.1+build.3
//
//	TSRG / TSRG2（MCPConfig，Forge / NeoForge 历史体系）：
//	  go run ./cmd/import-mappings --file joined.tsrg --format tsrg \
//	    --mc-version 1.16.5 --mapping-type tsrg --channel forge
//
// 设计说明：
//   - 导入是「按数据集整体替换」的：同一 set_key 重新导入会先清空旧类 / 成员，避免残留脏数据；
//   - 不臆造数据：解析器未提供的命名空间与描述符一律留空，各列语义写入 mapping_sets.note；
//   - 大文件（官方映射约 10MB / 数十万成员）按批写入，避免一次性构造巨量 SQL 参数。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/envutil"
	"mckit/backend/internal/mappings"
	db "mckit/backend/internal/repository"
)

// classBatch / memberBatch 批量写入规模（Postgres 单语句参数上限内）。
const (
	classBatch  = 500
	memberBatch = 1000
)

// maxImportBytes 单次导入文件体积上限（512MB），防止误传超大文件。
const maxImportBytes = 512 * 1024 * 1024

func main() {
	_ = envutil.LoadDotEnv()

	var (
		filePath    string
		format      string
		mcVersion   string
		mappingType string
		channel     string
		buildLabel  string
		setKey      string
		sourceURL   string
		license     string
		note        string
		limit       int
		dryRun      bool
	)

	flag.StringVar(&filePath, "file", "", "映射文件路径（必填）")
	flag.StringVar(&format, "format", "auto", "映射格式：auto|proguard|tsrg|tiny|tiny_v1|tiny_v2")
	flag.StringVar(&mcVersion, "mc-version", "", "目标 Minecraft 版本（必填，如 1.21.1）")
	flag.StringVar(&mappingType, "mapping-type", "", "映射类型：mojang|srg|tsrg|yarn|parchment|mixed（必填）")
	flag.StringVar(&channel, "channel", "", "来源渠道（mojang|forge|neoforge|fabric|parchment）")
	flag.StringVar(&buildLabel, "build-label", "", "构建号 / 版本标签（如 1.21.1+build.3）")
	flag.StringVar(&setKey, "set-key", "", "数据集唯一键；留空按 <type>-<version>[-build] 生成")
	flag.StringVar(&sourceURL, "source-url", "", "数据来源 URL（建议填官方下载地址，便于溯源）")
	flag.StringVar(&license, "license", "", "许可与再分发说明")
	flag.StringVar(&note, "note", "", "补充说明（默认写入各名字列对应的命名空间）")
	flag.IntVar(&limit, "limit", 0, "仅导入前 N 个类（0 表示不限，用于抽样验证）")
	flag.BoolVar(&dryRun, "dry-run", false, "只解析并打印统计，不写库")
	flag.Parse()

	if strings.TrimSpace(filePath) == "" || strings.TrimSpace(mcVersion) == "" || strings.TrimSpace(mappingType) == "" {
		flag.Usage()
		log.Fatal("--file / --mc-version / --mapping-type 均为必填")
	}
	if !domain.IsValidMappingType(mappingType) {
		log.Fatalf("非法映射类型 %q，可选：%s", mappingType, strings.Join(domain.MappingTypes, " / "))
	}

	// 读取并解析
	info, err := os.Stat(filePath)
	if err != nil {
		log.Fatalf("读取文件失败: %v", err)
	}
	if info.Size() > maxImportBytes {
		log.Fatalf("文件过大（%d 字节），超过上限 %d 字节", info.Size(), maxImportBytes)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("读取文件失败: %v", err)
	}

	result, err := mappings.Parse(data, format)
	if err != nil {
		log.Fatalf("解析映射失败: %v", err)
	}
	if len(result.Classes) == 0 {
		log.Fatal("未解析出任何类映射，请确认文件格式与 --format 是否匹配")
	}
	if limit > 0 && limit < len(result.Classes) {
		result.Classes = result.Classes[:limit]
		result.MemberCount = 0
		for _, c := range result.Classes {
			result.MemberCount += len(c.Members)
		}
	}

	key := strings.TrimSpace(setKey)
	if key == "" {
		key = defaultSetKey(mappingType, mcVersion, buildLabel)
	}
	noteText := strings.TrimSpace(note)
	if noteText == "" {
		noteText = namespaceNote(result.Namespaces)
	}

	fmt.Printf("格式=%s 数据集=%s 类=%d 成员=%d\n", result.Format, key, len(result.Classes), result.MemberCount)
	if dryRun {
		fmt.Println("dry-run：未写入数据库")
		return
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("缺少必需配置 DATABASE_URL，请在环境或 .env 中设置")
	}
	gormDB, err := db.Open(databaseURL)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		log.Fatalf("获取底层连接失败: %v", err)
	}
	defer sqlDB.Close()

	ctx := context.Background()
	setID, err := importResult(ctx, gormDB, importParams{
		SetKey:      key,
		MCVersion:   mcVersion,
		MappingType: mappingType,
		Channel:     channel,
		BuildLabel:  buildLabel,
		SourceURL:   sourceURL,
		License:     license,
		Note:        noteText,
	}, result)
	if err != nil {
		log.Fatalf("导入失败: %v", err)
	}
	fmt.Printf("导入完成：数据集 id=%s，类=%d，成员=%d\n", setID, len(result.Classes), result.MemberCount)
}

// importParams 写入 mapping_sets 的元数据。
type importParams struct {
	SetKey      string
	MCVersion   string
	MappingType string
	Channel     string
	BuildLabel  string
	SourceURL   string
	License     string
	Note        string
}

// importResult 在单个事务内整体替换数据集内容。
func importResult(ctx context.Context, gormDB *gorm.DB, p importParams, res *mappings.Result) (uuid.UUID, error) {
	var setID uuid.UUID
	err := gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()

		// 1. upsert 数据集元数据
		var set domain.MappingSet
		err := tx.Where("set_key = ?", p.SetKey).First(&set).Error
		switch {
		case err == nil:
			if err := tx.Model(&domain.MappingSet{}).Where("id = ?", set.ID).
				Updates(map[string]any{
					"mc_version":   p.MCVersion,
					"mapping_type": p.MappingType,
					"channel":      p.Channel,
					"build_label":  p.BuildLabel,
					"source_url":   p.SourceURL,
					"license":      p.License,
					"note":         p.Note,
					"class_count":  len(res.Classes),
					"member_count": res.MemberCount,
					"imported_at":  now,
					"updated_at":   now,
				}).Error; err != nil {
				return err
			}
			// 整体替换：先清空旧数据（成员由外键级联删除）
			if err := tx.Where("set_id = ?", set.ID).Delete(&domain.MappingClass{}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			set = domain.MappingSet{
				ID:          uuid.New(),
				SetKey:      p.SetKey,
				MCVersion:   p.MCVersion,
				MappingType: p.MappingType,
				Channel:     p.Channel,
				BuildLabel:  p.BuildLabel,
				SourceURL:   p.SourceURL,
				License:     p.License,
				Note:        p.Note,
				ClassCount:  len(res.Classes),
				MemberCount: res.MemberCount,
				ImportedAt:  &now,
			}
			if err := tx.Create(&set).Error; err != nil {
				return err
			}
		default:
			return err
		}
		setID = set.ID

		// 2. 写入类与成员（显式生成 UUID，避免依赖 RETURNING 的顺序回填）
		classes := make([]domain.MappingClass, 0, len(res.Classes))
		members := make([]domain.MappingMember, 0, res.MemberCount)
		seenMember := make(map[string]struct{}, res.MemberCount)

		for _, c := range res.Classes {
			classID := uuid.New()
			classes = append(classes, domain.MappingClass{
				ID:          classID,
				SetID:       setID,
				ObfName:     c.ObfName,
				MidName:     c.MidName,
				NamedName:   c.NamedName,
				Package:     c.Package,
				MemberCount: len(c.Members),
			})
			for _, m := range c.Members {
				dedupeKey := c.ObfName + "\x00" + m.Kind + "\x00" + m.ObfName + "\x00" + m.DescKey
				if _, ok := seenMember[dedupeKey]; ok {
					continue
				}
				seenMember[dedupeKey] = struct{}{}
				members = append(members, domain.MappingMember{
					ID:        uuid.New(),
					ClassID:   classID,
					Kind:      m.Kind,
					ObfName:   m.ObfName,
					MidName:   m.MidName,
					NamedName: m.NamedName,
					DescKey:   m.DescKey,
					ObfDesc:   m.ObfDesc,
					NamedDesc: m.NamedDesc,
				})
			}
		}

		if len(classes) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(classes, classBatch).Error; err != nil {
				return err
			}
		}
		if len(members) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				CreateInBatches(members, memberBatch).Error; err != nil {
				return err
			}
		}

		// 3. 以实际写入的成员数回填元数据（去重后可能与解析数略有差异）
		if err := tx.Model(&domain.MappingSet{}).Where("id = ?", setID).
			Updates(map[string]any{"member_count": len(members), "updated_at": now}).Error; err != nil {
			return err
		}
		return nil
	})
	return setID, err
}

// defaultSetKey 生成数据集唯一键：<type>-<version>[-<build 归一化>]。
func defaultSetKey(mappingType, mcVersion, buildLabel string) string {
	key := mappingType + "-" + mcVersion
	if b := normalizeBuild(buildLabel); b != "" {
		key += "-" + b
	}
	return key
}

// normalizeBuild 归一化构建号（去掉 +build. 等分隔符，仅保留字母数字与点）。
func normalizeBuild(build string) string {
	build = strings.TrimSpace(build)
	if build == "" {
		return ""
	}
	var sb strings.Builder
	for _, r := range build {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.':
			sb.WriteRune(r)
		case r == '+', r == '-', r == '_', r == ' ':
			sb.WriteRune('.')
		}
	}
	return strings.Trim(sb.String(), ".")
}

// namespaceNote 说明各名字列对应的命名空间，写入 mapping_sets.note。
// 输出形如「各名字列对应的命名空间：obf 列 = obf，named 列 = named（未提供对应命名空间的列为空）。」
func namespaceNote(namespaces []string) string {
	columns := []string{"obf 列", "mid 列", "named 列"}
	labels := make([]string, 0, len(columns))
	for i, ns := range namespaces {
		if i >= len(columns) || ns == "" {
			continue
		}
		labels = append(labels, columns[i]+" = "+ns)
	}
	if len(labels) == 0 {
		return ""
	}
	return "各名字列对应的命名空间：" + strings.Join(labels, "，") + "（未提供对应命名空间的列为空）。"
}
