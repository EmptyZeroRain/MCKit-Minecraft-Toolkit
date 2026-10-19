// Package tagging 维护「内容对象 → 标签」的关联表，
// 为标签筛选与统计提供可靠、可索引的数据来源。
//
// 设计说明：
//   - 关联表是内容 `tags`（JSONB）字段的**派生索引**：JSONB 仍是接口输出的
//     数据来源（保持向后兼容），关联表仅服务于筛选与统计；
//   - 管理端保存内容时，在同一事务内调用 Sync 重建关联行，保证两者原子一致；
//   - 表名 / 列名全部来自包内白名单常量，不接受外部输入，避免拼接注入。
package tagging

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Target 关联表目标：关联表名、属主外键列、属主表名（均为白名单常量）。
type Target struct {
	table string
	col   string
	owner string
}

// 三个受支持的关联目标。
var (
	// ResourceTarget 资源标签。
	ResourceTarget = Target{table: "resource_tags", col: "resource_id", owner: "resources"}
	// ToolTarget 工具标签。
	ToolTarget = Target{table: "tool_tags", col: "tool_id", owner: "tools"}
	// WikiTarget 百科标签。
	WikiTarget = Target{table: "wiki_tags", col: "wiki_entry_id", owner: "wiki_entries"}
)

// maxTagLen 单个标签长度上限，防止超长脏数据进入索引。
const maxTagLen = 64

// normalize 清洗标签集合：去空白、去空串、截断超长项、去重（保持首次出现顺序）。
func normalize(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if len(t) > maxTagLen {
			t = t[:maxTagLen]
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// Sync 在事务内重建某内容对象的标签关联行（先删后插，幂等）。
// tx 必须与内容写入使用同一事务句柄，保证 JSONB 与关联表原子一致。
func Sync(tx *gorm.DB, target Target, ownerID string, tags []string) error {
	if tx == nil {
		return errors.New("tagging: 事务句柄为空")
	}
	if _, err := uuid.Parse(ownerID); err != nil {
		return fmt.Errorf("tagging: 属主 ID 非法: %w", err)
	}

	// 表名 / 列名来自白名单常量，此处拼接安全。
	if err := tx.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE %s = ?", target.table, target.col), ownerID,
	).Error; err != nil {
		return err
	}

	items := normalize(tags)
	if len(items) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(items))
	for _, t := range items {
		rows = append(rows, map[string]any{target.col: ownerID, "tag": t})
	}
	return tx.Table(target.table).Create(rows).Error
}

// ExistsClause 返回「按标签筛选」的 EXISTS 子句与绑定参数。
// 用法：q = q.Where(clause, tag)。
func (t Target) ExistsClause(tag string) (string, string) {
	return fmt.Sprintf(
		"EXISTS (SELECT 1 FROM %s WHERE %s = %s.id AND tag = ?)",
		t.table, t.col, t.owner,
	), tag
}

// UsageRow 单个标签的使用统计（跨资源 / 工具 / 百科）。
type UsageRow struct {
	Tag           string `json:"tag"`
	Total         int64  `json:"total"`
	ResourceCount int64  `json:"resource_count"`
	ToolCount     int64  `json:"tool_count"`
	WikiCount     int64  `json:"wiki_count"`
}

// Usage 汇总三个关联表中标签的使用次数（按总次数降序，limit<=0 时默认 200）。
func Usage(ctx context.Context, db *gorm.DB, limit int) ([]UsageRow, error) {
	if limit <= 0 {
		limit = 200
	}
	const sqlText = `
SELECT tag,
       COUNT(*)                                    AS total,
       COUNT(*) FILTER (WHERE kind = 'resource')   AS resource_count,
       COUNT(*) FILTER (WHERE kind = 'tool')       AS tool_count,
       COUNT(*) FILTER (WHERE kind = 'wiki')       AS wiki_count
FROM (
    SELECT tag, 'resource' AS kind FROM resource_tags
    UNION ALL SELECT tag, 'tool' FROM tool_tags
    UNION ALL SELECT tag, 'wiki' FROM wiki_tags
) t
GROUP BY tag
ORDER BY total DESC, tag ASC
LIMIT ?`
	rows := make([]UsageRow, 0, limit)
	if err := db.WithContext(ctx).Raw(sqlText, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
