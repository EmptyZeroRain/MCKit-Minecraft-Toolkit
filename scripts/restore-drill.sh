#!/usr/bin/env bash
# 数据库恢复演练脚本（gj.md §16.4）。
#
# 作用：把最近一次备份恢复到一个**隔离的演练库**（默认 mc_tools_drill），
# 校验关键表与迁移版本后可选择保留，最后删除演练库。全程不触碰生产库 mc_tools。
#
# 用法:
#   ./scripts/restore-drill.sh                # 使用 backup/ 下最新的备份
#   ./scripts/restore-drill.sh <备份文件>      # 指定备份文件
#   KEEP_DRILL=1 ./scripts/restore-drill.sh   # 保留演练库便于人工检查
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "缺少 .env：请先 cp .env.example .env" >&2
  exit 1
fi
set -a; source .env; set +a

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backup}"
DRILL_DB="${DRILL_DB:-mc_tools_drill}"
KEEP_DRILL="${KEEP_DRILL:-0}"

if [[ $# -ge 1 ]]; then
  BACKUP_FILE="$1"
else
  BACKUP_FILE="$(ls -1t "$BACKUP_DIR"/mc_tools_*.sql.gz 2>/dev/null | head -n 1 || true)"
fi

if [[ -z "${BACKUP_FILE:-}" || ! -f "$BACKUP_FILE" ]]; then
  echo "找不到备份文件；请先执行 ./scripts/backup.sh 或显式传入备份路径" >&2
  exit 1
fi

psql_admin() {
  docker compose -f deploy/compose.yaml exec -T postgres \
    psql -U mc_user -d postgres -v ON_ERROR_STOP=1 "$@"
}

psql_drill() {
  docker compose -f deploy/compose.yaml exec -T postgres \
    psql -U mc_user -d "$DRILL_DB" -v ON_ERROR_STOP=1 "$@"
}

echo "== 恢复演练：$BACKUP_FILE -> $DRILL_DB =="

# 1) 重建演练库（仅当库名符合白名单时才允许 DROP，避免误删生产库）。
if [[ ! "$DRILL_DB" =~ ^mc_tools_(drill|restore_test)$ ]]; then
  echo "DRILL_DB 必须为 mc_tools_drill 或 mc_tools_restore_test，当前: $DRILL_DB" >&2
  exit 1
fi
psql_admin -c "DROP DATABASE IF EXISTS ${DRILL_DB}"
psql_admin -c "CREATE DATABASE ${DRILL_DB}"

# 2) 导入备份。
echo "== 导入备份 =="
gunzip -c "$BACKUP_FILE" | docker compose -f deploy/compose.yaml exec -T postgres \
  psql -U mc_user -d "$DRILL_DB" -v ON_ERROR_STOP=1 >/dev/null

# 3) 校验：迁移版本 + 关键业务表存在且可查询。
echo "== 校验 =="
VERSION="$(psql_drill -At -c "SELECT COALESCE(max(version), 0) FROM schema_migrations")"
echo "schema_migrations 最高版本: $VERSION"

for table in accounts roles resources tools wiki_pages audit_logs; do
  COUNT="$(psql_drill -At -c "SELECT count(*) FROM ${table}")"
  echo "  ${table}: ${COUNT} 行"
done

# 审计表与账号表是恢复后最关键的可用性指标：账号为 0 说明备份不完整。
ACCOUNTS="$(psql_drill -At -c "SELECT count(*) FROM accounts")"
if [[ "$ACCOUNTS" -eq 0 ]]; then
  echo "演练失败：恢复后 accounts 表为空，备份可能不完整" >&2
  exit 1
fi

echo "== 演练成功 =="

if [[ "$KEEP_DRILL" == "1" ]]; then
  echo "已保留演练库 ${DRILL_DB}（KEEP_DRILL=1）"
else
  psql_admin -c "DROP DATABASE IF EXISTS ${DRILL_DB}"
  echo "已删除演练库 ${DRILL_DB}"
fi
