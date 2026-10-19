#!/usr/bin/env bash
# 数据库恢复脚本。
# 用法: ./scripts/restore.sh <备份文件>
# 注意: 恢复将覆盖当前数据库，操作前请确认。被恢复库需已存在。
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "用法: $0 <备份文件(.sql.gz)>" >&2
  exit 1
fi
BACKUP_FILE="$1"
if [[ ! -f "$BACKUP_FILE" ]]; then
  echo "备份文件不存在: $BACKUP_FILE" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "缺少 .env" >&2
  exit 1
fi
set -a; source .env; set +a

echo "即将覆盖数据库 mc_tools，此操作不可撤销！"
read -rp "确认恢复？输入 yes 继续: " CONFIRM
if [[ "$CONFIRM" != "yes" ]]; then
  echo "已取消"
  exit 0
fi

echo "== 恢复 $BACKUP_FILE =="
gunzip -c "$BACKUP_FILE" | docker compose -f deploy/compose.yaml exec -T postgres \
  psql -U mc_user -d mc_tools

echo "== 恢复完成 =="