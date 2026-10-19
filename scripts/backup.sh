#!/usr/bin/env bash
# 每日数据库备份脚本。
# 用法: ./scripts/backup.sh
# 产物: backup/mc_tools_YYYYMMDD_HHMMSS.sql.gz
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "缺少 .env：请先 cp .env.example .env" >&2
  exit 1
fi
set -a; source .env; set +a

BACKUP_DIR="${BACKUP_DIR:-$ROOT/backup}"
mkdir -p "$BACKUP_DIR"

STAMP="$(date +%Y%m%d_%H%M%S)"
OUT="$BACKUP_DIR/mc_tools_${STAMP}.sql.gz"

# 从容器内执行 pg_dump，避免宿主机依赖 psql 客户端。
echo "== 备份数据库到 $OUT =="
docker compose -f deploy/compose.yaml exec -T postgres \
  pg_dump -U mc_user -d mc_tools --no-owner --no-acl | gzip > "$OUT"

echo "== 完成: $OUT =="
ls -lh "$OUT"