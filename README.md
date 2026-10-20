# MC 工具站（mckit）

面向 Minecraft Java 版玩家、整合包玩家、服务器管理者和 Mod 开发者的**工具与资源导航站**。非官方社区项目，不暗示 Mojang 或 Microsoft 背书；不重新分发任何 Minecraft 代码或字节码。

技术栈：**Go 1.27 + Chi + GORM + PostgreSQL 16** · **Vue 3.5 + TypeScript + Vite 6 + Pinia + Tailwind CSS 4**

![首页](./images/首页.png)

## 核心能力

- **资料查询**：物品 ID、合成配方、药水与酿造、村民交易、生物资料（掉落 / 生成 / 驯服繁殖）、红石元件与机器设计、Mod 与整合包（依赖 / 冲突**仅呈现已记录项**，不做自动冲突分析）。
- **内置工具**：指令格式辅助、可视化指令生成器、附魔与装备计算器、坐标换算、MC 版本清单、物品 ID 对照表、**种子地图**、**种子与结构工具**、**存档诊断**。
- **开发栏目**：开发文档知识库（NeoForge / Forge / Fabric / 原版数据）+ **版本映射查询**（obf / mid / named 对照）+ **MC 源码浏览器**（按版本浏览官方 jar 的类索引、继承链、字节码、引用与浏览器内反编译）。
- **百科与资源**：中文 Wiki 条目抓取与浏览；资源导航（类型 / 版本 / 加载器筛选、来源跳转、失效链接检查）。
- **全站搜索**：跨 **工具 / 资源 / 百科 / 物品 / 开发文档** 统一检索，支持类型与版本筛选、相关度 / 最近更新排序与无结果引导。
- **用户与后台**：可选账号（收藏 / 最近使用 / 常用种子；公开工具免登录）；每个账号拥有随机生成的对外用户 ID（形如 `U7K3M2QX9F`，内部主键仍为 UUID）；**用户资源投稿**（结构化提交 → 人工审核 → 审核通过转为资源）；管理后台含内容 CRUD、投稿审核（红石 / Mod / 资源投稿 / 反馈）、链接健康复查、数据质量与导入历史、运营看板、审计日志。
- **用户成长**：服务端判定的经验 / 等级（仅展示）/ 每日任务 / 成就体系；由已确认的业务事件驱动，经验账本只增不删、支持审计与人工冲正。新用户默认 Lv.0「访客」，累计满 7 个活跃日后进入 Lv.1 经验等级；后台可按对外用户 ID **直接覆盖**单用户展示等级（不接受内部 UUID / 用户名，普通用户与后台账号均可），覆盖后经验继续累积但不会自动改级，并手动授予 / 撤销成就，按 `ops.growth` 权限管理。等级采用 Lv.1-Lv.9999 连续自动曲线（默认 `step_xp=100`，达到 Lv.N 所需经验 = `step_xp × N × (N-1) / 2`），后台可调整等级上限与每级递增值并一键重算；称号支持区间规则（如 Lv.1-10「初来乍到」）和指定等级规则（如 Lv.100「MCkit 传奇」），指定等级优先于区间，启用区间不能重叠。后台「用户详情」可按用户树状展开等级进度、活跃天数、成就解锁时间、经验流水、礼物币收支与成长审计日志。
- **礼物中心**：每日签到领取「签到礼物币」；礼物币分为**通用 / 签到**两类余额，后台按 `ops.gifts` 权限受控增减（支持内部 UUID / 对外用户 ID），任一余额不得为负，每次变动写入不可删除的账本并记录操作人。后台可维护礼物目录（名称、图标、价格、是否允许签到币兑换、排序、启停）与每日签到奖励，并在同一「礼物」页查看签到 / 礼物 / 礼物币排行榜；用户用礼物币兑换礼物并查看兑换记录与流水；已有兑换记录的礼物只能停用、不能删除。
- **玩家社区**：帖子 / 留言 / 点赞与分类筛选；公开读免登录，发帖 / 留言 / 点赞要求普通用户会话并走 CSRF；作者可用对外用户 ID 署名；后台按 `content.community` 权限隐藏 / 恢复 / 删除内容，并在「用户评论」中跨帖子检索全部评论。内容健康模块支持维护屏蔽关键字，发帖 / 评论命中启用规则时由服务端直接拒绝。

## Minecraft 能力与版本边界

站内所有 MC 算法均为**自研或移植实现**（cubiomes / Minecraft Wiki），不依赖第三方 MC 协议或世界生成库。能力与限制在服务端集中声明（`internal/mcstructure/capabilities.go`、`internal/mcbiome`），前端会显式提示，避免把只适用于部分版本的结果当成通用结论。

| 能力 | 适用版本 | 说明与已知限制 |
|---|---|---|
| 结构理论候选坐标 | **1.13+** | 基于 `random_spread`（spacing / separation + 随机偏移）；1.12 及更早算法不同，结果不可作为依据。部分结构有额外频率门槛（前哨站 20%、埋藏宝藏 1%、废弃矿井 0.4%） |
| 生物群系查询 | **1.18+** | Xoroshiro128++ + 改进 Perlin + 六维气候参数 + 决策树；取海平面附近地表群系。**当前仅内嵌 `btree18.h`**，1.19–1.21 会向下回退到 1.18 决策树（数据补齐后自动切换） |
| 要塞估算 | 1.9+（1.8 仅供参考） | 1.9+ 为 8 环共 128 个；环内估算误差约 ±112 格，精确落点需进游戏核实 |
| 史莱姆区块 | Java 版全版本 | 按 Java `Random` 与区块坐标派生种子判定（cubiomes 同款实现）；基岩版算法不同 |
| 种子搜索 | 全版本 | 概率性随机采样 + 理论候选坐标校验，未做群系可行性筛选，命中结果仍需进游戏确认 |
| 存档诊断 | Anvil 格式 | 只读解析 `.mca` / `level.dat`，输出区块清单、压缩统计与错误 / 警告；**不修改、不自动修复** |
| MC 源码浏览 | manifest 内含 client / server 下载项的版本 | 服务端下载并缓存官方 jar、构建类索引；浏览器内 Vineflower WASM 反编译（失败回退纯 JS 实现） |
| 版本映射 | 按导入数据集 | ProGuard / TSRG·TSRG2 / Tiny v1·v2；仓库不预置映射数据 |

> 版本区间：`1.8-1.12` / `1.13-1.15` / `1.16-1.17` / `1.18+`。所有种子以**字符串**返回，避免 64 位整数在 JavaScript 侧丢失精度。

## 界面预览

| 前台功能 | MC 源码浏览器 |
|---|---|
| <img src="./images/功能1.png" width="420" alt="前台功能"> | <img src="./images/代码浏览器.png" width="420" alt="MC 源码浏览器"> |

| 登录页（普通用户 / 管理员共用） | 后台总览 |
|---|---|
| <img src="./images/后台登陆.png" width="420" alt="登录页"> | <img src="./images/后台.png" width="420" alt="后台总览"> |

| 后台内容管理 | 运营看板 |
|---|---|
| <img src="./images/后台功能截图.png" width="420" alt="后台内容管理"> | <img src="./images/后台运营看板.png" width="420" alt="运营看板"> |

| 数据质量检查 | 操作日志 |
|---|---|
| <img src="./images/后题质量检查.png" width="420" alt="数据质量检查"> | <img src="./images/后台操作日志.png" width="420" alt="操作日志"> |

## 架构速览

```text
浏览器（Vue 3 SPA + Monaco + ECharts）
   │  POST /api/v1（JSON body；上传为 multipart；jar 下载返回二进制）
   ▼
Chi 路由 + 中间件
RequestID → Recover → AccessLog → Metrics → SecurityHeaders → CORS → RateLimit
   ├── 内容 / 账号 / 运营 API ── GORM ── PostgreSQL 16
   ├── MC 引擎（纯 Go，无外部 MC 库）
   │     mcbiome / mcrand / mcstructure / nbt / region / mappings
   └── 外部集成（Mojang manifest + piston 下载）── 磁盘 TTL 缓存 cache/
```

- **前端**：无 UI 组件库，自研组件 + Tailwind CSS 4；三套 Axios 实例（管理端 / 用户端 / 匿名统计），CSRF 双重提交、401 静默刷新与请求重放。
- **后端**：全站查询统一 `POST`；统一响应 `{"data": ..., "meta": ...}` / 错误 `{"error": {...}, "request_id": ...}`；迁移使用可回滚 SQL 文件，启动时默认自动应用。
- **缓存分层**：Mojang 元数据 7 天磁盘缓存；mcsrc jar/索引/meta 磁盘缓存 + 内存继承 / 引用索引；region 仅内存（默认 256MiB / 16 条 / 10 分钟）；决策树与气候 spline 进程内 `sync.Once`。
- **限流分层**：全站默认 10 rps；`/mc/seeds/find` 1 rps；重接口 3 rps + 并发 2；`/mcsrc/prepare` 并发 1。

## 目录结构

```text
.
├── backend/
│   ├── cmd/
│   │   ├── api/              # API 服务（含自动迁移、任务调度、告警评估）
│   │   ├── migrate/          # 数据库迁移（正向 / 回退 / 指定版本）
│   │   ├── seed/             # JSON 数据导入（11 类数据 + all，幂等）
│   │   ├── import-mappings/  # 映射文件导入（ProGuard / TSRG·TSRG2 / Tiny v1·v2）
│   │   ├── wikicrawl/        # 中文 Wiki 抓取（可选）
│   │   └── create-admin/     # 初始化管理员
│   ├── internal/
│   │   ├── http/             # handler / middleware / response
│   │   ├── service/          # 业务服务（内容 / 账号 / 成长 / 礼物 / 社区 / 搜索 / 告警…）
│   │   ├── repository/       # DB 与迁移执行器（advisory lock 串行化）
│   │   ├── domain/           # 领域模型与权限点
│   │   ├── mcbiome/          # 1.18+ 主世界生物群系采样（内嵌 tables/*.h）
│   │   ├── mcrand/           # Java Random（48 位 LCG）复刻
│   │   ├── mcstructure/      # 结构候选 / 要塞 / 史莱姆区块 / 种子搜索
│   │   ├── mcversion/        # Mojang 版本元数据客户端（磁盘缓存）
│   │   ├── nbt/              # NBT 解析（含 gzip，防压缩炸弹）
│   │   ├── region/           # Anvil .mca 解析与只读诊断
│   │   ├── mcsrc/            # 官方 jar 下载、类索引、继承 / 引用 / 字节码
│   │   ├── mappings/         # 映射格式解析
│   │   ├── blockids/         # 物品 ID 对照表解析
│   │   ├── builtintools/     # 内置工具 slug 单一来源（与前端对齐）
│   │   └── auth/ mfa/ captcha/ audit/ search/ linkcheck/ metrics/ cache/ config/
│   ├── migrations/           # 42 个版本 × (正向 + 反向) SQL
│   ├── seeds/                # 演示数据（items / potions / mobs / … / tools_builtin）
│   ├── data/物品id.md        # 物品对照表权威源
│   ├── openapi/openapi.yaml  # OpenAPI 3.1 完整规范
│   └── Dockerfile
├── frontend/
│   ├── src/
│   │   ├── api/              # 40 个文件：接口模块 + 三套 HTTP 实例
│   │   ├── mcsrc/            # 浏览器端源码浏览引擎（Worker / 反编译 / 跳转 / 备注）
│   │   ├── router/ stores/ composables/ layouts/ components/ types/ utils/
│   │   └── views/            # 47 个前台页面 + admin/ 49 个后台页面
│   ├── tests/                # 91 个 Vitest spec
│   ├── nginx.conf
│   └── Dockerfile
├── deploy/                   # compose.yaml（PG + API + FE + Caddy） + Caddyfile
├── scripts/                  # dev.sh / migrate.sh / backup.sh / restore.sh / restore-drill.sh
├── images/                   # README 截图
├── .env.example
└── README.md
```

> `mcsrc-main/` 是上游 [mcsrc.dev](https://mcsrc.dev/) 的参考副本，仅用于架构与算法借鉴，**未纳入版本控制、不参与本站构建**。本站源码浏览能力由 `frontend/src/mcsrc/` + `backend/internal/mcsrc/` 自研实现。

## 快速开始（开发环境）

### 前置要求

- **Go ≥ 1.27**（`backend/go.mod`）
- **Node.js ≥ 20**（推荐 22 / 24 LTS；Docker 构建使用 `node:24-alpine`）、npm ≥ 10
- **Docker + Docker Compose**（PostgreSQL 16）

### 步骤

```bash
# 1. 生成 .env 并启动 PostgreSQL
cp .env.example .env
# 编辑 .env，至少修改 SESSION_SECRET 为强随机值
openssl rand -hex 32   # 生成 SESSION_SECRET

docker compose -f deploy/compose.yaml --env-file .env up -d postgres

# 2.（可选）手动执行数据库迁移
#    API 启动时会默认自动应用未执行的迁移（AUTO_MIGRATE=true）；
#    手动执行便于提前发现失败，或在不启动服务时完成变更。
cd backend && go run ./cmd/migrate

# 2b. 迁移回滚（可选）
#     反向文件命名约定：<序号>_<名称>.sql 为正向，<序号>_<名称>.down.sql 为反向
go run ./cmd/migrate --down 1          # 回退最近 1 个版本
go run ./cmd/migrate --down 3          # 回退最近 3 个版本（按版本降序，逐版本单事务）
go run ./cmd/migrate --to 20           # 前进 / 回退到指定版本（自动判断方向）
go run ./cmd/migrate --dir migrations  # 显式指定迁移目录（默认 migrations）
#     --to 与 --down 互斥；缺少反向文件时明确报错停止，不会静默跳过

# 3. 初始化管理员（无默认密码，必须提供强密码）
ADMIN_USERNAME=admin ADMIN_PASSWORD='你的强密码' go run ./cmd/create-admin

# 4. 启动后端（:8080）
go run ./cmd/api

# 5. 启动前端（:5173，热更新）
cd ../frontend && npm install && npm run dev
```

浏览器访问 <http://localhost:5173>，API 基础地址 <http://localhost:8080/api/v1>。

前端开发代理由 `VITE_PROXY_TARGET`（默认 `http://localhost:8080`）控制，可选配置：

```bash
# frontend/.env
VITE_API_BASE_URL=/api/v1
VITE_PROXY_TARGET=http://localhost:8080
VITE_SITE_URL=http://localhost:5173
```

> `./scripts/dev.sh` 只负责**生成 .env、启动 PostgreSQL、执行迁移**，不会代启前后端；后端与前端仍需按上面第 4 / 5 步单独运行。

## 配置说明

完整示例见 [`.env.example`](./.env.example)。启动时会强校验：`DATABASE_URL`、`SESSION_SECRET`（≥16 字符）、`CORS_ALLOWED_ORIGINS`（非空且禁止 `*`）、`SESSION_STORE` 取值、`MFA_ENCRYPTION_KEY` 可解析、限流参数 > 0；生产环境设置 `CAPTCHA_FIXED_CODE` 会拒绝启动。

### 基础与数据库

| 变量 | 默认值 | 说明 |
|---|---|---|
| `APP_ENV` | `development` | `production` 时启用更严格校验 |
| `APP_ADDR` | `:8080` | API 监听地址 |
| `APP_BASE_URL` | `http://localhost:5173` | 站点对外地址（SEO / 链接） |
| `DATABASE_URL` | **必填** | PostgreSQL 连接串 |
| `AUTO_MIGRATE` | `true` | 启动时自动应用未执行迁移 |
| `MIGRATIONS_DIR` | `migrations` | 迁移目录（相对工作目录，自动向上查找） |
| `LOG_LEVEL` | `info` | `debug/info/warn/error` |
| `CORS_ALLOWED_ORIGINS` | **必填** | 逗号分隔，禁止 `*` |
| `CSRF_EXEMPT_ORIGINS` | 空 | CSRF 豁免来源（仅确有需要的第三方回调） |

### 会话、账号与认证

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SESSION_SECRET` | **必填** | 会话签名密钥，≥16 字符 |
| `SESSION_COOKIE_NAME` | `mc_session` | 管理员与普通用户共用 |
| `SESSION_SECURE_COOKIE` | `false` | 生产应设为 `true`（仅 HTTPS 发送 Cookie） |
| `SESSION_STORE` | `memory` | `memory`（单实例）/ `postgres`（多实例共享） |
| `SESSION_TTL_HOURS` | `72` | 会话绝对有效期 |
| `SESSION_IDLE_MINUTES` | `0` | 空闲超时；0 不启用 |
| `PASSWORD_MIN_LENGTH_ADMIN` | `12` | 后台账号最小密码长度 |
| `PASSWORD_MIN_LENGTH_USER` | `8` | 普通用户最小密码长度 |
| `PASSWORD_MAX_LENGTH` | `128` | 密码最大长度 |
| `MFA_ENCRYPTION_KEY` | 空 | AES-GCM 密钥（base64 / hex，16/24/32 字节）；留空表示不部署 MFA |
| `MFA_ISSUER` | `MC 工具站` | 认证器 App 展示名 |
| `CAPTCHA_ENABLED` | `true` | 注册图形验证码开关 |
| `CAPTCHA_TTL_SECONDS` | `300` | 验证码有效期 |
| `CAPTCHA_FIXED_CODE` | 空 | 仅自动化测试；生产设置会拒绝启动 |

### 限流与 MC 工具

| 变量 | 默认值 | 说明 |
|---|---|---|
| `RATE_LIMIT_RPS` | `10` | 全局限流速率 |
| `RATE_LIMIT_BURST` | `20` | 全局突发 |
| `MC_FIND_RPS` / `MC_FIND_BURST` | `1` / `2` | `/mc/seeds/find` 单独限流（0 可禁用） |
| `MC_HEAVY_RPS` / `MC_HEAVY_BURST` | `3` / `6` | 遍历 / 上传型 MC 接口限流 |
| `MC_HEAVY_CONCURRENCY` | `2` | 重接口同时执行上限（≤0 不限制） |
| `REGION_CACHE_MAX_MB` | `256` | `region_token` 内存缓存上限 |
| `REGION_CACHE_TTL_SECONDS` | `600` | `region_token` 有效期 |

### 缓存、元数据与源码库

| 变量 | 默认值 | 说明 |
|---|---|---|
| `MCMETA_CACHE_DIR` | `cache` | Mojang 元数据磁盘缓存目录 |
| `MCMETA_CACHE_TTL_HOURS` | `168` | 元数据缓存有效期（7 天） |
| `BLOCK_ID_FILE` | `data/物品id.md` | 物品对照表源文件 |
| `MCSRC_ENABLED` | `true` | 关闭后 `/mcsrc/*` 返回 503 |
| `MCSRC_CACHE_DIR` | `<MCMETA_CACHE_DIR>/mcsrc` | jar / 索引 / meta 缓存目录 |
| `MCSRC_JAR_MAX_MB` | `512` | 单 jar 体积上限 |
| `MCSRC_INDEX_MAX_CLASSES` | `60000` | 单 jar 类索引上限（超出截断） |
| `MCSRC_CACHE_ALERT_MB` | `4096` | 缓存占用告警阈值（`mcsrc.capacity`） |
| `MCSRC_AUTO_PREPARE_MAX` | `12` | 匿名访客自动准备版本数；0 关闭自动准备 |

### 统计与保留

| 变量 | 默认值 | 说明 |
|---|---|---|
| `STATS_ENABLED` | `true` | 匿名使用统计采集开关 |
| `STATS_FLUSH_SECONDS` | `60` | 内存聚合落库周期 |
| `STATS_RETENTION_DAYS` | `180` | 日聚合保留天数 |
| `JOB_RETENTION_DAYS` | `90` | 已结束后台任务保留天数 |
| `AUDIT_RETENTION_DAYS` | `180` | 审计日志保留天数 |
| `GROWTH_TIMEZONE` | `Asia/Shanghai` | 成长系统业务时区（IANA） |

### 命令行专用

| 变量 | 使用位置 | 说明 |
|---|---|---|
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `cmd/create-admin` | 必填；密码按后台策略校验 |
| `ADMIN_ROLE` | `cmd/create-admin` | 默认 `admin`，角色必须已存在 |
| `IMPORT_OPERATOR` | `cmd/seed` | 导入任务执行者；空则取系统用户名 |
| `TEST_DATABASE_URL` | 集成测试 | 测试数据库 |
| `MCSRC_LIVE_TEST` | mcsrc 真实上游测试 | 设为 `1` 才执行 |

## 数据导入

```bash
cd backend
# dry-run 预检查
go run ./cmd/seed --type items --file seeds/items_demo.json --dry-run
# 正式导入
go run ./cmd/seed --type items --file seeds/items_demo.json
go run ./cmd/seed --type recipes --file seeds/recipes_demo.json
go run ./cmd/seed --type commands --file seeds/commands_demo.json
# 资源（含发布版本）
go run ./cmd/seed --type resources --file seeds/resources_demo.json
# 进阶资料：药水与酿造 / 村民交易 / 生物资料 / 开发文档 / 红石资料 / Mod 兼容性
go run ./cmd/seed --type potions --file seeds/potions_demo.json
go run ./cmd/seed --type villagers --file seeds/villagers_demo.json
go run ./cmd/seed --type mobs --file seeds/mobs_demo.json
go run ./cmd/seed --type devdocs --file seeds/dev_docs_demo.json
go run ./cmd/seed --type redstone --file seeds/redstone_demo.json
go run ./cmd/seed --type mods --file seeds/mods_demo.json
# 统一入口：以 seeds 目录批量导入（按依赖顺序）
go run ./cmd/seed --type all --file seeds
```

导入顺序：`items → potions → villagers → mobs → redstone → mods → devdocs → recipes → commands → tools → resources`。

可用类型：`items` / `recipes` / `commands` / `tools` / `resources` / `potions` / `villagers` /
`mobs` / `devdocs` / `redstone` / `mods` / `all`。注意 `tools` 对应 `tools_builtin.json`，`devdocs` 对应 `dev_docs_demo.json`。

导入器按业务主键（`item_id` / `recipe_key` / `slug`）幂等更新，重复运行不产生重复记录；每行数据会做必填、枚举、数值与版本格式（含 `1.16-1.17`、`1.18+` 范围，以及 `all` 全版本通配）校验，失败逐行汇总；每次执行写入 `data_import_jobs`，可在后台 `审核与运营 → 内容健康 → 数据质量` 查看导入历史与数据质量概览。

资源导入额外校验**加载器枚举、依赖结构（`type ∈ dependency·prerequisite·conflict`）与发布版本结构**；`releases` 字段缺省表示「不触碰已有发布版本」，显式空数组表示「清空」。

资源、搜索与配方页的**版本筛选项动态取自官方版本清单**（`POST /api/v1/versions`，服务端带缓存）；拉取失败时回退内置兜底清单，保证筛选始终可用。声明 `all` 表示适用于全部 MC 版本（必须独占），详情页可展开完整正式版清单。

后台 `审核与运营 → 运营分析 → 搜索词统计` 可查看热门搜索、高频无结果词与查询失败词——统计为**匿名化聚合**（仅关键词与计数，不含 IP / 会话 / 用户标识）。

### 版本映射导入

`/api/v1/mappings/*` 与源码浏览器的「映射重命名」都读取数据库中的映射数据集。仓库不预置映射文件，需自行导入：

```bash
cd backend
go run ./cmd/import-mappings \
  --file ./client.txt --mc-version 1.21.1 --mapping-type mojang --channel mojang --format auto
# --mapping-type：mojang / srg / tsrg / yarn / parchment / mixed（必填）
# --format：auto / proguard / tsrg / tiny / tiny_v1 / tiny_v2（默认 auto）
# 其他参数：--build-label、--set-key、--source-url、--license、--note、--limit、--dry-run
```

同一 `--set-key` 会整体替换；批量 500 类 / 1000 成员，单事务提交。

## MC 源码浏览器

- 服务端按 `<version>-<side>.jar` 下载并缓存 Mojang 官方 jar，校验 SHA1 与大小后原子落盘，随后只读 zip 中央目录构建类索引（不解析字节码）。
- 同一 `version-side` 使用单飞锁，避免并发重复下载；匿名访客自动准备受 `MCSRC_AUTO_PREPARE_MAX` 限制，具备 `ops.mcsrc` 权限的管理员不受限。
- 前端首次选中版本会触发 `/mcsrc/prepare` 并轮询进度；jar 通过 `/mcsrc/jar` 下发到浏览器，由 Vineflower WASM 反编译，失败回退纯 JS 实现。
- 类结构、继承链、字节码、引用查找全部为纯 Go 字节码解析，不依赖 JVM；继承 / 引用索引在内存中按 jar 摘要缓存。
- 缓存是可重建的派生数据，后台「运营与系统 → MC 源码缓存」可查看占用、预览影响、清理单版本或清空，并写审计；占用达到 `MCSRC_CACHE_ALERT_MB` 会触发运营告警。

## 运行测试

```bash
# 后端（102 个 _test.go，含集成测试）
cd backend
go test ./...        # 单元 + 集成测试（集成测试需 TEST_DATABASE_URL）
go vet ./...         # 静态检查
go run ./cmd/api     # 本地运行

# 前端（91 个 spec，752 个用例，jsdom，无覆盖率阈值）
cd frontend
npm run typecheck    # vue-tsc 类型检查
npm run test         # Vitest 单次运行
npm run test:watch   # 监听模式
npm run build        # 类型检查 + 生产构建
```

测试重点：后台合并页与权限点、mcsrc 反编译 / 跳转 / token、种子与区域算法、Markdown XSS、分享链接敏感参数剥离、路由守卫。

## API 说明

### 方法约定（项目决策）

- **所有查询、搜索、列表接口统一使用 `POST`**，参数放在 JSON 请求体。
- 例外：`robots.txt` / `sitemap.xml` 保持 `GET`（搜索引擎协议要求）；`.mca` / `level.dat` 上传使用 `multipart/form-data`；`/mcsrc/jar` 为 POST 但返回二进制。
- 统一响应：成功 `{"data": ..., "meta": ...}`，错误 `{"error": {...}, "request_id": ...}`。
- 分页：`page` 从 1 开始，`page_size` 默认 20、最大 50。
- 写操作需 CSRF 双重提交（管理员与用户各自独立 token）；登录接口按 `(用户名, IP)` 限流。

### 公开内容

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/health` | 健康检查 |
| POST | `/api/v1/search` | 全站搜索（`q`、`type`、`game_version`、`sort`、`page`、`page_size`；`type` 取 `tool` / `resource` / `wiki` / `item` / `devdoc`） |
| POST | `/api/v1/tools`、`/tools/:slug` | 工具列表 / 详情 |
| POST | `/api/v1/resources`、`/resources/:slug` | 资源列表 / 详情（含发布版本与兼容性结论，无依据返回「未验证」） |
| POST | `/api/v1/items`、`/items/:item_id` | 物品查询 / 详情（`item_id` 含冒号） |
| POST | `/api/v1/recipes` | 配方查询（`item_id`、`game_version`、`recipe_type`） |
| POST | `/api/v1/commands`、`/commands/:slug` | 指令列表 / 详情 |
| POST | `/api/v1/wiki/:slug` | 百科详情 |
| POST | `/api/v1/potions`、`/potions/rules`、`/potions/:slug` | 药水与酿造 |
| POST | `/api/v1/villagers`、`/villagers/levels`、`/villagers/:slug` | 村民交易 |
| POST | `/api/v1/mobs`、`/mobs/:slug` | 生物资料 |
| POST | `/api/v1/redstone/components`、`/redstone/designs`（含 `/:slug`） | 红石元件与机器设计 |
| POST | `/api/v1/mods`、`/mods/:slug`、`/modpacks`、`/modpacks/:slug` | Mod 与整合包 |
| POST | `/api/v1/dev-docs`、`/dev-docs/facets`、`/dev-docs/:slug` | 开发文档知识库 |
| POST | `/api/v1/mappings/sets`、`/mappings/search`、`/mappings/class/:id` | 版本映射查询 |
| POST | `/api/v1/categories` | 分类列表 |
| POST | `/api/v1/feedback` | 提交反馈 / 举报 |
| POST | `/api/v1/stats/event` | 匿名使用统计上报（工具访问 / 使用 / 失败；登录用户的「使用」同时驱动成长事件） |
| GET | `/robots.txt`、`/sitemap.xml` | SEO |

### MC 工具与元数据

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/mc/structures` | 结构理论候选坐标（`seed`、`type`、`version`、`region_x/z`、`count_x/z`；1.18+ 附群系可行性 `feasible`） |
| POST | `/api/v1/mc/structures/types` | 支持的结构清单、参数描述与版本区间 |
| POST | `/api/v1/mc/capabilities` | 各能力的版本支持矩阵 |
| POST | `/api/v1/mc/biome` | 地表群系查询（仅 1.18+，否则 `supported=false`） |
| POST | `/api/v1/mc/strongholds` | 要塞估算（`count` 默认 3，最大 128） |
| POST | `/api/v1/mc/slime-chunks` | 史莱姆区块（跨度最大 128） |
| POST | `/api/v1/mc/seeds/generate` | 生成随机 Java 种子（1–100） |
| POST | `/api/v1/mc/seeds/find` | 按结构概率性搜索种子（独立限流 1 rps，5s 超时返回部分结果） |
| POST | `/api/v1/mc/seeds/parse` | 文本 / 数字种子互转 |
| POST | `/api/v1/seed/analyze` | 一次性分析：史莱姆区块 + 结构候选 + 要塞，可选群系底色（`radius` 最大 32） |
| POST | `/api/v1/mc/region/inspect` | 上传 `.mca`（≤64MiB），返回区块清单、统计与只读诊断；`region_token` 复用 |
| POST | `/api/v1/mc/region/chunk` | 按 `region_token` 或重新上传文件，读取单区块顶层 NBT |
| POST | `/api/v1/mc/level/parse` | 上传 `level.dat`，解析世界名、种子、版本、出生点、游戏规则 |
| POST | `/api/v1/versions`、`/versions/detail` | MC 版本清单 / 版本详情（下载地址、启动命令、Java 要求） |
| POST | `/api/v1/block-ids` | 方块 / 物品中文名与 ID 对照表 |

### MC 源码浏览器

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/mcsrc/status`、`/prepared` | 缓存 / 索引状态、已准备版本列表 |
| POST | `/api/v1/mcsrc/classes/list` | 类索引检索（`q`、`pkg`、分页；未索引返回 409 `NEED_PREPARE`） |
| POST | `/api/v1/mcsrc/classes/detail` | 类结构（修饰符、父类、接口、字段、方法） |
| POST | `/api/v1/mcsrc/classes/hierarchy` | 祖先链 + 直接 / 递归子类 |
| POST | `/api/v1/mcsrc/classes/bytecode` | 字节码反汇编文本 |
| POST | `/api/v1/mcsrc/classes/references` | 类 / 方法 / 字段引用查找 |
| POST | `/api/v1/mcsrc/classes/mapping` | 映射重命名（未导入映射时 `available=false`） |
| POST | `/api/v1/mcsrc/jar` | 下载已缓存 jar（二进制，返回 `X-Mcsrc-Jar-Sha1`） |
| POST | `/api/v1/mcsrc/prepare` | 下载 jar + 建索引（幂等；匿名受自动准备额度限制） |

### 用户、成长、礼物与社区

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/auth/login` | 统一登录（普通用户 / 后台账号共用；后台启用 MFA 时返回 `mfa_required` 票据） |
| POST | `/api/v1/user/captcha`、`/register`、`/login` | 注册验证码 / 注册 / 用户侧登录（保留兼容） |
| POST | `/api/v1/user/logout`、`/me`、`/profile`、`/change-password`、`/delete` | 会话与自助资料 |
| POST | `/api/v1/user/favorites/*`、`/recents/*`、`/seeds/*` | 收藏 / 最近使用 / 常用种子 |
| POST | `/api/v1/user/submissions/submit`、`/submissions/list` | 资源投稿 / 我的投稿 |
| POST | `/api/v1/growth/overview`、`/ledger`、`/daily-tasks`、`/daily-tasks/history`、`/achievements` | 成长中心（仅本人） |
| POST | `/api/v1/public/users/detail`、`/public/users/achievements` | 公开主页成长摘要与成就 |
| POST | `/api/v1/gifts/overview`、`/ledger/list`、`/catalog`、`/checkin`、`/redeem`、`/redemptions/list` | 礼物中心（写操作需 CSRF） |
| POST | `/api/v1/community/posts`、`/posts/detail`、`/comments` | 社区公开读（登录后附带 `liked`） |
| POST | `/api/v1/community/posts/create`、`/posts/delete`、`/posts/like`、`/comments/create`、`/comments/delete` | 社区写操作（登录 + CSRF） |

### 管理后台

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/admin/auth/login`、`/mfa/verify`、`/logout`、`/me`、`/change-password` | 后台认证（MFA 独立第二因子） |
| POST | `/api/v1/admin/auth/mfa/setup`、`/mfa/enable`、`/mfa/disable`、`/mfa/status`、`/profile`、`/sessions/*` | MFA 自助管理、资料、本人会话 |
| POST | `/api/v1/admin/accounts/*` | 统一账号管理（列表 / 创建 / 详情 / 更新 / 重置密码 / 删除 / 导出 / 解绑 MFA） |
| POST | `/api/v1/admin/admins/*`、`/admin/users/*` | 旧管理员 / 普通用户接口（保留兼容） |
| POST | `/api/v1/admin/roles/*` | 角色与权限点（`roles.manage`） |
| POST | `/api/v1/admin/sessions/list`、`/revoke` | 全站会话管理（`ops.sessions`） |
| POST | `/api/v1/admin/resources`、`/tools`、`/wiki` 等 | 内容 CRUD（登录 + CSRF） |
| POST | `/api/v1/admin/resources/releases/list`、`/releases/replace` | 资源发布版本整体替换（幂等） |
| POST | `/api/v1/admin/submissions/list`、`/detail`、`/review` | 资源投稿审核（`content.resources`） |
| POST | `/api/v1/admin/community/*` | 社区审核（`content.community`） |
| POST | `/api/v1/admin/content-health/keywords/*` | 关键字屏蔽维护（`ops.dataquality`） |
| POST | `/api/v1/admin/data-quality/overview`、`/data-import-jobs/list` | 数据质量与导入任务 |
| POST | `/api/v1/admin/link-health/*` | 链接健康复查 |
| POST | `/api/v1/admin/growth/*`、`/daily-tasks/*`、`/achievements/*` | 成长运营（`ops.growth`） |
| POST | `/api/v1/admin/gifts/*` | 礼物运营（`ops.gifts`） |
| POST | `/api/v1/admin/mcsrc/cache/list`、`/preview`、`/purge`、`/purge-all`、`/rebuild` | 源码缓存运维（`ops.mcsrc`） |
| POST | `/api/v1/admin/analytics/*`、`/search-terms/list` | 运营看板 / 搜索词统计 |
| POST | `/api/v1/admin/jobs/*`、`/alerts/*` | 后台任务 / 运营告警 |
| POST | `/api/v1/admin/audit-logs` | 审计日志（`ops.audit`） |

完整规范见 [`backend/openapi/openapi.yaml`](./backend/openapi/openapi.yaml)（OpenAPI 3.1，约 120 个后台操作 + 全部公开端点）。

### 请求示例

```bash
# 结构候选坐标
curl -s -X POST http://localhost:8080/api/v1/mc/structures \
  -H "Content-Type: application/json" \
  -d '{"seed":"-4172144997902289642","type":"village","version":"1.18+","region_x":0,"region_z":0,"count_x":4,"count_z":4}'

# 群系查询（仅 1.18+）
curl -s -X POST http://localhost:8080/api/v1/mc/biome \
  -H "Content-Type: application/json" \
  -d '{"seed":"12345","x":0,"z":0,"version":"1.18+"}'

# 上传区域文件做只读诊断
curl -s -X POST http://localhost:8080/api/v1/mc/region/inspect \
  -F "file=@r.0.0.mca"
```

## 部署（Docker Compose）

```bash
# 生产环境变量（务必修改所有敏感项）
cp .env.example .env
# SESSION_SECRET 用强随机值；SESSION_SECURE_COOKIE=true；DATABASE_URL 指向 compose 内服务
# 多实例部署时 SESSION_STORE=postgres

# 构建并启动（前端、后端、Caddy、PostgreSQL）
docker compose -f deploy/compose.yaml --env-file .env up --build -d

# （可选）显式执行迁移，便于在服务启动前确认变更结果
docker compose -f deploy/compose.yaml exec backend /usr/local/bin/migrate

# 初始化管理员
docker compose -f deploy/compose.yaml exec -e ADMIN_USERNAME=admin -e ADMIN_PASSWORD=xxx backend /usr/local/bin/create-admin
```

- 镜像只打包 `api / migrate / seed / create-admin` 四个二进制；`import-mappings` 与 `wikicrawl` 需在源码环境运行。
- PostgreSQL 端口仅绑定 `127.0.0.1:5432`，默认库 `mc_tools`，数据卷 `pg_data`。
- `deploy/Caddyfile` 是 **HTTP 反代模板**（`:80`）：`/api/*` → `backend:8080`，其余 → `frontend:80`，上传超时 120s。生产环境请替换为实际域名、配置 HTTPS 并持久化 Caddy 证书。
- compose 当前未给 backend / frontend 配置 healthcheck，依赖关系仅保证启动顺序。

## 备份与恢复

```bash
# 备份（每日 cron 建议）：backup/mc_tools_YYYYMMDD_HHMMSS.sql.gz
./scripts/backup.sh

# 恢复（交互确认，会覆盖现有库）
./scripts/restore.sh <备份文件>

# 带自动备份的迁移（默认先备份）
./scripts/migrate.sh              # 应用全部未执行迁移
./scripts/migrate.sh --to 27      # 迁移到指定版本
./scripts/migrate.sh --down 1     # 回退最近 1 个版本
SKIP_BACKUP=1 ./scripts/migrate.sh  # 本地开发跳过备份（不推荐）

# 恢复演练（默认取最新备份，使用 mc_tools_drill 库）
./scripts/restore-drill.sh
```

备份文件应加密并限制访问；定期做恢复演练。`restore.sh` 与 `migrate.sh` 都有破坏性操作，执行前请确认目标库。

## 安全说明

- 密码 Argon2id 加盐哈希存储；登录失败限流（5 次 / 15 分钟，IP 隔离）；注册图形验证码仅内存存储。
- 后台账号管理基于**权限点**：`accounts.admins` / `accounts.users` / `roles.manage` / `content.*` / `review.*` / `ops.*`；内置 `admin` 角色拥有全部权限点。普通用户可被提升为后台角色，提升后必须修改密码才能进入后台。
- 统一登录入口 `/api/v1/auth/login` 按账号角色分流：普通用户走用户登录链路，后台账号走后台认证链路（MFA、登录审计与权限点不变）；旧 `/user/login` 与 `/admin/auth/login` 保留兼容。后台账号登录后同样可使用前台个人能力（收藏 / 投稿 / 成长 / 礼物）。
- 改密、停用、改角色、重置密码、删除后，该账号所有会话立即失效；后台账号不可自助注销；MFA 密钥使用 AES-GCM 加密存储。
- 系统管理提供全站会话管理（`ops.sessions`）：可按指纹踢下线，列表不回显 SessionID 原文，当前会话禁止自踢，操作写审计。
- Cookie 会话 HttpOnly + SameSite；生产应开启 `SESSION_SECURE_COOKIE`。写操作 CSRF 双重提交校验。
- 链接检查内置 SSRF 防护（拒绝内网 / 回环 / 云元数据，重定向逐跳校验）。
- 上传解析限制：`.mca` / `level.dat` ≤64MiB；NBT 递归 ≤64 层、解压 ≤16MiB；单区块解压 ≤64MiB；映射解压 ≤512MiB。
- 全部输入后端校验，SQL 参数化；Markdown 渲染禁用原始 HTML。
- 审计日志不记录密码、Cookie、Authorization 头、数据库连接串。

## 故障排查

| 现象 | 检查 |
|---|---|
| 启动报 `缺少必需配置 DATABASE_URL` | 是否已 `cp .env.example .env` 并 source；migrate 需在 backend 目录运行 |
| 启动报 `SESSION_SECRET` / `CORS_ALLOWED_ORIGINS` | 密钥 ≥16 字符；CORS 不能为空或 `*` |
| 数据库连接失败 | `docker compose -f deploy/compose.yaml ps` 确认 postgres 健康 |
| 登录 401 | 确认 `create-admin` 已执行且密码符合策略（后台 ≥12 位） |
| 后台登录要求 MFA 但未配置密钥 | 补 `MFA_ENCRYPTION_KEY`，或由管理员执行「重置 MFA」 |
| CSRF 403 | 前端无 `X-CSRF-Token` 头，或 token 与会话 / 端口不符 |
| 前端访问接口 405 | 确认使用 POST（全站查询为 POST） |
| `/mcsrc/*` 返回 409 `NEED_PREPARE` | 该版本尚未建索引，先调用 `/mcsrc/prepare` 或在页面选中版本触发 |
| `/mcsrc/prepare` 返回 429 `AUTO_PREPARE_LIMIT` | 匿名自动准备达到 `MCSRC_AUTO_PREPARE_MAX`，由管理员准备或调大额度 |
| 源码缓存占满磁盘 | 后台「MC 源码缓存」清理，或调整 `MCSRC_CACHE_ALERT_MB` / 挂载更大卷 |
| `/mc/region/chunk` 返回 422 | `region_token` 已过期（`REGION_CACHE_TTL_SECONDS`）或无效，重新 inspect |
| MC 工具结果为「未支持」 | 版本超出能力边界（如 1.17 查群系、1.12 查结构），见能力矩阵 |
| 迁移文件改动不生效 | 迁移按版本号记录；新改动要新增序号文件，勿改已应用的 |
| 启动报 `数据库自动迁移失败` | 检查数据库账号是否有 DDL 权限、`MIGRATIONS_DIR` 是否指向正确的迁移目录 |
| 启动报 `未找到迁移目录` | 在 `backend` 目录启动，或通过 `MIGRATIONS_DIR` 指定绝对路径 |
| 不需要启动时自动迁移 | 设置 `AUTO_MIGRATE=false`，改为发布流程中显式执行 `cmd/migrate` |

## 决策记录

1. 查询接口全站 POST（用户确认）；SEO `robots.txt` / `sitemap.xml` 例外保持 GET；文件上传为 multipart 例外。
2. ORM 使用 GORM，数据库迁移仍用 SQL 文件（可回滚），不使用纯 AutoMigrate；API 启动时自动执行待应用 SQL 迁移（`AUTO_MIGRATE=false` 可关闭），多实例启动由 PostgreSQL advisory lock 串行化。
3. 会话默认内存存储（单实例）；多实例部署必须切换 `SESSION_STORE=postgres`，否则各实例登录状态互不相通。
4. MC 世界生成、NBT、Region、class 文件解析全部自研 / 移植，不引入第三方 MC 运行时库，保证可审计与跨平台。
5. 演示数据为自建整理（数据版本 demo-2026-01），来源标注在数据文件内。
6. 仓库不预置映射数据与 MC jar：受版权与分发限制，由使用者自行导入 / 服务端按需从 Mojang 下载。
7. 后台入口按「合并页 + 权限点」组织（如账号 / 审核 / 内容健康 / 运营），旧路径保留 18 条重定向兼容书签。

## 版本记录

- **v0.3.0（开发中）**：MC 工具引擎（种子地图 / 结构候选 / 要塞 / 史莱姆区块 / 1.18+ 群系 / 区域与 level.dat 只读诊断）、MC 源码浏览器（jar 缓存 + 类索引 + 继承 / 引用 / 字节码 + 浏览器内反编译）、版本映射查询、用户成长与礼物中心、玩家社区、统一账号与权限点、MFA 与全站会话管理、源码缓存运维与容量告警。
- **v0.2.0**：进阶资料（药水与酿造 / 村民交易 / 生物资料 / 红石与机器设计 / Mod 与整合包）、开发栏目（开发文档知识库 + 版本映射查询）、用户可选账号与工作台、使用统计与运营看板、投稿与纠错、链接健康检查、分享与易用性；开发文档接入全站搜索。
- **v0.1.0**：工程骨架、认证、后台 CRUD、公开 API、搜索、数据导入、SEO、部署配置。

## 许可

代码以 [MIT License](./LICENSE) 发布。站内抓取的中文 Minecraft Wiki 内容遵循 **CC BY-NC-SA 3.0**，并在记录中保留来源 URL 与许可信息；Minecraft 相关名称与内容版权归 Mojang Studios / Microsoft 所有。

本项目为社区非官方项目，与 Mojang 或 Microsoft 无任何隶属或背书关系。
