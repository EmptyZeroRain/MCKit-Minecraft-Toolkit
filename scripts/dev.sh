#!/usr/bin/env bash
# 开发环境一键启动：仅拉起 PostgreSQL，后端与前端在宿主机热更新运行。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 加载 .env（不存在则从示例复制）
if [[ ! -f .env ]]; then
  cp .env.example .env
  echo "已从 .env.example 生成 .env，请按需修改 SESSION_SECRET 等敏感配置"
fi
set -a; source .env; set +a

echo "== 启动 PostgreSQL =="
docker compose -f deploy/compose.yaml up -d postgres

echo "== 等待数据库就绪 =="
until docker compose -f deploy/compose.yaml exec postgres pg_isready -U mc_user -d mc_tools >/dev/null 2>&1; do
  sleep 1
done

echo "== 执行数据库迁移 =="
(cd backend && go run ./cmd/migrate)

echo "== 启动后端 (热重载提示：生产默认不自动) =="
echo "   后端:  go run ./cmd/api"
echo "   前端:  cd frontend && npm run dev"
echo "   浏览器: http://localhost:5173"