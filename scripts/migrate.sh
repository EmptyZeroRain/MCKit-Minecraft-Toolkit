#!/usr/bin/env bash
# 数据库迁移执行脚本：先备份，再迁移（gj.md §16.4 迁移规程）。
#
# 用法:
#   ./scripts/migrate.sh              # 备份后应用全部未执行的正向迁移
#   ./scripts/migrate.sh --to 27      # 备份后迁移到指定版本（可前进或回退）
#   ./scripts/migrate.sh --down 1     # 备份后回退最近 1 个迁移
#
# 说明:
#   * 默认强制先备份，避免不可逆迁移没有回退点；
#   * 本地开发可 SKIP_BACKUP=1 跳过（不推荐，尤其涉及 DROP/ALTER 的迁移）；
#   * 参数原样透传给 cmd/migrate。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "${SKIP_BACKUP:-0}" != "1" ]]; then
  echo "== 步骤 1/2：备份数据库 =="
  "$ROOT/scripts/backup.sh"
else
  echo "== 已跳过备份（SKIP_BACKUP=1） =="
fi

echo "== 步骤 2/2：执行迁移 =="
cd "$ROOT/backend"
go run ./cmd/migrate "$@"
