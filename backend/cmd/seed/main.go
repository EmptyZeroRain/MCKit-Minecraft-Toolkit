// main 程序：数据导入器（需求 §10.1；TODO §4 统一导入入口）。
// 从 JSON 文件导入 items / potions / devdocs / recipes / commands / tools / resources，支持 dry-run、
// 幂等重跑、逐行校验与失败汇总，并把每次执行记录到 data_import_jobs（数据质量监控）。
//
// 用法：
//
//	go run ./cmd/seed --type items --file seeds/items_demo.json
//	go run ./cmd/seed --type potions --file seeds/potions_demo.json
//	go run ./cmd/seed --type devdocs --file seeds/dev_docs_demo.json
//	go run ./cmd/seed --type all --file seeds --dry-run
//	go run ./cmd/seed --type recipes --file seeds/recipes_demo.json --dry-run
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mckit/backend/internal/domain"
	"mckit/backend/internal/envutil"
	db "mckit/backend/internal/repository"
	"mckit/backend/internal/repository/types"
	"mckit/backend/internal/versioning"
)

func main() {
	_ = envutil.LoadDotEnv()

	var (
		filePath string
		dataType string
		dryRun   bool
	)
	flag.StringVar(&filePath, "file", "", "JSON 数据文件路径；--type all 时为目录（默认 seeds）")
	flag.StringVar(&dataType, "type", "", "数据类型：items | potions | villagers | mobs | redstone | mods | devdocs | recipes | commands | tools | resources | all")
	flag.BoolVar(&dryRun, "dry-run", false, "仅预检查，不写入")
	flag.Parse()

	if dataType == "" {
		fmt.Fprintln(os.Stderr, "用法: go run ./cmd/seed --type <items|potions|villagers|mobs|redstone|mods|devdocs|recipes|commands|tools|resources|all> --file <path|dir> [--dry-run]")
		os.Exit(1)
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

	if dataType == "all" {
		importAll(gormDB, filePath, dryRun)
		return
	}

	if filePath == "" {
		fmt.Fprintln(os.Stderr, "缺少 --file")
		os.Exit(1)
	}
	if runImport(gormDB, dataType, filePath, dryRun) == nil {
		os.Exit(1)
	}
}

// importAll 统一导入入口：按依赖顺序导入 items → potions → recipes → commands → tools → resources。
// fileOrDir 为空时默认使用 seeds 目录下的演示数据。
func importAll(gormDB *gorm.DB, fileOrDir string, dryRun bool) {
	dir := strings.TrimSpace(fileOrDir)
	if dir == "" {
		dir = "seeds"
	}

	var failed bool
	for _, dt := range []string{"items", "potions", "villagers", "mobs", "redstone", "mods", "devdocs", "recipes", "commands", "tools", "resources"} {
		path := seedFilePath(dir, dt)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("跳过 %s：未找到 %s\n", dt, path)
			continue
		}
		if runImport(gormDB, dt, path, dryRun) == nil {
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

// seedFilePath 返回某数据类型在目录下的约定文件名。
func seedFilePath(dir, dataType string) string {
	switch dataType {
	case "items":
		return filepath.Join(dir, "items_demo.json")
	case "potions":
		return filepath.Join(dir, "potions_demo.json")
	case "villagers":
		return filepath.Join(dir, "villagers_demo.json")
	case "mobs":
		return filepath.Join(dir, "mobs_demo.json")
	case "redstone":
		return filepath.Join(dir, "redstone_demo.json")
	case "mods":
		return filepath.Join(dir, "mods_demo.json")
	case "devdocs":
		return filepath.Join(dir, "dev_docs_demo.json")
	case "recipes":
		return filepath.Join(dir, "recipes_demo.json")
	case "commands":
		return filepath.Join(dir, "commands_demo.json")
	case "tools":
		return filepath.Join(dir, "tools_builtin.json")
	case "resources":
		return filepath.Join(dir, "resources_demo.json")
	default:
		return ""
	}
}

// runImport 读取文件、按类型导入、输出报表并记录导入任务。
// 返回 nil 表示数据处理失败（存在失败行或未知类型）。
func runImport(gormDB *gorm.DB, dataType, filePath string, dryRun bool) *summary {
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取文件失败: %v\n", err)
		return nil
	}

	var s *summary
	switch dataType {
	case "items":
		s = importItems(gormDB, content, dryRun)
	case "potions":
		s = importPotions(gormDB, content, dryRun)
	case "villagers":
		s = importVillagers(gormDB, content, dryRun)
	case "mobs":
		s = importMobs(gormDB, content, dryRun)
	case "redstone":
		s = importRedstone(gormDB, content, dryRun)
	case "mods":
		s = importMods(gormDB, content, dryRun)
	case "devdocs":
		s = importDevDocs(gormDB, content, dryRun)
	case "recipes":
		s = importRecipes(gormDB, content, dryRun)
	case "commands":
		s = importCommands(gormDB, content, dryRun)
	case "tools":
		s = importTools(gormDB, content, dryRun)
	case "resources":
		s = importResources(gormDB, content, dryRun)
	default:
		fmt.Fprintf(os.Stderr, "未知数据类型: %s\n", dataType)
		return nil
	}

	s.report(dataType)
	recordJob(gormDB, dataType, filePath, dryRun, s)
	if s.failed > 0 {
		return nil
	}
	return s
}

// summary 导入统计。
type summary struct {
	total    int
	created  int
	updated  int
	skipped  int
	failed   int
	messages []string
}

func (s *summary) report(dataType string) {
	fmt.Printf("导入完成 [%s]: 共 %d 条，新增 %d，更新 %d，跳过 %d，失败 %d\n",
		dataType, s.total, s.created, s.updated, s.skipped, s.failed)
	for _, m := range s.messages {
		fmt.Println("  -", m)
	}
}

// fail 记录一条失败并附带原因，统一维护失败计数与信息。
func (s *summary) fail(reason string) {
	s.failed++
	s.messages = append(s.messages, reason)
}

// countOutcome 按「新增 / 更新」累计计数。
func (s *summary) countOutcome(isNew bool) {
	if isNew {
		s.created++
		return
	}
	s.updated++
}

// upsertVerb 返回 dry-run 输出用的动作词。
func upsertVerb(isNew bool) string {
	if isNew {
		return "新增"
	}
	return "更新"
}

// isNewRecord 判断按业务主键是否已存在。
//
// 背景：PostgreSQL 的 INSERT ... ON CONFLICT DO UPDATE 无论插入还是更新都返回 1 行，
// RowsAffected 无法区分二者；而导入监控需要准确的新增 / 更新数量，故显式预查。
func isNewRecord(gormDB *gorm.DB, model any, where string, args ...any) (bool, error) {
	var n int64
	if err := gormDB.Model(model).Where(where, args...).Count(&n).Error; err != nil {
		return false, err
	}
	return n == 0, nil
}

// maxErrorSummary 记录到任务表的错误摘要条数上限，避免摘要无限膨胀。
const maxErrorSummary = 50

// recordJob 把一次导入执行记录到 data_import_jobs（TODO §4）。
// 记录失败不阻断导入本身，仅降级为 stderr 输出。
func recordJob(gormDB *gorm.DB, dataType, filePath string, dryRun bool, s *summary) {
	if gormDB == nil || s == nil {
		return
	}
	job := buildImportJob(dataType, filePath, dryRun, s, importOperator(), time.Now())
	if err := gormDB.Create(&job).Error; err != nil {
		fmt.Fprintf(os.Stderr, "记录导入任务失败: %v\n", err)
	}
}

// buildImportJob 由导入统计构造任务记录（纯函数，便于单测）。
// 错误摘要超过上限时截断并追加省略说明，避免单条记录过大。
func buildImportJob(dataType, filePath string, dryRun bool, s *summary, operator string, now time.Time) domain.DataImportJob {
	msgs := s.messages
	if len(msgs) > maxErrorSummary {
		msgs = append(append([]string{}, msgs[:maxErrorSummary]...),
			fmt.Sprintf("……另有 %d 条未展示", len(s.messages)-maxErrorSummary))
	}
	return domain.DataImportJob{
		DataType:     dataType,
		SourceFile:   filePath,
		DryRun:       dryRun,
		TotalCount:   s.total,
		CreatedCount: s.created,
		UpdatedCount: s.updated,
		SkippedCount: s.skipped,
		FailedCount:  s.failed,
		ErrorSummary: types.JSONSlice(append([]string{}, msgs...)),
		Operator:     operator,
		StartedAt:    now,
		FinishedAt:   &now,
	}
}

// importOperator 记录执行者：优先 IMPORT_OPERATOR，其次系统用户名。
func importOperator() string {
	if v := strings.TrimSpace(os.Getenv("IMPORT_OPERATOR")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

// validateVersions 校验版本列表（格式合法 + 无重复），返回聚合错误。
func validateVersions(label string, versions []string) error {
	errs := versioning.Validate(versions)
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return fmt.Errorf("%s 校验失败: %s", label, strings.Join(parts, "；"))
}

// itemRow 物品导入原始行（TODO §1：统一物品资料库）。
type itemRow struct {
	ItemID            string   `json:"item_id"`
	NameZh            string   `json:"name_zh"`
	NameEn            string   `json:"name_en"`
	Aliases           []string `json:"aliases"`
	Description       string   `json:"description"`
	ItemCategory      string   `json:"item_category"`
	StackSize         *int     `json:"stack_size"`
	Durability        *int     `json:"durability"`
	ObtainMethod      string   `json:"obtain_method"`
	Usage             string   `json:"usage"`
	ImageURL          string   `json:"image_url"`
	GameEdition       string   `json:"game_edition"`
	SupportedVersions []string `json:"supported_versions"`
	SourceURL         string   `json:"source_url"`
	DataVersion       string   `json:"data_version"`
	LastVerifiedAt    string   `json:"last_verified_at"`
}

// buildItem 校验并归一化单行物品资料。
// 必填项缺失、类别非法、数值越界、版本非法或时间格式错误均视为致命错误（跳过该行）。
func buildItem(row itemRow) (*domain.Item, error) {
	itemID := strings.TrimSpace(row.ItemID)
	nameZh := strings.TrimSpace(row.NameZh)
	if itemID == "" || nameZh == "" {
		return nil, errors.New("item_id 与 name_zh 必填")
	}

	category := strings.TrimSpace(row.ItemCategory)
	if category == "" {
		category = domain.ItemCategoryOther
	}
	if !domain.IsValidItemCategory(category) {
		return nil, fmt.Errorf("item_category 非法: %q", row.ItemCategory)
	}
	if row.Durability != nil && *row.Durability < 0 {
		return nil, errors.New("durability 不能为负")
	}
	if row.StackSize != nil && *row.StackSize < 0 {
		return nil, errors.New("stack_size 不能为负")
	}

	aliases := row.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	versions := row.SupportedVersions
	if versions == nil {
		versions = []string{}
	}
	if err := validateVersions("supported_versions", versions); err != nil {
		return nil, err
	}
	edition := row.GameEdition
	if edition == "" {
		edition = "java"
	}

	item := &domain.Item{
		ItemID:            itemID,
		NameZh:            nameZh,
		NameEn:            strings.TrimSpace(row.NameEn),
		Aliases:           types.JSONSlice(aliases),
		Description:       row.Description,
		ItemCategory:      category,
		StackSize:         row.StackSize,
		Durability:        row.Durability,
		ObtainMethod:      row.ObtainMethod,
		UsageNotes:        row.Usage,
		ImageURL:          row.ImageURL,
		GameEdition:       edition,
		SupportedVersions: types.JSONSlice(versioning.Sort(versions)),
		SourceURL:         row.SourceURL,
		DataVersion:       row.DataVersion,
	}

	if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("last_verified_at 需为 RFC3339 时间: %v", err)
		}
		item.LastVerifiedAt = &t
	}
	return item, nil
}

func importItems(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var rows []itemRow
	if err := json.Unmarshal(content, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(rows)}
	for i, row := range rows {
		item, err := buildItem(row)
		if err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("第 %d 行：%v", i+1, err))
			continue
		}

		// 数据质量提醒（非阻断）：来源与适用版本缺失时提示补全（TODO §1）。
		if item.SourceURL == "" {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 缺少 source_url", item.ItemID))
		}
		if len(item.SupportedVersions) == 0 {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 缺少 supported_versions", item.ItemID))
		}

		isNew, err := isNewRecord(gormDB, &domain.Item{}, "item_id = ?", item.ItemID)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", item.ItemID, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s item %s (%s)\n", upsertVerb(isNew), item.ItemID, item.NameZh)
			s.countOutcome(isNew)
			continue
		}

		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "item_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name_zh", "name_en", "aliases", "description", "item_category", "stack_size",
				"durability", "obtain_method", "usage_notes", "image_url", "game_edition",
				"supported_versions", "source_url", "data_version", "last_verified_at", "updated_at",
			}),
		}).Create(item).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.ItemID, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// recipeRow 配方导入原始行。
type recipeRow struct {
	RecipeKey         string            `json:"recipe_key"`
	RecipeType        string            `json:"recipe_type"`
	GameVersion       string            `json:"game_version"`
	ResultItemID      string            `json:"result_item_id"`
	ResultCount       int               `json:"result_count"`
	Pattern           []string          `json:"pattern"`
	KeyMapping        map[string]string `json:"key_mapping"`
	Ingredients       []json.RawMessage `json:"ingredients"`
	RequiresCrafting  bool              `json:"requires_crafting_table"`
	SupportedVersions []string          `json:"supported_versions"`
	SourceURL         string            `json:"source_url"`
	DataVersion       string            `json:"data_version"`
	LastVerifiedAt    string            `json:"last_verified_at"`
}

func importRecipes(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var rows []recipeRow
	if err := json.Unmarshal(content, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(rows)}
	for i, row := range rows {
		if row.RecipeKey == "" || row.ResultItemID == "" {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("第 %d 行：recipe_key 与 result_item_id 必填", i+1))
			continue
		}
		if strings.TrimSpace(row.GameVersion) == "" {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s：game_version 必填", row.RecipeKey))
			continue
		}
		if !versioning.IsValid(row.GameVersion) {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s：game_version 格式非法 %q", row.RecipeKey, row.GameVersion))
			continue
		}
		if err := validateVersions("supported_versions", row.SupportedVersions); err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s：%v", row.RecipeKey, err))
			continue
		}
		// 校验结果物品必须存在于 items 表（需求 §7.6 外键语义）。
		var count int64
		if err := gormDB.Model(&domain.Item{}).Where("item_id = ?", row.ResultItemID).Count(&count).Error; err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s 引用校验失败: %v", row.RecipeKey, err))
			continue
		}
		if count == 0 {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s：结果物品 %s 不存在，跳过", row.RecipeKey, row.ResultItemID))
			continue
		}

		ingredients := marshalRaw(row.Ingredients)
		keyMapping := marshalStringMap(row.KeyMapping)
		pattern := row.Pattern
		if pattern == nil {
			pattern = []string{}
		}

		recipe := &domain.Recipe{
			RecipeKey:         row.RecipeKey,
			RecipeType:        row.RecipeType,
			GameVersion:       row.GameVersion,
			ResultItemID:      row.ResultItemID,
			ResultCount:       row.ResultCount,
			Pattern:           typesJSONSlice(pattern),
			KeyMapping:        typesMap(keyMapping),
			Ingredients:       typesIngredientSlice(ingredients),
			RequiresCrafting:  row.RequiresCrafting,
			SupportedVersions: types.JSONSlice(versioning.Sort(row.SupportedVersions)),
			SourceURL:         row.SourceURL,
			DataVersion:       row.DataVersion,
		}
		if recipe.ResultCount <= 0 {
			recipe.ResultCount = 1
		}
		if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf("%s：last_verified_at 需为 RFC3339 时间: %v", row.RecipeKey, err))
				continue
			}
			recipe.LastVerifiedAt = &t
		}
		if recipe.SourceURL == "" {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 缺少 source_url", recipe.RecipeKey))
		}
		if len(recipe.SupportedVersions) == 0 {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 缺少 supported_versions", recipe.RecipeKey))
		}

		isNew, err := isNewRecord(gormDB, &domain.Recipe{}, "recipe_key = ?", recipe.RecipeKey)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", recipe.RecipeKey, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s recipe %s (%s)\n", upsertVerb(isNew), recipe.RecipeKey, recipe.ResultItemID)
			s.countOutcome(isNew)
			continue
		}

		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "recipe_key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"recipe_type", "game_version", "result_item_id", "result_count", "pattern",
				"key_mapping", "ingredients", "requires_crafting_table", "supported_versions",
				"source_url", "data_version", "last_verified_at", "updated_at",
			}),
		}).Create(recipe).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.RecipeKey, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// commandRow 指令导入原始行。
type commandRow struct {
	Slug             string           `json:"slug"`
	Name             string           `json:"name"`
	Syntax           string           `json:"syntax"`
	Description      string           `json:"description"`
	Parameters       []map[string]any `json:"parameters"`
	Examples         []map[string]any `json:"examples"`
	PermissionNotes  string           `json:"permission_notes"`
	GameVersions     []string         `json:"game_versions"`
	SourceReferences []map[string]any `json:"source_references"`
	SourceURL        string           `json:"source_url"`
	LastVerifiedAt   string           `json:"last_verified_at"`
	Status           string           `json:"status"`
}

func importCommands(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var rows []commandRow
	if err := json.Unmarshal(content, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(rows)}
	for i, row := range rows {
		if row.Slug == "" || row.Name == "" || row.Syntax == "" {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("第 %d 行：slug/name/syntax 必填", i+1))
			continue
		}
		if err := validateVersions("game_versions", row.GameVersions); err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s：%v", row.Slug, err))
			continue
		}
		cmd := &domain.Command{
			Slug:             row.Slug,
			Name:             row.Name,
			Syntax:           row.Syntax,
			Description:      row.Description,
			Parameters:       mustJSONB(row.Parameters),
			Examples:         mustJSONB(row.Examples),
			PermissionNotes:  row.PermissionNotes,
			GameVersions:     types.JSONSlice(versioning.Sort(row.GameVersions)),
			SourceReferences: mustJSONB(row.SourceReferences),
			SourceURL:        row.SourceURL,
			Status:           row.Status,
		}
		if cmd.Status == "" {
			cmd.Status = domain.StatusPublished
		}
		if raw := strings.TrimSpace(row.LastVerifiedAt); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				s.failed++
				s.messages = append(s.messages, fmt.Sprintf("%s：last_verified_at 需为 RFC3339 时间: %v", row.Slug, err))
				continue
			}
			cmd.LastVerifiedAt = &t
		}
		if cmd.SourceURL == "" {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 缺少 source_url", cmd.Slug))
		}
		if len(cmd.GameVersions) == 0 {
			s.messages = append(s.messages, fmt.Sprintf("提示：%s 缺少 game_versions", cmd.Slug))
		}
		isNew, err := isNewRecord(gormDB, &domain.Command{}, "slug = ?", cmd.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", cmd.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s command %s (%s)\n", upsertVerb(isNew), cmd.Slug, cmd.Name)
			s.countOutcome(isNew)
			continue
		}

		if err := gormDB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "syntax", "description", "parameters", "examples", "permission_notes",
				"game_versions", "source_references", "source_url", "last_verified_at", "status", "updated_at",
			}),
		}).Create(cmd).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

// importTools 导入内置工具记录（按 slug 幂等 upsert）。
func importTools(gormDB *gorm.DB, content []byte, dryRun bool) *summary {
	var rows []struct {
		Slug              string   `json:"slug"`
		Name              string   `json:"name"`
		Summary           string   `json:"summary"`
		Description       string   `json:"description"`
		Category          string   `json:"category"`
		Icon              string   `json:"icon"`
		Tags              []string `json:"tags"`
		SupportedVersions []string `json:"supported_versions"`
		IsFeatured        bool     `json:"is_featured"`
		SortOrder         int      `json:"sort_order"`
		Status            string   `json:"status"`
	}
	if err := json.Unmarshal(content, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 解析失败: %v\n", err)
		os.Exit(1)
	}

	s := &summary{total: len(rows)}
	for i, row := range rows {
		if row.Slug == "" || row.Name == "" || row.Summary == "" {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("第 %d 行：slug/name/summary 必填", i+1))
			continue
		}
		if err := validateVersions("supported_versions", row.SupportedVersions); err != nil {
			s.failed++
			s.messages = append(s.messages, fmt.Sprintf("%s：%v", row.Slug, err))
			continue
		}

		tool := &domain.Tool{
			Slug:              row.Slug,
			Name:              row.Name,
			Summary:           row.Summary,
			Description:       row.Description,
			Category:          row.Category,
			Icon:              row.Icon,
			Tags:              types.JSONSlice(row.Tags),
			SupportedVersions: types.JSONSlice(versioning.Sort(row.SupportedVersions)),
			InputSchema:       types.JSONB("[]"),
			OutputSchema:      types.JSONB("[]"),
			IsFeatured:        row.IsFeatured,
			SortOrder:         row.SortOrder,
			Status:            row.Status,
		}
		if tool.Category == "" {
			tool.Category = domain.ToolCategoryQuery
		}
		if tool.Status == "" {
			tool.Status = domain.StatusPublished
		}

		isNew, err := isNewRecord(gormDB, &domain.Tool{}, "slug = ?", tool.Slug)
		if err != nil {
			s.fail(fmt.Sprintf("%s 预查失败: %v", tool.Slug, err))
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] %s tool %s (%s)\n", upsertVerb(isNew), tool.Slug, tool.Name)
			s.countOutcome(isNew)
			continue
		}

		if err := gormDB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "slug"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "summary", "description", "category", "icon", "tags", "supported_versions", "is_featured", "sort_order", "status", "updated_at"}),
		}).Create(tool).Error; err != nil {
			s.fail(fmt.Sprintf("%s 导入失败: %v", row.Slug, err))
			continue
		}
		s.countOutcome(isNew)
	}
	return s
}

func marshalRaw(items []json.RawMessage) []byte {
	b, _ := json.Marshal(items)
	return b
}

func marshalStringMap(m map[string]string) []byte {
	b, _ := json.Marshal(m)
	return b
}

// typesJSONSlice 解析 JSON 数组文本为 types.JSONB。
func typesJSONSlice(raw []string) types.JSONB {
	b, _ := json.Marshal(raw)
	return types.JSONB(b)
}

// typesMap 解析 JSON 对象文本为 types.JSONB。
func typesMap(raw []byte) types.JSONB {
	return types.JSONB(raw)
}

// typesIngredientSlice 解析 JSON 数组为 types.JSONB（配方材料）。
func typesIngredientSlice(raw []byte) types.JSONB {
	return types.JSONB(raw)
}

// mustJSONB 序列化任意值到 types.JSONB。
func mustJSONB(v any) types.JSONB {
	b, _ := json.Marshal(v)
	return types.JSONB(b)
}
