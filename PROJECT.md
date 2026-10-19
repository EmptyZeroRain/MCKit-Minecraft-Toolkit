# MC 工具站 · 项目文档（PROJECT）

> 本文档由 `开发需求.md`、`PROGRESS.md`、`BACKEND_TODO.md`、`todo.md` 四份文件**整合**而成。
> 原始文件已归档至 `bak/`，本文件为**项目总览与需求规格**的事实来源；**功能与验收状态以 `Function.md` 为准，待办与进度勾选以 `todo.md` 为准**。
> 整合日期：2026-10-14 ｜ **最近同步：2026-10-19**
> 本次同步补齐：§9-B 村民交易、§9-C 生物资料、§13–§23 全部交付与验收、迁移 `0012`–`0022`、内置工具 19 项、管理端内容审核页面（红石 / Mod）、开发文档接入全站搜索（`type=devdoc`）、后台角色与权限底座（`roles` + 17 个权限点）、统一账号体系（迁移 `0022`）、MC 源码浏览器切片 1 + 2 + 3-D1 + 3-D2（含浏览器内 Vineflower WASM 反编译与源码内跳转）。
> 需求版本：1.0 ｜ 技术栈：Go 1.27 + Chi + GORM + PostgreSQL 16 · Vue 3 + TypeScript + Vite

## 阅读指引

| 部分 | 内容 | 用途 |
|---|---|---|
| 第一部分 | 项目总览 | 技术栈、目录、决策记录、环境 |
| 第二部分 | 需求规格（原 `开发需求.md`） | 完整需求与验收依据 §0–§16 |
| 第三部分 | 开发进度（原 `PROGRESS.md`） | 阶段完成情况 |
| 第四部分 | 未完成事项 TODO（原 `todo.md`） | 整合时的待办快照；**实时待办以根目录 `todo.md` 为准** |
| 第五部分 | 历史待办归档（原 `BACKEND_TODO.md`） | 已修复条目存档，避免重复开发 |

---

# 第一部分 · 项目总览

## 1.1 项目简介

面向 Minecraft Java 版玩家、整合包玩家、服务器管理者和 Mod 开发者的**工具与资源导航站**。非官方社区项目，不暗示 Mojang 或 Microsoft 背书。

## 1.2 技术栈

| 层级 | 选型 |
|---|---|
| 前端 | Vue 3 + TypeScript + Vite（Composition API / `<script setup>`） |
| UI | Tailwind CSS + 自建组件 |
| 路由 / 状态 / 请求 | Vue Router（懒加载） / Pinia / Axios |
| 后端 | Go 1.27 + Chi + GORM |
| 数据库 | PostgreSQL 16+（SQL 迁移文件，可回滚） |
| API 文档 | OpenAPI 3.1（`backend/openapi/openapi.yaml`） |
| 测试 | Go `testing` / 前端 Vitest |
| 部署 | Docker Compose + Caddy / Nginx |

## 1.3 目录结构

```text
.
├── backend/               # Go 后端
│   ├── cmd/               # api / migrate / seed / import-mappings / create-admin / wikicrawl
│   ├── internal/          # config/http/domain/service/auth/search/linkcheck/audit/metrics/analytics/builtintools/mappings/versioning/mcsrc/mc*
│   ├── migrations/        # SQL 迁移文件（0001_basics … 0021_roles，共 21 个）
│   ├── seeds/             # 演示数据（items/recipes/potions/villagers/mobs/redstone/mods/dev_docs/commands/tools_builtin/resources）
│   ├── openapi/           # OpenAPI 规范
│   └── Dockerfile
├── frontend/              # Vue 3 前端（src/api|components|views|stores|utils|types）
├── deploy/                # compose.yaml + Caddyfile
├── scripts/               # dev.sh / backup.sh / restore.sh
├── bak/                   # 已归档的历史文档
├── .env.example
├── PROJECT.md             # 本文档
└── README.md
```

## 1.4 已确认的关键决策记录

1. **查询接口全站 POST**（用户确认）：所有列表/搜索/详情查询统一 `POST`，参数放 JSON body；详情页 slug 保留在路径。例外：`robots.txt` / `sitemap.xml` 保持 `GET`（爬虫协议要求），健康检查为 `POST`。
2. **ORM 使用 GORM**（用户确认）；数据库迁移仍用 SQL 文件（需求 §7 要求可回滚），不使用纯 AutoMigrate。
3. **会话 MVP 用内存存储**（单实例）；多实例部署需替换为共享存储。
4. **JSONB 数组字段**：资源/工具/百科等用 GORM string 序列化处理。
5. **演示数据为自建整理**，来源标注在数据文件内；数据版本按来源区分（如 `demo-2026-01` / `wiki-2026-10` / `curated-2026-10`），随条目 `source_url` 与 `last_verified_at` 一并记录。
6. **版本口径**：药水 / 村民 / 生物 / 开发文档等采用「声明式版本集合 + `versioning.Satisfies` 精确判断」（如 `1.8+`）；工具 / 资源沿用「声明文本匹配 + 通配 `all`」语义，两类语义在 `Function.md` 分别说明。
7. **内容审核**：红石设计、Mod / 整合包等投稿类内容默认 `pending`，**仅 `published` 对公开接口可见**，审核人取自登录会话并写入审计日志。

## 1.5 环境信息

- PostgreSQL 16（Docker compose）`127.0.0.1:5432`，db=`mc_tools`，user=`mc_user`
- 后端 `:8080`，前端 `:5173`；测试管理员 `admin`

## 1.6 当前验证结果（2026-10-19 实跑）

| 检查项 | 命令 | 结果 |
|---|---|---|
| 后端静态检查 | `cd backend && go vet ./...` | ✅ 通过 |
| 后端测试 | `cd backend && go test ./...`（含 `TEST_DATABASE_URL` 集成测试） | ✅ 全绿（并行包运行，连续 2 次）；本批顺带修掉两处**预存在**的测试隔离缺陷（见 §4.0.7） |
| 前端类型检查 | `cd frontend && npx vue-tsc --noEmit` | ✅ 通过 |
| 前端单测 | `cd frontend && npx vitest run` | ✅ **57 文件 / 536 项全绿** |
| 前端构建 | `cd frontend && npx vite build` | ✅ 通过 |
| 数据库容器 | `docker compose -f deploy/compose.yaml ps` | ✅ postgres healthy |
| 数据库迁移 | `cd backend && go run ./cmd/migrate`（全新库） | ✅ 22 个迁移全部应用，43 张表 |
| 备份恢复 | `scripts/backup.sh` → 独立库恢复 | ✅ 14 张核心表行数逐一比对一致 |
| MC 源码库真实链路 | `MCSRC_LIVE_TEST=1 go test ./internal/mcsrc/ -run TestLivePrepareRealJar` | ✅ 1.8.9 客户端 jar（8,461,484 字节）sha1 校验通过、索引 2507 个类、检索命中、二次准备不重复下载 |
| MC 源码库 jar 下发 + 反编译真实链路 | 临时程序（真实 jar 的 `GET /mcsrc/jar` + Node 24 跑 `@run-slicer/vf`） | ✅ 200 且 sha1 与缓存摘要一致、`Range: bytes=0-3` → 206、`If-None-Match` → 304、未缓存 → 404；真实反编译 `net.minecraft.server.MinecraftServer` 成功（约 1 s，产出真实 Java 源码） |
| MC 源码库 token / 跳转真实链路 | 临时脚本（真实 1.8.9 jar + Node 24 真实 Vineflower 纯 JS 运行时，逐段复刻 Worker 的 token 收集与类名对齐） | ✅ `MinecraftServer` 产出 1493 个 token（class 309 / method 450 / field 311 / parameter 156 / local 267）、抽样区间文本与源码一致、越界 0、`inJar` 与 jar 条目完全一致（1231 个可跳转）、类名无残留斜杠、内部类引用对齐 `$` 形式（`adp.a` → `adp$a`） |

**结论**：代码可编译、测试可通过，无阻断级缺陷；功能与验收状态见 `Function.md §九`，剩余缺口见第四部分。

---

# 第二部分 · 需求规格

> 原文件：`开发需求.md` v1.0 ｜ 项目类型：Minecraft Java 版工具与资源导航网站
> 首期原则：先交付可运行、可维护的 MVP；资源只做索引和原始来源跳转，不托管第三方下载文件。

## 0. 开发约定

### 0.1 首期默认决策

为避免开发过程中反复等待确认，首期采用以下默认方案：

1. **游戏版本范围**：首期以 Minecraft Java 版为主；数据模型保留 `game_edition`，方便以后扩展基岩版。
2. **资源策略**：只保存资源元数据和作者/平台原始链接，不代理、不镜像、不托管第三方文件。
3. **用户体系**：公开工具免登录可用；自 §12 起提供**可选**的普通用户账号（收藏 / 最近使用 / 常用种子服务端持久化、跨设备同步），未登录时收藏与最近使用仍保存在浏览器本地。
4. **管理后台**：必须有管理员登录、角色校验、操作日志和资源/工具/百科 CRUD。
5. **搜索**：首期使用 PostgreSQL 全文检索及标题/别名匹配，不额外引入搜索服务。
6. **合成数据**：通过可重复执行的数据导入程序导入；不得把配方硬编码在 Vue 页面中。
7. **部署**：Docker Compose 启动前后端、PostgreSQL 和反向代理；开发环境支持本地热更新。
8. **语言**：界面简体中文；数据字段和 API 使用稳定的英文命名。
9. **品牌合规**：网站声明为非官方社区项目，不暗示 Mojang 或 Microsoft 背书。

### 0.2 交付要求

开发完成必须提供：

- 可运行的前端、后端和数据库迁移文件。
- `.env.example`，不得提交真实密钥。
- 初始化管理员的安全流程或命令。
- 数据导入示例和至少一份演示数据。
- OpenAPI/Swagger API 文档。
- Docker Compose 部署配置、备份与恢复说明。
- 单元测试、关键 API 集成测试和上线验收清单。
- README：本地开发、测试、构建、部署、故障排查。

### 0.3 禁止事项

- 不得把管理员密码、JWT 密钥或数据库密码写入代码仓库。
- 不得在前端判断用户是否有管理员权限来替代后端鉴权。
- 不得执行任意用户输入的 SQL、Shell 或模板代码。
- 不得直接抓取或复制其他网站的受版权保护内容。
- 不得在没有明确授权的情况下托管第三方资源文件。
- 不得让资源 URL 检查接口访问任意内网地址，必须防 SSRF。

## 1. 产品目标与 MVP 范围

### 1.1 产品目标

建设一个面向 Minecraft Java 版玩家、整合包玩家、服务器管理者和 Mod 开发者的工具与资源导航站，集中提供常用工具、配方查询、资源检索和基础资料。

### 1.2 首期必须实现（P0）

1. 首页：全站搜索、快捷工具、资源分类、最近更新。
2. 工具中心：分类、搜索、标签、工具详情。
3. 至少 4 个可用工具：合成配方查询、坐标换算、物品 ID 查询、指令格式辅助。
4. 资源中心：资源列表、详情、类型/版本/加载器筛选、来源跳转。
5. 管理后台：管理员登录和退出；资源/工具/百科 CRUD；分类和标签维护；资源链接检查；操作日志。
6. 全站搜索：工具、资源、百科、物品、开发文档。
7. 响应式页面、SEO 基础、错误页、隐私与版权声明。
8. Docker Compose 部署、数据库迁移、日志、备份说明。

### 1.3 暂不纳入首期

- 普通用户注册、云端收藏、评论、论坛、投稿工作流。
- 第三方文件上传或下载代理。
- 复杂推荐算法、独立搜索集群。
- 实时多人功能。
- 原生移动端应用。

## 2. 技术架构

### 2.1 技术选型

| 层级 | 选型 | 约束 |
|---|---|---|
| 前端 | Vue 3 + TypeScript + Vite | Composition API，优先 `<script setup>` |
| UI | Tailwind CSS + 自建组件 | 统一设计 token，避免页面各自定义样式 |
| 路由 | Vue Router | 路由级懒加载 |
| 状态 | Pinia | 仅存放跨页面共享状态 |
| 请求 | Axios 或原生 fetch（二选一） | 统一错误处理、超时和鉴权 |
| 后端 | Go（建议当前稳定版） | 分层结构，`context.Context` 贯穿请求 |
| HTTP | Chi 或 Gin（二选一，默认 Chi） | 不混用多个 Web 框架 |
| 数据库 | PostgreSQL 16+ | 所有变更通过迁移文件 |
| ORM/SQL | sqlc + pgx，或 GORM（二选一，默认 pgx + sqlc） | 复杂查询可使用显式 SQL |
| 校验 | Go 后端参数校验 | 后端校验为最终依据 |
| API 文档 | OpenAPI 3.1 | 与接口实现同步 |
| 测试 | Go `testing`、前端 Vitest | 核心逻辑必须有测试 |
| 部署 | Docker Compose + Caddy/Nginx | HTTPS 由部署环境负责 |

### 2.2 架构原则

- Vue 只负责展示和交互；业务规则、权限、数据校验在 Go 后端完成。
- 所有数据通过 `/api/v1` 访问。
- 数据库访问不得散落在 HTTP Handler 中。
- 公开 API 与管理 API 分组，管理 API 必须进行服务端鉴权。
- 公共查询接口可缓存；管理接口和含敏感信息的接口禁止公共缓存。
- API 返回统一 JSON 格式。
- 时间统一存储 UTC，接口使用 RFC 3339；页面按浏览器本地时区展示。
- ID 使用 UUID；URL 友好页面使用唯一 `slug`。

### 2.3 建议仓库结构

```text
mc-tool-site/
├── frontend/
│   ├── src/{api,assets,components,layouts,router,stores,types,utils,views}/
│   ├── public/
│   ├── tests/
│   ├── Dockerfile
│   └── package.json
├── backend/
│   ├── cmd/{api,migrate}/
│   ├── internal/{config,http/{handler,middleware,response},domain,service,repository,auth,search,linkcheck,audit}/
│   ├── migrations/
│   ├── seeds/
│   ├── openapi/
│   ├── Dockerfile
│   └── go.mod
├── deploy/{compose.yaml,Caddyfile}
├── scripts/{dev.sh,backup.sh,restore.sh}
├── .env.example
├── .gitignore
└── README.md
```

## 3. 页面与路由

### 3.1 公共页面

| 路由 | 页面 | 核心功能 |
|---|---|---|
| `/` | 首页 | 全站搜索、快捷工具、资源分类、最近更新 |
| `/tools` | 工具中心 | 分类筛选、关键词搜索、工具卡片 |
| `/tools/:slug` | 工具详情 | 输入、输出、复制、清空、示例、说明 |
| `/recipes` | 合成查询 | 物品搜索、版本筛选、配方展示 |
| `/resources` | 资源中心 | 类型、版本、加载器、关键词筛选 |
| `/resources/:slug` | 资源详情 | 介绍、兼容信息、依赖、安装说明、原始来源 |
| `/items` | 物品 ID 查询 | 中文名、英文名、ID 搜索 |
| `/commands` | 指令格式辅助 | 命令模板、参数说明、示例 |
| `/potions`、`/potions/:slug` | 药水与酿造 | 效果与时长（普通/延长/增强）、酿造步骤链、通用规则、来源核验 |
| `/dev`、`/dev/:slug` | 开发文档 | 开发知识库检索（名称/平台/主题/版本）、要点正文、关键接口速查、官方文档与可核验来源、同平台相关条目；已接入全站搜索（`type=devdoc`） |
| `/mappings` | 版本映射查询 | 类/方法/字段的 obf / mid / named 三列名字对照，数据集选择与成员明细；未导入数据集时展示导入指引 |
| `/mcsrc` | MC 源码浏览器 | 按版本浏览官方 jar 的类索引：版本与侧别（客户端/服务端）选择、缓存状态、类名/包名检索、类结构与继承关系、**浏览器内反编译源码**（Vineflower WASM，无需服务端 JVM）、**源码内跳转**（Ctrl/⌘ 点击 / F12 跳转定义，字段与方法沿继承链定位声明类）；jar 由服务端统一下载缓存（管理员触发「准备该版本」） |
| `/coordinate` | 坐标换算 | 主世界/下界坐标换算 |
| `/wiki/:slug` | 百科详情 | 文章、目录、来源、更新时间 |
| `/about` | 关于本站 | 非官方声明、项目介绍 |
| `/privacy` | 隐私政策 | 本地存储、账号数据与保留期限、日志说明 |
| `/login` | 登录 / 注册（可选账号） | 普通用户登录与注册（不登录也可用全部公开工具） |
| `/workspace` | 我的工作台 | 收藏、最近使用、常用种子、改密与删除账号（需登录） |
| `/copyright` | 版权与侵权联系 | 版权声明和处理入口 |
| `/feedback` | 反馈 | 问题反馈、失效链接举报 |
| `/:pathMatch(.*)*` | 404 | 返回首页、搜索入口 |

### 3.2 管理页面

管理路由统一使用 `/admin` 前缀：

- `/admin/login`
- `/admin`
- `/admin/resources`、`/admin/resources/new`、`/admin/resources/:id/edit`
- `/admin/tools`、`/admin/tools/new`、`/admin/tools/:id/edit`
- `/admin/wiki`、`/admin/wiki/new`、`/admin/wiki/:id/edit`
- `/admin/categories`、`/admin/tags`、`/admin/feedback`、`/admin/audit-logs`
- `/admin/admins`、`/admin/data-quality`、`/admin/search-terms`、`/admin/analytics`
- `/admin/link-health`、`/admin/redstone`、`/admin/mods`（内容审核与链接健康）
- `/admin/roles`（角色与权限点管理）、`/admin/users`（普通用户管理）
- `/workspace`（普通用户个人工作台）

路由守卫只用于改善前端体验，真正的访问控制必须在后端完成。管理侧边栏入口按**权限点**过滤（登录 / `me` 返回 `permissions`）。

## 4. UI/UX 规范

### 4.1 视觉方向

- 风格：Minecraft 方块世界氛围 + 现代开发者工具站。
- 主背景：深海军蓝/炭黑；内容区使用高对比卡片。
- 主操作色：草地绿；警告使用橙色；错误使用红色。
- 避免大面积无效装饰、过度模仿游戏 HUD、影响可读性的像素字体。
- 正文使用易读的系统字体；像素风只用于少量标题或装饰。
- 桌面端优先，但必须适配手机和平板。

### 4.2 布局要求

- 桌面：顶部导航、居中内容区、工具页可采用侧栏 + 主工作区。
- 移动端：导航折叠为菜单；筛选项使用抽屉或折叠面板。
- 列表页支持加载、空结果、请求失败、无权限等状态。
- 所有可交互控件提供 hover、focus、disabled、loading 状态。
- 表单必须有可见标签；键盘可操作；不能只用颜色表达状态。
- 复制成功后给出轻量提示；复制失败时允许手动选中内容。

### 4.3 通用组件

至少实现：

- `AppHeader`、`AppFooter`、`MobileNav`
- `GlobalSearch`、`SearchResults`
- `PageContainer`、`Breadcrumbs`
- `ToolCard`、`ResourceCard`、`WikiCard`
- `VersionBadge`、`LoaderBadge`、`ResourceTypeBadge`
- `FilterPanel`、`Pagination`
- `EmptyState`、`ErrorState`、`LoadingSkeleton`
- `CopyButton`、`FavoriteButton`、`ShareButton`
- `AdminSidebar`、`AdminTable`、`ConfirmDialog`
- `FormField`、`Toast`、`StatusBadge`

### 4.4 状态与可访问性规范（批次 §14 落地）

**四种界面状态**（禁止只渲染空表格或裸文案）：

| 状态 | 组件 / 约定 | 可访问性语义 |
|---|---|---|
| 加载 | 列表用 `LoadingSkeleton`；分栏内局部加载用文案 | 骨架屏作为**单一** `role="status" aria-live="polite" aria-busy` 区域，占位块 `aria-hidden="true"`；局部加载文案带 `role="status" aria-live="polite"` |
| 空 | `EmptyState`（标题 + 说明 + 可选操作） | 文本可读，不做纯图标表达 |
| 错误 | `ErrorState`（可重试）或局部错误块 | 统一 `role="alert"`；字段级错误由 `FormField` 就近渲染并带 `role="alert"` |
| 降级 | 允许部分能力不可用（如种子地图后端失败仅保留本地史莱姆区块） | 显式文案说明降级原因，不静默失败 |

**可访问性与移动端**：

- 全局 `:focus-visible` 焦点环；顶部导航与后台侧边栏提供「跳到主要内容」跳转链接（`.skip-link`），`<main>` 带 `id` 与 `tabindex="-1"`。
- 表头 `<th>` 统一 `scope="col"`；宽表置于 `overflow-x-auto` 容器内（如物品列表、区域区块表）。
- 长文本：`.code-block` 设 `overflow-wrap: anywhere`，flex 行内代码块加 `min-w-0`，避免指令 / 长 ID / 启动命令在窄屏撑破布局。
- 数字输入：`<input type="number">` 上的 `v-model` 会被 Vue 自动转为 `number`，因此**状态一律按字符串读取**（`utils/share.ts` 的 `asText` / `coordinate` 的归一化 / `utils/command-generator.ts` 的 `asText`），避免 `.trim()` 在数字上抛错。

### 4.5 分享与稳定链接规范（批次 §14 落地）

- 交互状态只把**非默认值**写入 `route.query`；「默认状态」等价于「无查询参数」，链接可读、可回退。
- 分享链接**不含任何会话秘密**：Cookie 为 HttpOnly 且从不进入 URL；复制前用 `sanitizeShareUrl` 再剥离疑似机密参数（`token`/`csrf`/`session`/`cookie`/`password`/`authorization`/`api_key`/`signature` 等）与 hash 中的同类片段。
- 路由参数统一经 `queryString` / `queryNumber` / `queryBool` 读取，非法值回退默认值（含版本、结构类型等枚举校验），避免坏链接引发后端 422。
- 分享入口统一为 `ShareButton`（默认复制当前页链接）；表单配置类工具（指令生成器、附魔计算器）以「复制产出」为分享方式。

## 5. 功能详细需求

### 5.1 全站搜索

**输入**：`q`，可选 `type`、`game_version`、`page`、`page_size`。

**搜索范围**：工具、资源、百科、物品、开发文档（`devdoc`，命中标题 / 英文标题 / slug / 摘要 / 正文 / 标签，仅返回已发布条目）。

**要求**：

- 标题优先，其次别名、摘要和正文。
- 支持中文、英文、物品 ID 和 slug。
- 结果按相关度排序，标题和摘要展示关键词高亮。
- 空关键词不执行全库模糊扫描。
- 搜索结果按类型分组或通过类型筛选。
- 空结果展示建议：检查拼写、清除筛选或浏览分类。
- 限制 `page_size` 最大值为 50。
- 首期使用 PostgreSQL `tsvector`/`tsquery` 与必要的 `ILIKE` 别名匹配；建立相应索引。

### 5.2 工具中心

工具分类：查询类、计算类、生成类、编辑类、辅助类。

工具卡片字段：

- 名称、slug、简述、图标、分类、标签、支持版本、是否推荐。
- 支持关键词、分类、版本筛选。
- 收藏和最近使用仅保存在浏览器本地。

工具详情通用行为：

- 输入区、输出区、复制输出、清空输入、填入示例。
- 输入校验失败时在对应字段附近显示错误原因。
- 工具无需服务端计算时尽量在浏览器本地运行。
- 任何来自 URL、剪贴板或用户输入的文本均按不可信内容处理，不执行为 HTML/脚本。

### 5.3 坐标换算工具

功能：

- 主世界坐标换算为下界坐标：X、Z 除以 8。
- 下界坐标换算为主世界坐标：X、Z 乘以 8。
- Y 坐标原样显示，不参与 8 倍换算。
- 支持小数输入；明确展示原始坐标与换算结果。
- 可选展示取整后的建议坐标，但不得覆盖精确计算结果。
- 输入必须是有效数值；空值、NaN、无穷值均报错。
- 页面说明：坐标换算是理论对应关系，实际传送门链接会受生成位置、空间和游戏机制影响。

验收示例：主世界 `(800, 70, -240)` 对应下界理论坐标 `(100, 70, -30)`。

### 5.4 物品 ID 查询

- 支持按中文名、英文名、标准 ID、别名搜索。
- 显示中文名、英文名、标准 namespaced ID、适用版本、简短说明。
- 一键复制 ID。
- 数据来源必须记录来源和数据版本。
- 未收录物品显示空状态，不伪造查询结果。
- ID 示例采用 `minecraft:stone` 格式；数据模型允许模组命名空间。

### 5.5 合成配方查询

页面功能：

- 搜索中文名、英文名、物品 ID。
- 按 Minecraft 版本筛选。
- 展示配方网格、材料、产出数量、配方类型及工作台要求。
- 对无序合成、有序合成、熔炼/烧炼预留不同配方类型。
- 支持从配方材料跳转到物品详情或再次查询。
- 无结果时提供相似名称或建议移除版本筛选。
- 配方数据必须来自有许可的数据源或自建整理的数据；记录来源、版本和导入时间。

配方类型枚举：`shaped`、`shapeless`、`smelting`、`blasting`、`smoking`、`campfire_cooking`、`stonecutting`、`smithing`、`other`。

建议配方数据结构：

```json
{
  "recipe_type": "shaped",
  "game_version": "1.21.1",
  "result_item_id": "minecraft:crafting_table",
  "result_count": 1,
  "pattern": ["##", "##"],
  "key": { "#": "minecraft:oak_planks" },
  "ingredients": [{ "item_id": "minecraft:oak_planks", "count": 4 }],
  "requires_crafting_table": true,
  "source_url": "https://example.com/source"
}
```

以上为结构示例；导入器应校验字段，实际数据必须来自合法来源。

### 5.6 指令格式辅助

- 按命令名称、用途和标签检索。
- 展示语法、参数解释、权限/执行环境说明、示例和版本差异。
- 一键复制命令模板。
- 模板中的占位符要清晰，例如 `<玩家名>`、`<数量>`。
- 不在页面中自动执行命令，也不连接用户服务器。
- 版本差异必须基于可验证资料；未确认时标记“待核实”。

### 5.7 资源中心

资源类型：`mod`、`modpack`、`resource_pack`、`shader`、`world`、`datapack`、`plugin`。

筛选条件：关键词、资源类型、游戏版本、加载器、来源平台、更新时间、状态（公开页面只显示已发布资源）。

加载器枚举：`forge`、`neoforge`、`fabric`、`quilt`、`vanilla`、`paper`、`spigot`、`purpur`、`other`。

资源卡片展示：封面、名称、摘要、作者、资源类型、支持版本、加载器、来源平台、更新时间。封面缺失时使用统一占位图。不展示未经核实的下载量、评分或安全背书。

资源详情展示：完整介绍、适用版本、加载器、依赖、安装步骤、作者、许可证、原始发布页。显示“前往原始来源”链接，并在外链旁标明外部网站。资源失效、待审核或已下架时不应继续出现在公开列表。任何“安全”状态必须有实际检查依据；默认只标注“来源链接已检查”，不等于文件安全。

### 5.8 资源 URL 检查与 SSRF 防护

后台链接检查功能必须：

1. 仅允许 `http`、`https` 协议。
2. 拒绝用户名/密码形式的 URL。
3. 解析 DNS 后拒绝回环、私有、链路本地、保留地址和云元数据地址。
4. 每次重定向后重新校验目标主机和解析 IP；限制重定向次数（建议最多 3 次）。
5. 设置连接、响应头和整体超时；限制响应体大小，不下载完整文件。
6. 不跟随非 HTTP(S) 重定向。
7. 禁止访问 localhost、内网域名、容器服务名和云元数据地址。
8. 对检查频率限流，并记录操作者、时间、URL、HTTP 状态和结果。
9. 只检查链接可达性，不宣称完成恶意文件扫描。
10. 链接检查失败不能直接删除资源，只更新检查状态并允许人工复核。

### 5.9 收藏与最近使用

- 首期通过 `localStorage` 保存收藏的工具/资源 ID 和最近使用工具。
- 保存结构需带 schema 版本；解析失败时安全重置，不导致页面崩溃。
- 提供清除本地数据的入口。
- 不在本地存储认证令牌、管理员密码或敏感信息。
- 后续如增加账号，可在独立迁移方案中将本地收藏合并到云端。

### 5.10 反馈与举报

公开表单字段：

- 类型：错误反馈、资源失效、侵权通知、其他。
- 标题、内容、关联资源 URL（可选）、联系方式（可选）。
- 服务端进行长度校验、限流、反垃圾处理。
- 默认不要求登录。
- 管理后台支持查看、标记处理中/已解决/忽略，并记录操作日志。
- 对联系方式等个人信息仅按处理反馈所需范围收集。

## 6. 后台管理需求

### 6.1 身份认证

- 首期仅管理员账号。
- 密码使用 Argon2id 或 bcrypt 安全哈希，不保存明文。
- 登录失败限流；记录失败次数和时间，防止暴力破解。
- 推荐采用服务端会话 + HttpOnly、Secure、SameSite Cookie；若使用 JWT，必须明确过期、刷新、撤销和存储策略，不得把长期令牌放在 `localStorage`。
- 写操作必须防 CSRF（采用 Cookie 会话时）。
- 生产环境强制 HTTPS。
- 管理员支持退出登录和会话失效。
- 提供初始化管理员命令，从环境变量或交互式终端读取密码；不得提供固定默认密码。
- MFA 可作为上线增强项；如支持，必须可配置并有恢复流程。

### 6.2 权限

首期至少两个角色：

- `admin`：所有管理操作。
- `editor`：资源、工具、百科内容编辑；不可管理管理员账号和安全配置。

如果首期只实现 `admin`，数据模型仍保留角色字段。

所有管理 API 必须服务端验证登录状态、角色和资源状态。前端隐藏按钮不能作为权限控制。

> **实现现状（2026-10-19）**：已超出首期要求——迁移 `0021_roles` 引入 `roles` 表（`name` 主键 + `permissions` JSONB + `is_system`）与 **17 个权限点 / 4 组**，`admins.role` 外键引用角色，路由以 `RequirePermission(roleSvc, <权限点>)` 校验；内置 `admin`（全量、不可削权）/ `editor`（内容管理）/ `user`（占位），并支持自定义角色。详见 `Function.md §4.7 / §4.10`、`todo.md §23`。

### 6.3 CRUD 规范

- 创建时校验必填字段、slug 唯一性、URL 协议、枚举值。
- 更新时只允许修改白名单字段。
- 删除优先采用软删除或状态下架，避免误删。
- 列表必须分页，默认每页 20 条，最大 100 条。
- 支持按名称、状态、类型、更新时间筛选。
- 保存成功后显示反馈；失败时保留已填写内容并显示明确错误。
- 重要操作写入审计日志。

### 6.4 审计日志

至少记录：操作者 ID、动作、对象类型、对象 ID；变更前/后的必要字段摘要；请求时间、请求 ID、来源 IP（按隐私政策处理）；结果成功/失败。

不得记录密码、会话 Cookie、Authorization 头、数据库连接串或完整敏感请求体。

## 7. 数据库设计

所有表均使用 UUID 主键，时间字段使用 `timestamptz`。迁移文件必须可重复部署，并提供回滚策略。

### 7.1 `admins`

- `id UUID PK`
- `username VARCHAR(80) UNIQUE NOT NULL`
- `password_hash TEXT NOT NULL`
- `role VARCHAR(20) NOT NULL`：**外键引用 `roles.name`**（迁移 `0021_roles` 由 CHECK 约束改为外键，使自定义角色可立即分配）；内置 `admin` / `editor` / `user`
- `is_active BOOLEAN NOT NULL DEFAULT true`
- `last_login_at TIMESTAMPTZ NULL`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

### 7.2 `resources`

- `id UUID PK`
- `slug VARCHAR(180) UNIQUE NOT NULL`
- `name VARCHAR(200) NOT NULL`
- `summary VARCHAR(500) NOT NULL`
- `description TEXT NOT NULL DEFAULT ''`
- `resource_type VARCHAR(40) NOT NULL`
- `cover_image_url TEXT NULL`
- `author_name VARCHAR(160) NULL`
- `source_platform VARCHAR(100) NULL`
- `source_url TEXT NOT NULL`
- `license VARCHAR(160) NULL`
- `game_edition VARCHAR(20) NOT NULL DEFAULT 'java'`
- `supported_versions JSONB NOT NULL DEFAULT '[]'`
- `loader_types JSONB NOT NULL DEFAULT '[]'`
- `dependencies JSONB NOT NULL DEFAULT '[]'`
- `install_guide TEXT NOT NULL DEFAULT ''`
- `tags JSONB NOT NULL DEFAULT '[]'`
- `status VARCHAR(20) NOT NULL DEFAULT 'draft'`：`draft` / `published` / `hidden` / `removed`
- `link_status VARCHAR(20) NOT NULL DEFAULT 'unchecked'`：`unchecked` / `ok` / `redirected` / `broken` / `blocked` / `error`
- `last_checked_at TIMESTAMPTZ NULL`
- `published_at TIMESTAMPTZ NULL`
- `created_by UUID NULL REFERENCES admins(id)`
- `updated_by UUID NULL REFERENCES admins(id)`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`
- `deleted_at TIMESTAMPTZ NULL`

为 `status`、`resource_type`、`updated_at` 建立索引；公开查询只返回 `published` 且未软删除的记录。

### 7.3 `tools`

- `id UUID PK`
- `slug VARCHAR(180) UNIQUE NOT NULL`
- `name VARCHAR(160) NOT NULL`
- `summary VARCHAR(500) NOT NULL`
- `description TEXT NOT NULL DEFAULT ''`
- `category VARCHAR(40) NOT NULL`
- `icon VARCHAR(80) NULL`
- `tags JSONB NOT NULL DEFAULT '[]'`
- `supported_versions JSONB NOT NULL DEFAULT '[]'`
- `input_schema JSONB NOT NULL DEFAULT '{}'`
- `output_schema JSONB NOT NULL DEFAULT '{}'`
- `is_featured BOOLEAN NOT NULL DEFAULT false`
- `sort_order INT NOT NULL DEFAULT 0`
- `status VARCHAR(20) NOT NULL DEFAULT 'published'`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

工具的实际计算逻辑在前端/后端代码中注册，不允许后台保存并执行任意脚本。

### 7.4 `wiki_entries`

- `id UUID PK`
- `slug VARCHAR(180) UNIQUE NOT NULL`
- `title VARCHAR(240) NOT NULL`
- `summary VARCHAR(500) NOT NULL DEFAULT ''`
- `content TEXT NOT NULL`
- `content_format VARCHAR(20) NOT NULL DEFAULT 'markdown'`
- `content_type VARCHAR(40) NOT NULL`
- `game_edition VARCHAR(20) NOT NULL DEFAULT 'java'`
- `game_versions JSONB NOT NULL DEFAULT '[]'`
- `tags JSONB NOT NULL DEFAULT '[]'`
- `source_references JSONB NOT NULL DEFAULT '[]'`
- `status VARCHAR(20) NOT NULL DEFAULT 'draft'`
- `published_at TIMESTAMPTZ NULL`
- `created_by UUID NULL REFERENCES admins(id)`
- `updated_by UUID NULL REFERENCES admins(id)`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`
- `deleted_at TIMESTAMPTZ NULL`

Markdown 渲染必须禁用原始 HTML，或经严格净化后再展示。

### 7.5 `items`

- `id UUID PK`
- `item_id VARCHAR(200) UNIQUE NOT NULL`，例如 `minecraft:stone`
- `name_zh VARCHAR(200) NOT NULL`
- `name_en VARCHAR(200) NULL`
- `aliases JSONB NOT NULL DEFAULT '[]'`
- `description TEXT NOT NULL DEFAULT ''`
- `stack_size INT NULL`
- `game_edition VARCHAR(20) NOT NULL DEFAULT 'java'`
- `supported_versions JSONB NOT NULL DEFAULT '[]'`
- `source_url TEXT NULL`
- `data_version VARCHAR(80) NULL`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

### 7.6 `recipes`

- `id UUID PK`
- `recipe_key VARCHAR(240) UNIQUE NOT NULL`
- `recipe_type VARCHAR(40) NOT NULL`
- `game_version VARCHAR(40) NOT NULL`
- `result_item_id VARCHAR(200) NOT NULL REFERENCES items(item_id)`
- `result_count INT NOT NULL DEFAULT 1`
- `pattern JSONB NOT NULL DEFAULT '[]'`
- `key_mapping JSONB NOT NULL DEFAULT '{}'`
- `ingredients JSONB NOT NULL DEFAULT '[]'`
- `requires_crafting_table BOOLEAN NOT NULL DEFAULT false`
- `source_url TEXT NULL`
- `data_version VARCHAR(80) NULL`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

注意：引用 `items(item_id)` 要求该字段唯一；如果所用数据库/迁移工具对该外键处理复杂，可改为引用 `items.id` 并在 API 层使用 `item_id`。

### 7.7 `commands`

- `id UUID PK`
- `slug VARCHAR(180) UNIQUE NOT NULL`
- `name VARCHAR(160) NOT NULL`
- `syntax TEXT NOT NULL`
- `description TEXT NOT NULL`
- `parameters JSONB NOT NULL DEFAULT '[]'`
- `examples JSONB NOT NULL DEFAULT '[]'`
- `permission_notes TEXT NOT NULL DEFAULT ''`
- `game_versions JSONB NOT NULL DEFAULT '[]'`
- `source_references JSONB NOT NULL DEFAULT '[]'`
- `status VARCHAR(20) NOT NULL DEFAULT 'published'`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

### 7.8 `categories`、`tags`、关联关系

- `categories`: `id`、`name`、`slug`、`content_type`、`sort_order`、`created_at`、`updated_at`
- `tags`: `id`、`name`、`slug`、`created_at`
- 如需可靠的关联筛选，使用 `resource_tags`、`tool_tags`、`wiki_tags` 关联表；不要长期依赖 JSONB 标签实现所有关系查询。
- 首期可在 JSONB 保存版本和加载器数组，但必须验证每个元素为允许的值。

### 7.9 `feedback`

- `id UUID PK`
- `feedback_type VARCHAR(40) NOT NULL`
- `title VARCHAR(200) NOT NULL`
- `content TEXT NOT NULL`
- `related_url TEXT NULL`
- `contact TEXT NULL`
- `status VARCHAR(20) NOT NULL DEFAULT 'new'`
- `created_at TIMESTAMPTZ NOT NULL`
- `updated_at TIMESTAMPTZ NOT NULL`

### 7.10 `audit_logs`

- `id UUID PK`
- `actor_admin_id UUID NULL REFERENCES admins(id)`
- `action VARCHAR(80) NOT NULL`
- `object_type VARCHAR(80) NOT NULL`
- `object_id UUID NULL`
- `before_summary JSONB NULL`
- `after_summary JSONB NULL`
- `request_id VARCHAR(100) NULL`
- `source_ip INET NULL`
- `result VARCHAR(20) NOT NULL`
- `created_at TIMESTAMPTZ NOT NULL`

### 7.11 搜索与索引

- 为资源、工具、百科、物品建立标题/名称索引。
- PostgreSQL 全文检索字段在插入/更新时同步维护，或通过生成列/触发器维护。
- 对 `slug`、`status`、`resource_type`、`updated_at`、`item_id` 建立必要索引。
- JSONB 字段按实际查询建立 GIN 索引，避免无目的地为所有字段建索引。
- 搜索结果不得包含草稿、下架或软删除内容。

### 7.12 普通用户表（`users` / `user_favorites` / `user_recents` / `user_seeds`）

可选账号体系的表，与 `admins` **完全分离**（普通用户不获得任何后台权限）：

- `users`：`id UUID PK`、`username VARCHAR(40) UNIQUE NOT NULL`、`password_hash TEXT NOT NULL`（Argon2id）、`is_active`、`last_login_at`、`created_at`、`updated_at`。
- `user_favorites`：`id`、`user_id UUID REFERENCES users(id) ON DELETE CASCADE`、`kind`（`tool`/`item`/`resource`/`wiki`）、`object_id`、`label`、`url`、`created_at`；`UNIQUE (user_id, kind, object_id)`。
- `user_recents`：同 `user_favorites` 结构（保留最近 50 条），时间字段为 `visited_at`（由服务端写入）。
- `user_seeds`：`name`、`seed VARCHAR(64)`（**字符串**，避免 int64 精度问题）、`note`、`created_at`、`updated_at`。
- 三张个人数据表均通过外键 `ON DELETE CASCADE` 级联，删除账号即清除全部个人数据。

### 7.13 使用统计表（`tool_stats_daily` / `resource_stats_daily` / `api_stats_daily` / `search_stats_daily`）

运营看板（TODO §13）的数据来源。设计原则是**按天聚合、只存计数、不存个人数据**：

- 四张表**同构**：`id`、业务维度键、`day DATE`，维度键 + `day` 唯一；其余字段均为 `BIGINT` 计数（默认 0）。
  - `tool_stats_daily`：`slug`（内置工具或 `tools` 表 slug）+ `visits` / `uses` / `failures`。
  - `resource_stats_daily`：`slug` + `views`。
  - `search_stats_daily`：`queries` / `zero_result` / `failed`（全站汇总，无维度键）。
  - `api_stats_daily`：`route`（**chi 路由模板**，如 `/api/v1/resources/{slug}`；未匹配归并 `<unmatched>`）+ `method` + `total` / `errors`(5xx) / `client_errors`(4xx 不含 429) / `rate_limited`(429) / `sum_ms` / `max_ms` + 延迟分桶计数 `bucket_50` / `bucket_200` / `bucket_500` / `bucket_1000` / `bucket_3000` / `bucket_over`。
- **写入**：内存聚合 + 按周期（`STATS_FLUSH_SECONDS`）批量 `INSERT ... ON CONFLICT ... DO UPDATE SET x = x + EXCLUDED.x` 累加，避免逐请求写库；进程优雅退出时强制刷盘。
- **保留期限**：`STATS_RETENTION_DAYS`（默认 180），后台按 `day < 截止日` 删除。
- **延迟口径**：分桶近似，P95 取桶上界（保守值），不存逐次耗时明细。
- **隐私**：不存 IP / 会话 / 用户标识 / UA / `request_id` / 请求体 / 完整查询内容。

## 8. API 规范

### 8.1 通用约定

- 前缀：`/api/v1`
- 请求/响应：JSON，编码 UTF-8。
- 分页：`page` 从 1 开始，`page_size` 默认 20，最大 50（后台列表可最大 100）。
- 统一成功结构：

```json
{ "data": {}, "meta": {} }
```

- 统一错误结构：

```json
{
  "error": { "code": "VALIDATION_ERROR", "message": "请求参数不合法", "details": [] },
  "request_id": "..."
}
```

- HTTP 状态码：`200` 查询/更新成功，`201` 创建成功，`204` 删除/退出成功，`400` 参数错误，`401` 未登录，`403` 无权限，`404` 不存在，`409` 冲突，`422` 业务校验失败，`429` 频率超限，`500` 未知服务端错误。
- **查询方法约定**：经项目决策确认，全站查询、搜索、列表接口统一使用 `POST`，查询/筛选/分页参数一律放在 JSON 请求体内（`q`、`type`、`game_version`、`loader`、`sort`、`page`、`page_size` 等），不用 URL query 传递。路径中的 `:slug`、`:id`、`:item_id` 等资源标识仍保留在路径上。
- **方法例外**：`robots.txt` 与 `sitemap.xml` 保持 `GET`（搜索引擎爬虫协议强制要求，见 §9.3）；健康检查使用 `POST`。
- 错误响应不得泄露堆栈、SQL、文件路径或密钥。
- 所有写请求进行字段白名单校验。

### 8.2 公开 API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/health` | 健康检查，不暴露内部配置 |
| POST | `/api/v1/search` | 全站搜索（q 等参数放 body；类型含工具 / 资源 / 百科 / 物品 / 开发文档） |
| POST | `/api/v1/tools` | 工具列表，支持分类、关键词、分页 |
| POST | `/api/v1/tools/:slug` | 工具详情 |
| POST | `/api/v1/resources` | 资源列表，支持类型、版本、加载器、分页 |
| POST | `/api/v1/resources/:slug` | 资源详情 |
| POST | `/api/v1/items` | 物品查询 |
| POST | `/api/v1/items/:item_id` | 物品详情，路径参数需正确处理冒号 |
| POST | `/api/v1/recipes` | 配方查询，支持 `item_id`、`game_version` |
| POST | `/api/v1/commands` | 指令列表与搜索 |
| POST | `/api/v1/wiki/:slug` | 百科详情 |
| POST | `/api/v1/categories` | 分类列表 |
| POST | `/api/v1/feedback` | 提交反馈/举报 |
| POST | `/api/v1/stats/event` | 匿名使用统计上报（`{scope,id,action}`，成功 `204`，非法/未知对象 `422`） |
| POST | `/api/v1/mc/*` | 种子/结构/群系/区块/`level.dat` 计算能力 |

### 8.3 管理认证 API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/admin/auth/login` | 管理员登录 |
| POST | `/api/v1/admin/auth/logout` | 管理员退出 |
| POST | `/api/v1/admin/auth/me` | 当前管理员信息 |
| POST | `/api/v1/admin/auth/change-password` | 修改密码 |

### 8.4 普通用户 API（可选账号）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/user/register` | 注册（公开，全局限流） |
| POST | `/api/v1/user/login` | 登录（公开，按 用户名 + IP 限流） |
| POST | `/api/v1/user/logout`、`/me` | 退出 / 当前用户信息（登录即可） |
| POST | `/api/v1/user/change-password`、`/delete` | 改密 / 删除账号及个人数据（登录 + CSRF） |
| POST | `/api/v1/user/favorites/list`、`/toggle`、`/remove` | 收藏列表 / 切换 / 取消 |
| POST | `/api/v1/user/recents/list`、`/record`、`/clear` | 最近使用列表 / 记录 / 清空 |
| POST | `/api/v1/user/seeds/list`、`/save`、`/delete` | 常用种子列表 / 保存 / 删除 |

> 普通用户写操作使用会话绑定的 `X-CSRF-Token`；**统一账号（迁移 0022）**后管理员与普通用户共用同一个会话 Cookie（`SESSION_COOKIE_NAME`，默认 `mc_session`），但接口仍按角色隔离：`/user/*` 只接受 `role=user` 会话（后台会话 → 401），后台接口按权限点判定（`role=user` 不含任何权限点 → 403）。

### 8.5 管理 API

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/admin/resources` | 资源列表/创建 |
| POST/PATCH/DELETE | `/api/v1/admin/resources/:id` | 资源详情/更新/软删除或下架 |
| POST | `/api/v1/admin/resources/:id/check-link` | 检查来源链接 |
| POST | `/api/v1/admin/tools` | 工具列表/创建 |
| POST/PATCH/DELETE | `/api/v1/admin/tools/:id` | 工具详情/更新/下架 |
| POST | `/api/v1/admin/wiki` | 百科列表/创建 |
| POST/PATCH/DELETE | `/api/v1/admin/wiki/:id` | 百科详情/更新/软删除 |
| POST/PATCH/DELETE | `/api/v1/admin/categories` | 分类管理 |
| POST/PATCH/DELETE | `/api/v1/admin/tags` | 标签管理 |
| POST | `/api/v1/admin/feedback` | 反馈列表 |
| PATCH | `/api/v1/admin/feedback/:id` | 更新反馈状态 |
| POST | `/api/v1/admin/audit-logs` | 审计日志分页查询 |
| POST | `/api/v1/admin/search-terms/list` | 搜索词统计（admin） |
| POST | `/api/v1/admin/analytics/{overview,tools,resources,api,trend}` | 运营看板只读聚合（admin，可按 `days` 或 `from`-`to` 筛选） |

### 8.6 关键参数

`POST /api/v1/resources`

- `q`: 可选关键词
- `type`: 资源类型
- `game_version`: 游戏版本
- `loader`: 加载器
- `sort`: `updated_desc`、`name_asc`，默认 `updated_desc`
- `page`: 默认 1
- `page_size`: 默认 20，最大 50

`POST /api/v1/recipes`

- `item_id`: 目标物品 ID，建议必填
- `game_version`: 游戏版本，建议必填或使用明确默认值
- `recipe_type`: 配方类型，可选
- `page`、`page_size`

### 8.7 安全与可观测中间件

中间件顺序至少包含：

1. Request ID
2. Recover/Panic Recovery
3. Access Log（过滤敏感字段）
4. Metrics（使用统计采集：路由模板归一 + 状态码/延迟分桶，不采集请求体与查询内容）
5. 安全响应头
6. CORS（仅允许配置的前端来源）
7. 全局限流或关键路由限流
8. 身份认证与角色授权（管理路由）
9. CSRF（Cookie 会话写请求）

CORS 不得使用任意来源 `*` 与凭证同时启用。生产环境仅允许配置的站点域名。

## 9. 前端实现规范

### 9.1 TypeScript 类型

- 为 API 响应、资源、工具、百科、配方、物品、管理员建立明确类型。
- 禁止在业务代码中大面积使用 `any`。
- API 层统一处理超时、网络错误、401、403、429、500。
- 列表页的筛选状态应同步到 URL query，支持刷新和分享。
- 详情页以 slug 为稳定路由标识。

### 9.2 交互要求

- 所有搜索框支持 Enter 搜索、清空和加载状态。
- 筛选条件变更后重置到第 1 页。
- 复制操作使用 Clipboard API；不可用时提供备用方案。
- 删除/下架前二次确认。
- 表单提交时防止重复点击。
- API 失败后显示可理解的信息，不把服务端原始错误直接展示给访客。
- `localStorage` 读取必须捕获 JSON 解析异常。

### 9.3 SEO

- 每个工具、资源、百科详情页有独立 `title`、`description` 和 canonical URL。
- 生成 `sitemap.xml` 和 `robots.txt`。
- 草稿、管理页、登录页不进入站点地图。
- 如果仅使用 Vite SPA，需评估搜索引擎抓取效果；若 SEO 是主要流量来源，后续可迁移到 Nuxt 或引入预渲染。
- 不生成虚假的结构化数据、评分和下载统计。

## 10. 数据导入与内容治理

### 10.1 导入工具

必须实现至少一种可重复运行的数据导入方式，例如 JSON/CSV 导入命令。

要求：

- 校验必填字段、ID 唯一性、版本格式和枚举值。
- 支持 dry-run 预检查。
- 重复运行应可预测：按稳定业务键更新或跳过，不产生重复记录。
- 输出导入成功、跳过、失败的数量和错误行。
- 记录数据源、来源 URL、数据版本、导入时间。
- 不把无许可的第三方数据库直接打包进生产版本。

### 10.2 数据质量

- 内容应标记适用游戏版本。
- 未核实的兼容性、依赖和安全状态不能写成确定事实。
- 资源原始来源 URL 必填。
- 资源作者、许可证和来源信息尽可能完整。
- 对过时或失效内容可隐藏并保留审计记录。
- 百科文章必须保存来源参考与更新时间。

## 11. 安全、隐私与可靠性

### 11.1 安全基线

- 全部输入由后端校验；SQL 必须参数化。
- Markdown/富文本渲染防 XSS。
- 登录接口限流；密码使用安全哈希。
- 管理写接口验证权限与 CSRF。
- 普通用户与管理员**会话、Cookie、角色完全隔离**：用户会话角色固定为 `user`，用户写接口校验会话绑定 CSRF，管理员会话无法访问用户接口（反之亦然）。
- 上传功能首期关闭。
- URL 检查防 SSRF。
- 设置 `Content-Security-Policy`、`X-Content-Type-Options`、`Referrer-Policy` 等安全响应头；CSP 根据实际资源逐步收紧。
- 日志不得记录密码、Cookie、Authorization、密钥。
- 依赖库定期升级并扫描已知漏洞。

### 11.2 隐私

- 公开工具免登录可用，**不要求**访客注册；普通用户账号为可选能力。
- 未登录时收藏与最近使用默认存储在浏览器本地。
- 登录用户的收藏、最近使用与常用种子存于服务端并跨设备同步；保留期限：最近使用「最近 50 条」、收藏与种子保留至用户删除、删除账号即时永久清除全部个人数据。
- 不采集手机号、邮箱、设备标识等不必要信息；密码仅存 Argon2id 哈希。
- 反馈联系方式为可选项。
- 只收集排障和安全所需日志。
- 使用统计**匿名且按天聚合**：只存计数与归一化关键词，不存 IP、会话、用户标识、UA、请求体与完整查询内容；接口统计只保留路由模板；聚合数据保留期限默认 180 天（`STATS_RETENTION_DAYS`）；看板仅管理员可见，隐私政策已同步说明。
- 提供隐私政策、反馈联系方式和数据清除说明。
- **分享链接不含会话秘密**：可分享 URL 仅承载公开的查询/筛选参数（由 `utils/share.ts` 构造，默认值不写入）；复制前再剥离疑似机密参数（`token`/`csrf`/`session`/`cookie`/`password`/`authorization`/`api_key`/`signature` 等）与 hash 中的同类片段。会话 Cookie 为 HttpOnly，CSRF 仅存于前端内存/请求头，从不进入 URL。

### 11.3 性能目标

在常规网络和合理设备条件下：

- 首页 LCP 目标 ≤ 2.5 秒。
- 公共列表 API P95 目标 ≤ 500ms（基准数据规模内）。
- API 设置合理超时和数据库连接池。
- 图片懒加载、压缩和尺寸约束。
- 列表分页，不允许无界返回数据。
- 通过数据库索引和查询分析优化慢 SQL。

### 11.4 备份与恢复

- 每日数据库备份；根据业务重要程度配置异地副本。
- 备份文件加密并限制访问。
- 定期进行恢复演练。
- 记录备份保留周期。
- 发布失败可回滚应用版本；数据库迁移应尽量采用向前兼容策略。

## 12. 配置与环境变量

提供 `.env.example`，至少包含：

```dotenv
APP_ENV=development
APP_ADDR=:8080
APP_BASE_URL=http://localhost:5173
DATABASE_URL=postgres://mc_user:change_me@localhost:5432/mc_tools?sslmode=disable
SESSION_SECRET=replace_with_random_secret
SESSION_COOKIE_NAME=mc_admin_session
SESSION_SECURE_COOKIE=false
CORS_ALLOWED_ORIGINS=http://localhost:5173
LOG_LEVEL=debug
RATE_LIMIT_RPS=10
RATE_LIMIT_BURST=20
LINK_CHECK_TIMEOUT_SECONDS=8
LINK_CHECK_MAX_REDIRECTS=3
STATS_ENABLED=true
STATS_FLUSH_SECONDS=60
STATS_RETENTION_DAYS=180
```

> `STATS_ENABLED` 关闭后不再采集与落库（看板接口仍可用，但数据停止增长）；`STATS_FLUSH_SECONDS` 为内存聚合落库周期；`STATS_RETENTION_DAYS` 为按天聚合数据的保留期限。

约束：

- 生产环境 `SESSION_SECURE_COOKIE=true`。
- 生产环境 `SESSION_SECRET` 必须为强随机值。
- 真实 `.env` 不提交版本控制。
- 应用启动时校验必需配置；缺少关键配置应明确失败，不静默使用弱默认值。
- 数据库密码不得使用上述示例值部署生产环境。

## 13. 测试要求

### 13.1 后端单元测试

至少覆盖：

- 坐标换算和边界输入。
- 资源/工具/百科参数校验。
- slug 唯一性与冲突处理。
- 角色权限判定。
- 资源 URL 协议与 SSRF 拦截规则。
- 分页参数边界。
- 反馈表单长度和限流规则。
- 使用统计内存聚合与批量累加、分桶近似 P95、保留期清理；路由模板归一（参数折叠）；`/stats/event` 白名单校验。

### 13.2 API 集成测试

至少覆盖：

- 未登录不能访问管理 API。
- editor 不能执行 admin 专属操作。
- 管理员可创建、更新、下架资源。
- 公开 API 不返回草稿和下架内容。
- 搜索结果正确分页。
- 非法 URL 无法通过链接检查。
- 反馈提交成功、过长字段被拒绝。
- 错误响应不包含敏感内部信息。

### 13.3 前端测试

至少覆盖：

- 坐标换算结果。
- 搜索、筛选、分页状态。
- 收藏写入/读取异常。
- 复制按钮反馈。
- 表单校验和 API 错误状态。
- 管理路由的未登录体验。
- 使用统计上报（路径→slug 反查、测试环境静默、失败静默）与运营看板（卡片/趋势/明细/空态/错误态/时间范围）；图表配置用真实 ECharts（SSR）渲染校验。
- 分享与稳定链接：机密参数识别与剥离、默认值省略、种子与结构工具 URL 编解码往返、四类工具的 URL 还原与回写、带种子链接打开即分析、`ShareButton` 复制内容不含机密参数。
- 界面状态与输入健壮性：数字输入（`<input type="number">` 被 Vue 转为 number）不得导致运行期错误（指令生成器 / 坐标换算回归）。

### 13.4 手工验收

- 桌面和手机浏览器布局正常。
- 键盘可完成核心操作。
- 断网、接口失败、无结果时有明确反馈。
- 管理员权限无法通过直接请求绕过。
- 页面没有未处理的控制台错误。
- 生产环境无调试接口和默认凭证。

## 14. 开发阶段与任务拆分

### 阶段 A：工程骨架

1. 建立仓库结构、Go 模块和 Vue 项目。
2. 配置代码格式、lint、测试命令。
3. 配置 Docker Compose、PostgreSQL、环境变量。
4. 实现健康检查、Request ID、统一错误响应。
5. 建立数据库迁移与 OpenAPI 基础文件。

**验收**：开发环境一条文档化命令可启动；健康检查正常；迁移可执行。

### 阶段 B：数据库与管理认证

1. 创建核心表和索引。
2. 实现管理员初始化命令。
3. 实现登录、退出、当前用户和密码修改。
4. 实现角色授权、CSRF、限流和审计日志。
5. 建立后台布局和登录页。

**验收**：未登录/低权限访问被后端拒绝；密码不以明文保存。

### 阶段 C：资源与工具管理

1. 实现资源 CRUD、筛选、分页和下架。
2. 实现工具 CRUD。
3. 实现百科 CRUD。
4. 实现分类、标签和反馈管理。
5. 实现链接检查与 SSRF 防护。

**验收**：后台新增的已发布资源可在公共页面查看；草稿和下架内容不可见。

### 阶段 D：公共网站

1. 实现首页、公共导航和响应式布局。
2. 实现工具中心与工具详情。
3. 实现坐标换算。
4. 实现物品 ID 查询。
5. 实现合成配方查询。
6. 实现指令格式辅助。
7. 实现资源列表、筛选和详情。
8. 实现全站搜索、反馈、隐私和版权页面。

**验收**：四个核心工具可用；搜索与资源跳转完整；移动端可操作。

### 阶段 E：数据导入与 SEO

1. 实现 items、recipes、commands 的导入命令。
2. 准备可合法使用的演示数据。
3. 实现页面 metadata、canonical、sitemap 和 robots。
4. 检查数据来源与授权说明。

**验收**：导入器可重复运行；详情页 SEO 字段独立；无来源不明的生产数据。

### 阶段 F：发布准备

1. 执行单元、集成和前端测试。
2. 检查权限、SSRF、XSS、CSRF、限流和安全响应头。
3. 配置 HTTPS、备份、日志和监控。
4. 完成 README、部署手册和恢复演练。
5. 执行第 15 节验收清单。

**验收**：无阻断级缺陷，部署和恢复流程可复现。

## 15. 最终验收清单

> 本节为**需求原始验收条目**（保留原文）；**实际验收状态见第四部分 §4.7 验收清单状态**（截至 2026-10-19 除「移动端真机走查」外均已完成）。

- [ ] `docker compose up --build` 可按 README 启动开发/测试环境。
- [ ] 数据库迁移可从空库完整执行。
- [ ] 可安全初始化管理员，不存在固定默认密码。
- [ ] 未登录用户无法访问任何管理写接口。
- [ ] editor 无法执行 admin 专属操作。
- [ ] 首页、工具中心、资源中心、详情页适配桌面和手机。
- [ ] 坐标换算正确处理正数、负数、小数和非法输入。
- [ ] 物品 ID 查询和配方查询展示版本信息与来源。
- [ ] 资源支持类型、版本、加载器筛选。
- [ ] 资源原始来源链接可访问，失效状态可维护。
- [ ] URL 检查器阻止内网、回环、元数据地址和危险重定向。
- [ ] 全站搜索支持分页和空结果提示。
- [ ] 收藏和最近使用在本地存储损坏时不会导致页面崩溃。
- [ ] 普通用户账号可选：公开工具免登录可用；未登录访问工作台被拒（401）；管理员会话无法访问用户接口，用户会话无法访问管理接口。
- [ ] 普通用户写接口校验 CSRF；删除账号可清除其全部个人数据（收藏 / 最近使用 / 常用种子）。
- [ ] API 有统一错误格式、请求 ID、超时和限流。
- [ ] 日志中没有密码、令牌或密钥。
- [ ] SEO 标题、描述、canonical、sitemap、robots 配置完成。
- [ ] 版权、隐私、反馈入口可访问。
- [ ] 数据备份和恢复步骤经过验证。
- [ ] README 包含开发、测试、部署、初始化、备份和故障排查。

## 16. 给开发人员/AI 编程工具的执行指令

请严格按本文档实施，不要一次性生成所有代码后不验证。按阶段提交，每阶段完成后运行测试并汇报结果。

每次交付必须包含：

1. 本次新增/修改文件清单。
2. 实现内容与未完成项。
3. 运行命令和测试结果。
4. 数据库迁移与环境变量变化。
5. 已知风险和下一阶段任务。

实施顺序：

1. 先建立工程骨架、数据库迁移、健康检查和统一 API 响应。
2. 再实现管理员认证、授权、CSRF、审计日志。
3. 再实现资源/工具/百科后台 CRUD。
4. 再实现公共页面和四个核心工具。
5. 再接入合法数据导入、搜索、SEO。
6. 最后执行安全检查、测试、部署和验收。

若遇到未定义的细节，优先选择**实现简单、默认安全、便于以后扩展**的方案，并在 README 中记录决策；不得擅自扩大首期范围。

---

# 第三部分 · 开发进度

> 原文件：`PROGRESS.md` ｜ 同步基线：需求文档 1.0；技术栈：Go 1.27 + Chi + GORM + PostgreSQL 16 + Vue 3

## 3.1 阶段状态

- [x] **阶段 A：工程骨架** — 目录、配置、健康检查、中间件链、迁移框架、compose、.env、坐标换算核心 + 测试
- [x] **阶段 B**：管理员初始化、Argon2id、Cookie 会话、登录/登出/me/改密、限流、CSRF、角色中间件、审计日志
- [x] **阶段 C**：后台 CRUD（资源/工具/百科/分类/标签/反馈）+ 链接检查防 SSRF + JSONB 原生序列化
- [x] **阶段 D（后端部分）**：搜索 API、物品查询、配方查询、指令查询、演示数据导入
- [x] **阶段 E（后端）**：数据导入器（items/recipes/commands）+ SEO（sitemap/robots）+ 百科抓取器（wikicrawl：DOM→Markdown、重定向、限速重试、来源标注）
- [x] **MC 元数据与种子工具（后端）** — `mcversion`（Mojang 版本清单/详情 + 7 天磁盘缓存）、`blockids`（物品 ID 对照表）、`mcstructure`（结构理论坐标/要塞/史莱姆/种子生成与搜索）、`region`+`nbt`（上传 .mca 解析，仅内存不落盘）、`mcbiome`（cubiomes 移植，生物群系采样）
- [x] **前端**：Vue 3 全站 — 首页/工具中心/合成/物品/指令/坐标/资源/搜索/反馈/隐私/版权/关于/404 + 管理后台全部页面
- [x] **前端（补全）**：接线 `/versions`、新增 `/block-ids`、`/seed-map`、`/seed-tools`、`/region-inspect`；导航分层简化；`BUILTIN_TOOLS` 单一事实源；修复 `/versions` 重复注册
- [x] **资料库扩展（§9-A，2026-10-17）**：药水与酿造 — `potions` + `brewing_recipes` 表与种子导入（26 药水 / 39 步骤）、三个公开接口（检索 / 通用规则 / 详情）、前端 `/potions` 检索页 + `/potions/{slug}` 详情页（效果变体、酿造步骤链、后续变化、来源核验）
- [x] **开发栏目（§21，2026-10-18，已验收）**：开发文档知识库 + 版本映射查询 — 迁移 `0013_dev_docs` 四表、`seeds/dev_docs_demo.json` 40 条（189 条关键接口）、`internal/mappings` 解析包 + `cmd/import-mappings`（ProGuard / TSRG·TSRG2 / Tiny v1·v2）、`cmd/seed -type devdocs`、六个公开接口、前端 `/dev` 检索页 + `/dev/{slug}` 详情页 + `/mappings` 三列对照页、内置工具 12 → 14（**不预置映射数据**，未导入时展示导入指引）
- [x] **资料库扩展（§9-B 村民交易，2026-10-18）**：迁移 `0014_villagers` 三表（16 职业 / 5 级规则 / 223 交易）、`cmd/seed -type villagers`、三个公开接口、前端 `/villagers` + `/villagers/{slug}`（工作站 / 等级规则 / 分组交易表）；交易成本用区间表达，未收录物品不生成死链
- [x] **资料库扩展（§9-C 生物资料，2026-10-18）**：迁移 `0016_mobs` 两表（48 生物 / 70 掉落）、`cmd/seed -type mobs`、两个公开接口、前端 `/mobs` + `/mobs/{slug}`（生命值 / 掉落区间与概率 / 生成与驯服）；修复 GORM `default` 标签导致的 `count_min=0` 数据失真
- [x] **链接健康（§6-B，2026-10-18）**：迁移 `0015_link_health`、`internal/linkcheck` 并发硬顶 / 逐跳重定向 / SSRF 防护、后台复查与人工确认（确认失效 / 可用 / 暂不处理）+ 审核记录、`/admin/link-health` 页面
- [x] **运营与工程（§13 / §14 / §18 / §19 / §20，2026-10-17 ~ 2026-10-19）**：使用统计与运营看板、分享与稳定链接、可访问性规范、OpenAPI / 测试 / 文档同步、安全与可靠性加固、干净环境迁移与备份恢复实测
- [x] **红石资料库（§15，2026-10-18，已验收）**：迁移 `0018_redstone` 两表（27 元件 / 15 设计）、`cmd/seed -type redstone`、公开 + 管理端审核接口、前端 `/redstone` 双 tab + 详情页、管理端审核页 `/admin/redstone`
- [x] **Mod 兼容性（§16，2026-10-18，已验收）**：迁移 `0019_mods` 五表（17 Mod / 34 版本 / 12 依赖 / 7 整合包 / 57 清单项）、`cmd/seed -type mods` 两阶段导入、公开 + 管理端审核接口、前端 `/mods` 双 tab + 详情页、管理端审核页 `/admin/mods`
- [x] **用户投稿与纠错（§17，2026-10-18，已验收）**：迁移 `0017_feedback_submissions`（投稿类型 / 关联资料 / 审核留痕 / 授权条款 / 反垃圾 + `feedback_reviews` 历史表）、公开投稿 + 管理端审核接口、前端投稿表单与管理端审核页
- [x] **阶段 F**：全量测试、README、部署手册、验收清单 —— 已执行（`go vet` / `go test` / `vue-tsc` / `vitest` 472 项 / `vite build` / 干净库迁移 / 备份恢复均通过）；**移动端真机走查保留为人工待办**

## 3.2 环境

- PostgreSQL 16（Docker compose）`127.0.0.1:5432`，db=`mc_tools`，user=`mc_user`
- 后端 `:8080`，前端 `:5173`；测试管理员 `admin`

## 3.3 说明

阶段 F 已执行完毕（见上方勾选）。当前**唯一的功能性缺口**为「移动端真机走查」（需人工在真机 / 模拟器完成）；其余待办为可选增强（CI 流水线、标签关联表、迁移回滚命令等），详见第四部分。原 `BACKEND_TODO.md` 所列 P0–P2 事项**均已修复**，归档见第五部分。

---

# 第四部分 · 未完成事项 TODO

> 原文件：`todo.md` ｜ 核对基线：需求 v1.0 ｜ 核对方式：逐文件比对 + 实跑测试
> 本部分为**整合时的待办快照**；实时待办与勾选状态以根目录 `todo.md` 为准，功能状态以 `Function.md` 为准。旧 `BACKEND_TODO.md` 已滞后，其已修复条目见第五部分。

## 4.0 验证结果（2026-10-14 基线实跑）

> 最新全量验证结果见 **§1.6（2026-10-19）**；本节保留整合当时的基线数据以作追溯。

| 检查项 | 命令 | 结果 |
|---|---|---|
| 后端静态检查 | `cd backend && go vet ./...` | ✅ 通过（无输出） |
| 后端测试 | `cd backend && go test ./...` | ✅ 全部包通过（集成测试未设 `TEST_DATABASE_URL` 时自动跳过） |
| 前端类型检查 | `cd frontend && npx vue-tsc -b --noEmit` | ✅ 通过 |
| 前端单测 | `cd frontend && npm run test` | ✅ 11 文件 / 93 项全绿 |
| 前端构建 | `cd frontend && npm run build` | ✅ 通过 |
| 数据库容器 | `docker compose -f deploy/compose.yaml ps` | ✅ postgres healthy |

**结论**：代码可编译、测试可通过，**没有阻断级缺陷**。以下为需求符合度与工程完备度上的真实缺口。

### 4.0.1 本轮补充验证（TODO §13 使用统计与运营看板，2026-10-17 实跑）

| 检查项 | 命令 | 结果 |
|---|---|---|
| 迁移 | `cd backend && go run ./cmd/migrate` | ✅ 已应用 `0011_usage_stats` |
| 后端静态检查 / 测试 | `cd backend && go vet ./... && go test ./...` | ✅ 通过（含 `metrics` / `analytics` / `stats` 新增单测） |
| 接口实测（curl） | `POST /api/v1/stats/event` 与 5 个 `/admin/analytics/*` | ✅ 上报 `204`、未知对象 `422`、概览/明细/趋势数值正确、未登录 `401`、非法范围 `422` |
| 前端类型检查 | `cd frontend && npm run typecheck` | ✅ 通过 |
| 前端单测 | `cd frontend && npm run test` | ✅ 32 文件 / 335 项全绿 |
| 前端构建 | `cd frontend && npm run build` | ✅ 通过（`AdminAnalyticsView` 懒加载 chunk 含 ECharts，仅管理员加载） |
| 链路联调 | 经前端 dev 代理上报 → 落库 → 看板查询 | ✅ `tool_visits/tool_uses/tool_failures` 与 API 明细一致，路由只留模板 |

> 说明：图表渲染在 jsdom 下无法验证（无 canvas），故另设 `tests/analytics-chart.spec.ts` 用真实 ECharts **SSR + SVG** 渲染视图配置以替代浏览器验证；真实浏览器交互未在本环境执行。

### 4.0.2 本轮补充验证（TODO §14 分享与易用性，2026-10-17 实跑）

| 检查项 | 命令 | 结果 |
|---|---|---|
| 后端静态检查 / 测试 | `cd backend && go vet ./... && go test ./...` | ✅ 通过（本批未改动后端代码，回归确认无影响） |
| 前端类型检查 | `cd frontend && npm run typecheck` | ✅ 通过 |
| 前端单测 | `cd frontend && npm run test` | ✅ **36 文件 / 373 项**全绿（新增 4 个文件 38 项） |
| 前端构建 | `cd frontend && npm run build` | ✅ 通过（5.85s） |

| 新增测试 | 覆盖内容 |
|---|---|
| `tests/share.spec.ts`（14） | 机密参数识别、空值/默认值省略、`buildShareUrl` 丢弃 path 自带查询与 hash、`sanitizeShareUrl` 清理查询与 hash 片段、非 URL 原样返回、`queryString/Number/Bool` 回退 |
| `tests/seed-tools-query.spec.ts`（6） | 默认状态 ⇔ 无参数、全子表单往返一致、非法版本/非法结构类型回退、`0` 值不当作空值 |
| `tests/share-button.spec.ts`（4） | 复制内容被子表单机密参数剥离、默认取地址栏链接、失败提示、`aria-label` 同步 |
| `tests/share-url-sync.spec.ts`（10） | 物品 ID 对照表 / 坐标换算 / 种子地图 / 种子与结构工具的 URL 还原、回写、多字段互不覆盖、带种子链接打开即分析、默认筛选链接干净 |
| `tests/command-generator.spec.ts`（+4） | **数字表单值回归**：`number` 型 count / damage / duration / 坐标不再触发 `trim is not a function` |

> **顺带修复的真实缺陷**：`<input type="number">` 上的 `v-model` 会被 Vue 自动转成 `number`，而 `CoordinateView` 与 `utils/command-generator.ts` 按字符串调用 `.trim()`，导致**用户一输入就抛错**（此前无视图级测试覆盖）。已按字符串归一并补回归用例。
> **未做**：真机 / 模拟器上的移动端走查与屏幕阅读器实测（本环境无浏览器自动化）；已通过代码级检查（表格横向滚动、长文本换行、`scope`、跳转链接、状态语义）替代，并在 §13.4 保留人工验收项。

### 4.0.3 本轮补充验证（TODO §9-A 药水与酿造资料，2026-10-17 实跑）

| 检查项 | 命令 | 结果 |
|---|---|---|
| 迁移 | `cd backend && go run ./cmd/migrate` | ✅ 已应用 `0012_potions` |
| 种子导入（dry-run） | `go run ./cmd/seed -type potions --file seeds/potions_demo.json --dry-run` | ✅ 65 条全部通过校验（26 药水 + 39 酿造步骤），失败 0 |
| 种子导入（实导） | `go run ./cmd/seed -type items --file seeds/items_demo.json` → `-type potions ...` | ✅ items 33 条（新增 25 / 更新 8）、potions 65 条（新增 65），失败 0 |
| 接口实测（curl） | `POST /api/v1/potions`（默认 / `q` / `category` / `version`） | ✅ 默认 26 条；`q=治疗` → 1 条；`category=negative` → 8 条；`version=1.20` → 22 条且**不含** `1.21+` 四种、`version=1.21` → 26 条（**精确版本语义生效**） |
| 接口实测（curl） | `POST /api/v1/potions/potion-of-swiftness` | ✅ 返回普通/延长/增强三变体（180 / 480 / 90 秒）、酿出步骤（粗制的药水＋糖）、后续变化（迟缓药水）、4 条通用规则、6 项关联物品摘要 |
| 接口实测（curl） | `POST /api/v1/potions/rules` | ✅ 静态段未被 `/potions/{slug}` 吞掉，返回 4 条通用规则 + 4 项关联物品 |
| 接口实测（curl） | 未知 slug / 非法类别 | ✅ 分别 `404` / `200`（非法筛选值忽略而非 500） |
| 数据质量检查 | 临时程序调用 `dataquality.Overview`（已删除） | ✅ `potions_*` / `brewing_*` 共 10 项全部为 0 计数，含悬空引用 error 项 |
| SEO | `GET /sitemap.xml` | ✅ 收录 `/potions` 与 26 个药水详情页 |
| 后端静态检查 / 测试 | `cd backend && go vet ./... && go test ./...` | ✅ 通过（含 `versioning.Satisfies`、`cmd/seed` 药水种子有效性等新增单测） |
| 前端类型检查 | `cd frontend && npm run typecheck` | ✅ 通过 |
| 前端单测 | `cd frontend && npm run test` | ✅ **37 文件 / 384 项**全绿（新增 `tests/potions.spec.ts` 10 项，`routes.spec.ts` 因新增内置工具 +1） |
| 前端构建 | `cd frontend && npm run build` | ✅ 通过（`PotionsView` 8.15 kB、`PotionDetailView` 10.47 kB，均懒加载） |
| 服务日志 | API 运行期 stderr | ✅ grep `error\|panic` 计数为 0 |

> **版本口径差异（重要）**：药水以「自某版本起」声明适用版本（`1.0+` / `1.4.2+` / `1.7.2+` / `1.8+` / `1.9+` / `1.13+` / `1.21+`），查询侧新增 `versioning.Satisfies` 做**版本大小精确判断**（选 `1.20` 命中 `1.8+`、不命中 `1.21+`）。这与工具/资源采用的「声明文本匹配」语义不同，故在 `Function.md §2.19` 单独说明，避免误解。
> **未核验项（有意保留，不填猜测值）**：`potion-of-decay`（仅基岩版）效果数值标注「未在本站核验」（`effects: []`）；`potion-of-freezing` 尚未正式发布，未收录；`uncraftable-potion` / `potion-of-luck` / `potion-of-decay` 标记为不可酿造。
> **未做**：药水未接入全站搜索（`internal/search` 的 `item` 类型）；村民交易（§9-B）与生物资料（§9-C）未开工。

### 4.0.4 本轮开发记录（TODO §21 我的世界开发栏目，2026-10-18，**开发时未验证 → 2026-10-19 已补验收**）

> ⚠️ **本批按用户要求「不编译、不测试，只开发功能」执行**：以下条目**尚未执行** `go vet` / `go test` / `vue-tsc` / `vitest` / 迁移与种子导入 / `curl` 接口验收，因此 `Function.md §9` 对应行标记为 **🧪 待验收**而非「已实现」。仅做了 `gofmt`（解析 + 格式化，可捕获语法错误）与种子 JSON 的结构校验（`ruby JSON.parse`、slug 唯一、字段齐全）。

| 层面 | 交付内容 |
|---|---|
| 数据层 | 迁移 `0013_dev_docs.sql`：`dev_docs`（正文 `body_md` + 结构化 `api_entries` JSONB + 平台/主题/状态/版本/来源）+ `mapping_sets` / `mapping_classes` / `mapping_members`（映射数据集两级结构，`obf` / `mid` / `named` 三列名字 + 描述符列）；含索引、CHECK 与回滚 SQL 注释 |
| 种子数据 | `seeds/dev_docs_demo.json` 共 40 条（NeoForge 11 / Forge 5 / Fabric 9 / 原版数据 8 / 跨平台 1 / 工具链 6；主题覆盖入门 / 事件 / 注册表 / 网络 / 数据生成 / 资源 / 渲染 / 序列化 / API 速查 / 映射 / 工具链），合计 189 条关键接口；顶层 `_comment` 记录链接核对结论（NeoForge 官方站在核对网络中不可达，故以官方仓库文档源文件作为 `source_url` 锚点） |
| 导入器 | `internal/mappings` 解析包（ProGuard / TSRG·TSRG2 / Tiny v1·v2 + gzip 自动解压 + 命名空间归类，含合成夹具单测 `parsers_test.go`）；`cmd/import-mappings` CLI（单事务按 `set_key` 整体替换、批量写入、回填计数、`--dry-run`）；`cmd/seed -type devdocs` |
| 接口 | `POST /api/v1/dev-docs`、`/dev-docs/facets`、`/dev-docs/{slug}`、`/mappings/sets`、`/mappings/search`、`/mappings/class/{id}`；OpenAPI 同步 6 个路径 + 10 个 schema；数据质量新增 `dev_docs_*` 7 项 + `mapping_sets_*` 3 项；sitemap 收录 `published` 开发文档详情页 |
| 前端 | `types/devdoc.ts` / `types/mapping.ts` / `api/dev.ts`；`DevDocsView`（筛选 + 版本精确筛选说明 + 卡片 + 分享 URL）、`DevDocDetailView`（接口速查可整段复制 + 正文 Markdown + 来源与校验 + 相关条目）、`MappingsView`（数据集选择 + 三列对照 + 成员明细展开 + **未导入时导入指引**）；路由 `/dev`、`/dev/:slug`、`/mappings`；导航新增「开发文档」；内置工具 12 → 14；`ToolCard` 版本行新增 `all` → 「全版本」展示；`tests/dev-docs.spec.ts` |
| 关键决策 | **不预置任何映射数据**（真实混淆名随版本变化且无法核验，预置会误导），改为提供导入器 + 空态导入指引 + 官方下载入口；映射查询响应把 `truncated`（扫描上限截断标记）与分页字段一并放在 `data`（业务语义，非纯分页元信息） |

> **待验收清单（本批未做）**：`gofmt` 之外的编译、`go vet` / `go test`、`npm run typecheck` / `npm run test` / `npm run build`、迁移 `0013` 在干净库执行、`cmd/seed -type devdocs` 与 `cmd/import-mappings` 实跑、六个接口 `curl` 冒烟（含版本精确筛选与 404）、数据质量 13 项计数为 0、sitemap 收录开发文档详情页。
> **未做**：开发文档未接入全站搜索（`internal/search`）——**已于 2026-10-19 补做（见下）**；仓库内不含任何真实映射文件。
> ✅ **后续更新（2026-10-19）**：上述待验收项**均已补做通过** —— 迁移 `0013` 在全新库应用、`cmd/seed -type devdocs` 导入 40 条（`dev_docs` 表 40 行）、`/dev-docs`、`/dev-docs/{slug}`、`/mappings/sets`、`/mappings/search` 实测 200；`todo.md` §21 与 `Function.md` 已由「待验收」改为「已实现」。
> ✅ **再更新（2026-10-19，全站搜索接入）**：迁移 `0020_dev_docs_search_vector` 为 `dev_docs` 增加 `search_vector` 生成列 + GIN 索引；`internal/search` 新增 `devdoc` 类型（命中中英文标题 / slug / 摘要 / 正文 / 标签，仅返回已发布，结果直达 `/dev/{slug}`）；前端搜索页类型筛选与全局搜索框标签新增「开发文档」。实测：`POST /search {"q":"DeferredRegister"}` 与 `{"q":"事件","type":"devdoc"}`（total 8）、正文关键词 `mixin` 均命中；集成用例 `TestSearchDevDocs` 通过；前端 `vitest` 47 文件 / 476 项全绿。

### 4.0.5 本轮已完成与验证（2026-10-17 ~ 2026-10-19，§9-B / §9-C / §13–§21）

> 本小节汇总整合基线（§4.17，2026-10-17）之后完成的全部批次，均**已实测验收**；逐项证据以 `todo.md` 对应章节与 `Function.md §九` 为准。

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **§9-B 村民交易** | 迁移 `0014_villagers`（`villager_professions` 16 / `villager_trade_levels` 5 / `villager_trades` 223）；`cmd/seed -type villagers`；`POST /villagers`、`/villagers/levels`、`/villagers/{slug}`；前端 `/villagers` + `/villagers/{slug}` | 逐行校验 + 幂等 upsert；未收录物品不生成死链；`tests/villagers.spec.ts` |
| **§9-C 生物资料** | 迁移 `0016_mobs`（`mobs` 48 / `mob_drops` 70）；`cmd/seed -type mobs`；`POST /mobs`、`/mobs/{slug}`；前端 `/mobs` + `/mobs/{slug}` | 逐行校验 + 幂等 upsert；修复 GORM `default` 导致 `count_min=0` 被改写为 1 的失真；`tests/mobs.spec.ts` |
| **§6-B 链接健康** | 迁移 `0015_link_health`（`resource_link_checks`）；`internal/linkcheck` 并发硬顶 16 / 默认 4 + 逐跳重定向 + SSRF（拒绝环回 / 私网 / 链路本地 / 未指定）；后台复查 + 人工确认（`pending`/`confirmed_broken`/`confirmed_ok`/`ignored`）+ 审核留痕；前端 `/admin/link-health` | 集成测试实跑通过（`TEST_DATABASE_URL`）；`tests/admin-link-health.spec.ts` |
| **§15 红石与机器设计** | 迁移 `0018_redstone`（`redstone_components` 27 / `redstone_designs` 15）；`cmd/seed -type redstone`；公开 4 接口 + 管理端审核 2 接口；前端 `/redstone` 双 tab + 详情页 + `/admin/redstone` | 导入 42 条、二次「新增 0」；公开接口 200 / 未收录 404 / 管理端未登录 401；材料数量区间与授权存档校验；`tests/redstone.spec.ts`（11）+ `tests/admin-redstone.spec.ts`（4） |
| **§16 Mod 与整合包** | 迁移 `0019_mods`（`mods` 17 / `mod_versions` 34 / `mod_dependencies` 12 / `modpacks` 7 / `modpack_mods` 57）；`cmd/seed -type mods` 两阶段导入；公开 4 接口 + 管理端审核 4 接口；前端 `/mods` 双 tab + 详情页 + `/admin/mods` | 导入 24 条、二次「新增 0」；公开 200 / 未收录 404 / 管理端未登录 401；**不做自动冲突分析**；`tests/mods.spec.ts`（15）+ `tests/admin-mods.spec.ts`（5） |
| **§17 用户投稿与纠错** | 迁移 `0017_feedback_submissions`（`feedback` 扩展 + `feedback_reviews`）；`POST /feedback` 公开投稿 + `/admin/feedback/{update,reviews}`；前端投稿表单与管理端审核页 | curl：未接受条款 422 / 接受后 201 / 重复提交 409 / `javascript:` 422；`content_test.go`（4 例）+ `tests/feedback-submissions.spec.ts`（7） |
| **§13 使用统计与运营看板** | 迁移 `0011_usage_stats`（四张日聚合表）；`/stats/event` + 5 个 `/admin/analytics/*`；前端 `/admin/analytics` | 上报 204 / 未知对象 422 / 未登录 401；仅存计数与归一化关键词，不存个人数据 |
| **§14 分享与易用性 / 可访问性** | `ShareButton` + `utils/share.ts`（机密参数剥离、默认值省略、URL 往返）；界面四态规范与跳转链接 / `scope` / 长文本换行 | 12 个工具支持可分享 URL；修复 `<input type="number">` 触发 `.trim()` 的真实缺陷 |
| **§18 API / 测试 / 文档** | OpenAPI 同步、迁移脚本（`0017`–`0019`）、后端单测 / 集成测试、前端 Vitest、`go vet` / `go test` / `vue-tsc` / `vitest` / `vite build` | 全量回归通过；`vitest` **47 文件 / 476 项**全绿；`README` 导入章节补齐 |
| **§19 安全与可靠性** | 上传 64 MiB 上限 + 5s 解析时限 + `ConcurrencyLimit`；链接检查并发硬顶 + SSRF；73 处 `MaxBytesReader`；审核路由 `RequireAuth + RequireRole(admin) + CSRF`；`response.HandleError` 补 500 服务端日志 | 逐项实测（未登录 401、越权 403、SSRF 拒绝、备份恢复一致） |
| **§20 交付验收** | 干净环境迁移（20 迁移 / 43 表）；备份恢复实测（14 表行数一致）；`Function.md` / `todo.md` 同步 | §20 除「移动端真机走查」外全部勾选 |
| **§21 开发栏目** | 见 §4.0.4，已由「仅开发」转为**已验收** | 见上方 ✅ 后续更新 |
| **开发文档接入全站搜索** | 迁移 `0020_dev_docs_search_vector`（`dev_docs.search_vector` 生成列 + GIN 索引）；`internal/search` 新增 `devdoc` 类型；前端搜索页类型筛选 + 全局搜索框标签 + OpenAPI | 实测：标题 / 正文 / 类型筛选均命中，草稿排除；`TestSearchDevDocs` 集成用例通过；`vitest` 476 项全绿 |

### 4.0.6 本轮已完成与验证（2026-10-19，后台账号与权限 + mcsrc 切片 1）

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **角色与权限底座（§23）** | 迁移 `0021_roles`（`roles` 表 `name` 主键 + `permissions` JSONB + `is_system` + 内置 `admin`/`editor`/`user` 种子；`admins.role` 由 CHECK 改为外键引用 `roles.name`；含反向 SQL）；`internal/domain/role.go`（17 个权限点 / 4 组 + `AllPermissions` / `NormalizePermissions`）；`internal/service/role`；`middleware.RequirePermission`；`/admin/roles/{list,permissions,create,update,delete}` + 审计；`main.go` 全量替换 `RequireRole(admin)`；登录 / `me` 返回 `permissions` | 集成测试 `TestRoleManagement`（editor 403 / 非法权限点 422 / 删内置 422 / admin 削权 422 / 自定义角色权限真实生效 / 被引用角色 409 / 删号后 204）通过 |
| **角色管理页（§23）** | `/admin/roles`：列表 + 新建 / 编辑弹窗 + 权限点分组勾选 + 内置 admin 锁定 + 删除确认；侧边栏由 `adminOnly` 改为**权限点过滤**；`AdminAdminsView` 角色下拉改为动态加载 `roles` | `tests/admin-roles.spec.ts`（7 例）+ `tests/admin-sidebar.spec.ts` 通过 |
| **普通用户管理（§23）** | `internal/service/useradmin` + `/admin/users/list`（`accounts.users` + CSRF，只读）+ `types/admin.ts` 计数类型；后台仪表盘「用户总数」取该接口 `total` | `tests/admin-users.spec.ts` + `tests/admin-dashboard.spec.ts` 通过 |
| **MC 源码浏览器切片 1（§22）** | `internal/mcsrc`（`store.go` jar 下载 / sha1 校验 / 体积上限 / 原子改名 / 单飞锁；`index.go` 只读 zip 中央目录建类索引 + 相关度检索）；`POST /mcsrc/{status,prepared,classes/list}`（公开只读）+ `/mcsrc/prepare`（`ops.mcsrc` + CSRF + 重接口限流 + 并发上限 1）；配置 `MCSRC_ENABLED` / `MCSRC_CACHE_DIR` / `MCSRC_JAR_MAX_MB` / `MCSRC_INDEX_MAX_CLASSES`；前端 `/mcsrc` 内置工具（版本 + 侧别 + 状态 + 类检索 + 管理员准备按钮） | 单测（索引 / 检索 / 路径穿越 / 单飞 / 截断 / 零值时间）+ `handler` 单测 + 集成 `TestMcsrcRoutesAuthGating` + **真实上游 `TestLivePrepareRealJar`**（1.8.9：2507 类）+ 运行中服务 curl（200 / 422 / 409 `NEED_PREPARE` / 匿名 401）全部通过 |
| **MC 源码浏览器切片 2（§22）** | `internal/mcsrc`（`classfile.go` class 字节码解析；`descriptor.go` 描述符渲染；`structure.go` 结构视图 + 继承关系 + jar 摘要缓存的关系索引）；`POST /mcsrc/{classes/detail,classes/hierarchy}`（公开只读）；前端类详情面板（结构 + 继承关系 + 下钻） | 单测（合成 class 字节 / 描述符 / 截断不 panic / 结构 / 继承 / 缓存失效 / 子类截断）+ `handler` 单测 + 集成扩展 + **真实上游扩展**（1.8.9：`MinecraftServer` 53 字段 / 129 方法 / class 版本 50；继承链 `aye → axu → avp → java.lang.Object`）全部通过；`vitest`（52 文件 / 507 例）全绿 |
| **横向（§18 / §20）** | OpenAPI 新增 `/admin/roles/*`（5）+ `/mcsrc/*`（4）路径与 `mcsrc` tag；`.env.example` 补 4 个 `MCSRC_*` 键；`builtintools` 与前端 `builtin-tools.ts` 同步新增 `mcsrc`（内置工具 19 项） | `js-yaml` 校验通过；`go vet` / `go test` / `vue-tsc` / `vitest`（52 文件 / 503 项）/ `vite build` 通过 |

> **未做（本批明确不做）**：mcsrc 切片 3（反编译源码浏览，依赖部署侧 JVM / Vineflower；以及引用查找 / 重命名映射 / 字节码指令 / Javadoc）。
> **环境提示**：迁移 `0021` / `0022` 已应用于集成测试库（启动时重建 schema 并应用全部 22 个迁移）与**开发库 `mc_tools`**（均已执行 `go run ./cmd/migrate`）。

### 4.0.7 本轮已完成与验证（2026-10-19，统一账号体系 · 迁移 0022）

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **统一账号迁移（§23）** | 迁移 `0022_accounts`：`admins` / `users` 合并为 `accounts`（`username` 全局唯一、`role` 外键引用 `roles.name` 且默认 `'user'`、`is_active` / `last_login_at` / 时间戳），数据搬迁（管理员保留原角色、用户固定 `role='user'`），**8 个外键重指**（`audit_logs.actor_admin_id`、`resources.created_by/updated_by`、`wiki_entries.created_by/updated_by` → `accounts` ON DELETE SET NULL；`user_favorites/user_recents/user_seeds.user_id` → `accounts` ON DELETE CASCADE），末尾 `DROP TABLE admins` / `users`；含**反向 SQL** | 开发库 `mc_tools` 实测：版本 **22**、表 **43**、`accounts` 1 行（`admin`/`role=admin`）、8 个外键 `confrelid=accounts`（`n`=SET NULL / `c`=CASCADE）、`to_regclass('admins'|'users')` 均为空；集成测试库每次由迁移重建 |
| **领域模型与角色隔离** | 新增 `domain.Account`（`accounts`）；`domain.Admin` / `domain.User` 改为 `Account` 的**类型别名**（保留历史命名，避免大范围改名）；后台侧查询一律 `role <> 'user'`，用户侧一律 `role = 'user'`（仓储层显式过滤，杜绝越权与串号） | `service/admin.go`、`service/adminaccount`、`service/useradmin`、`service/user/repo.go`、`memory_repo.go` 全部叠加角色过滤；新增 `adminaccount.ErrReservedRole` 拒绝经后台接口创建 `role='user'` |
| **双入口 + 单一会话** | 保留 `/admin/login` 与 `/login` 双入口，但**共用 `accounts` 与同一个会话 Cookie**（`SESSION_COOKIE_NAME`，默认 `mc_session`；`USER_SESSION_COOKIE_NAME` 废弃）；`auth.Session.AdminID` → `AccountID`、`DeleteByAdminID` → `DeleteByAccountID`；删除 `middleware/user.go` 的独立 Cookie 逻辑，改为 `RequireUserRole()`（`role≠user` → 401）；`/user/*` = `RequireAuth + RequireUserRole + CSRF`；`/admin/auth/{me,change-password}` 对 `role=user` 会话返回 401；登录处理器 Cookie 名统一取 `middleware.SessionCookieName()`（修掉原先「处理器硬编码常量、中间件读配置」的隐患） | 集成测试 `TestUnifiedAccountBoundaries`（9 项：注册固定 `role=user` / 普通用户登后台 401 / 后台账号登前台 401 / 统一 Cookie / `/admin/auth/me` 401 / 后台接口 403 / 后台会话访问 `/user/me` 401 / 普通用户列表只含普通用户 / 管理员列表不含普通用户且拒绝创建 `role=user`）通过 |
| **横向** | `.env.example` 删除 `USER_SESSION_COOKIE_NAME` 并把 `SESSION_COOKIE_NAME` 默认值改为 `mc_session`；`openapi.yaml` `cookieAuth` 与 `/user/login` 描述改为统一会话；`Function.md` §2.15 / §4.5 / §4.7 / §4.10 / §4.11 / §五 / §7.1 / §九 与 `PROJECT.md` §8.4 / §4.1 同步 | `go vet` / `go test ./...`（全绿）/ `vue-tsc` / `vitest`（52 文件 / 503 例）/ `vite build` 通过；运行中服务 curl 实测 6 项 |
| **顺带修复（测试隔离）** | ① `cmd/api` 的 `resetData` 在 TRUNCATE 前先 `itCollector.Flush()`：统计采集器是包级单例，内存缓冲跨用例累积，曾致 `TestUsageStatsAndAnalytics` 全量运行时偶发失败（`resource_views` 期望 1 实际 4）；② `internal/service/linkhealth` 集成测试改用专用 schema（`linkhealth_it`，DSN 追加 `search_path` 运行时参数）：与 `cmd/api` 共用同一测试库时，后者的 `DROP SCHEMA public CASCADE` 会打断前者，曾致 `TestRunCheckAndConfirmIntegration` 偶发失败 | `go test ./...` 连续 2 次并行全绿（此前必须 `-p 1` 或靠运气） |

> **运行中服务实测（`go run ./cmd/api`，开发库 `mc_tools`）**：匿名 `/user/me` → 401；匿名 `/admin/auth/me` → 401；后台账号从前台登录 → 401（角色过滤生效）；注册普通用户 → 200；普通用户前台登录 → 200（Cookie 名沿用开发库 `.env` 的 `SESSION_COOKIE_NAME=mc_admin_session`）；用户会话 `/user/me` → 200、`/admin/auth/me` → 401、`/admin/users/list` → 403。验证用临时账号已删除，开发库仅剩真实 `admin`。
> **注意**：会话为**内存存储**，服务重启后所有登录态失效（含浏览器中既有后台会话）；统一账号迁移后浏览器需重新登录。

### 4.0.8 本轮已完成与验证（2026-10-19，mcsrc 切片 2：类结构视图 + 继承关系）

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **class 字节码解析（纯 Go，无 JVM）** | `internal/mcsrc/classfile.go`：手写 JVM class 文件解析（JVMS §4）——常量池全 tag（含 Long / Double 双槽位）、`access_flags` / 本类 / 父类 / 接口 / 字段 / 方法；属性表按长度跳过（结构视图不需要 `Code` 正文）。所有读取经边界检查并累积错误，**损坏 / 伪造字节不 panic** | 单测：合成 class 字节（自建最小合法 class 构造器）解析出本类 / 父类 / 接口 / 字段 / 方法 / 版本；`super_class=0` 归一化为 `java.lang.Object`；种类判定（类 / 接口 / 枚举 / 注解）；**魔数错误、未知常量池 tag、逐字节截断（0..n）均不 panic** |
| **描述符渲染** | `internal/mcsrc/descriptor.go`：JVM 描述符 → Java 类型 / 方法签名（基础类型、数组、内部类名 `a/b/C$D` → `a.b.C$D`）；`void` **仅允许作返回类型**（字段 / 参数出现即报错，与 JVMS §4.3 一致）；修饰符按类 / 字段 / 方法三套语义渲染 | 表驱动单测：`[I` → `int[]`、`[[Ljava/lang/Object;` → `java.lang.Object[][]`、`(Ljava/lang/String;I[Z)Ljava/util/List;` → 参数 3 个 + 返回 `java.util.List`；`""` / `X` / `Ljava/lang/String` / `II` / `[V` / `(I)VV` / `(V)V` 均报错；接口不重复输出 `abstract` |
| **结构视图 + 继承关系** | `internal/mcsrc/structure.go`：`ClassStructure`（点分 / 斜杠类名等价、`in_jar_super` / `in_jar_interfaces` 只查 zip 中央目录，**与 jar 体积无关**）+ `Hierarchy`（祖先链 + 直接子类（含实现本接口的类）+ 直接 / 递归计数）；关系索引按 **jar 摘要缓存于内存**（最多 4 个版本，摘要变化自动失效）+ 单飞锁；损坏 class 条目跳过而非整体失败；单 class 解析上限 16 MiB、子类列表上限 500（超出标 `truncated` 但计数仍准确）、祖先链深度上限 64（防成环） | 单测：继承树 `a.Base → a.Mid → a.Leaf` + `a.Side` + `a.Iface`/`a.Impl` 的双向验证（祖先链 `[a.Mid, a.Base, java.lang.Object]`、`a.Base` 直接子类 `[a.Mid, a.Side]`、递归 3、接口子类 = 实现类）；类不存在 → `CLASS_NOT_FOUND`、类名非法 → 422、版本未准备 → 404、未建索引 → 409；**jar 变更后关系索引失效重建**；**超上限截断仍报准确计数**；损坏 class 字节 → `ErrBadClassFile` 且关系索引跳过该条目 |
| **接口（公开只读）** | `POST /api/v1/mcsrc/classes/detail`、`POST /api/v1/mcsrc/classes/hierarchy`（与检索同口径，不触发下载）；错误码：未缓存 jar → 404、类不在 jar → 404 `CLASS_NOT_FOUND`、未建索引 → 409 `NEED_PREPARE`、缺参 / 非法类名 → 422、class 过大 → 413 `CLASS_TOO_LARGE` | `handler` 单测：未准备版本 404、缺参 422、准备后详情 200（父类 / 字段 / 方法渲染）、斜杠写法等价、类不存在 404 `CLASS_NOT_FOUND`、非法类名 422、继承关系祖先链与子类、未配置 503；集成测试 `TestMcsrcRoutesAuthGating` 新增两项只读接口的 404 / 422 边界 |
| **前端** | `McsrcView.vue` 类详情面板：修饰符 / 种类 / class 版本（major → `Java 8`）/ 父类 / 直接接口 / 继承关系（祖先链与子类**均可点击继续下钻**，jar 外终点不可点）/ 字段表 / 方法表（`bridge` / `synthetic` 成员**默认折叠**，可勾选显示）；切换版本 / 侧别 / 翻页自动关闭详情 | `tests/mcsrc.spec.ts` 新增 4 例（共 9 例）：点击类名打开详情并渲染结构 + 继承关系 + 合成方法折叠、勾选显示合成方法且点击祖先下钻、**继承关系接口失败不阻塞结构展示**、结构加载失败可返回列表 |
| **真实上游验证** | `TestLivePrepareRealJar`（`MCSRC_LIVE_TEST=1`）扩展：真实 jar 全量解析 + 关系索引 | 实测 1.8.9 客户端 jar：`net.minecraft.server.MinecraftServer` → 53 字段 / 129 方法 / class 版本 50（Java 6）、父类 `java.lang.Object`；继承关系实测 `aye extends axu extends avp extends java.lang.Object`，**反向验证父类子类列表包含该子类**；jar 外类 `java.lang.Object` → `CLASS_NOT_FOUND`；二次调用结果一致 |
| **横向** | `openapi.yaml` 新增两个 path（含错误码说明）；`Function.md` §2.26 / §4 接口表 / §九 状态表 / §7.1 测试表；`todo.md` §22 勾选与验收记录 | `go vet ./...` 通过；`go test ./...`（含集成）全绿；`vue-tsc --noEmit` 通过；`vitest run` **52 文件 / 507 例**全绿；`vite build` 通过；`ruby -ryaml` 解析 OpenAPI 通过（136 paths） |

> **切片 3 待办**：跳转定义与查找引用、重命名映射展示、字节码指令视图、Javadoc 备注。

### 4.0.9 本轮已完成与验证（2026-10-19，mcsrc 切片 3-D1：源码浏览 · 浏览器内 Vineflower WASM）

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **路线修正（关键）** | 逐文件核对 `mcsrc-main` 反编译链路后确认：上游**不在服务端反编译**，而是在 Web Worker 内运行 **Vineflower 的 WASM/JS 移植版**（npm `@run-slicer/vf`，Apache-2.0/MIT，别名 `vf-1.12.0`）。故原「依赖部署侧 JVM」的阻塞项**作废**，直接复用同一引擎 | 依赖可达性实测：`@run-slicer/vf` `0.6.4-1.12.0`（5.3 MB，含 `vf.wasm` / `vf.wasm-runtime.js` / `vf.runtime.js` 纯 JS 回退）、`@katana-project/zip` `0.7.3`（`DecompressionStream` 解 deflate-raw）、`comlink` `4.4.2`、`monaco-editor`（0.56 起子路径需按 `"./*": "./esm/vs/*.js"` 写短路径）。已排除：wazero 跑上游 TeaVM WasmGC 产物（wazero 不支持 WasmGC）、纯 Go 自研反编译器（无生产级实现） |
| **后端：jar 下发** | `Store.OpenJar`（复用 `normalize` 做版本号白名单校验防路径穿越；未缓存 → `ErrNotFound`）+ `McsrcHandler.Jar`（`http.ServeContent` 流式响应，`ETag` = 官方 sha1、`Cache-Control: public, max-age=86400`、`Content-Type: application/java-archive`）+ 路由 `GET /api/v1/mcsrc/jar`（**公开只读**，本模块唯一 GET：二进制下载需 `Range`/`ETag` 语义） | `handler` 单测 `TestMcsrcJarDownload`：未准备 404 / 缺 `version` 422 / 非法版本（`../etc/passwd`）422 / 准备后 200 且字节与缓存 sha1·size 一致 + `Content-Type` + `ETag` / `Range: bytes=0-3` → 206；集成 `TestMcsrcRoutesAuthGating` 新增 `GET /mcsrc/jar` 未缓存版本 404（用绝不会被缓存的版本号，规避共享临时缓存目录里的历史 jar）与缺 `version` 422 |
| **前端：反编译引擎** | `src/mcsrc/vineflower.ts`（**显式** `import wasmUrl from 'vf-1.12.0/vf.wasm?url'` 再交给 `load(wasmUrl, {noAutoImports:true})`，避免依赖预打包下包内 `new URL('./vf.wasm', import.meta.url)` 不被重写为产物 URL；失败回退 `vf.runtime.js` 纯 JS）、`decompile.worker.ts`（Comlink 暴露；`@katana-project/zip` 在 Worker 内解析 zip 并按 jar 键缓存条目表；`source` / `resources` / `options` 调用形态与上游一致）、`decompiler.ts`（懒创建 Worker；jar 按 `(version, side)` 只下载一次、失败不缓存、并发去重）、`names.ts`（点号 ⇄ 斜杠类名）、`monaco.ts`（`MonacoEnvironment.getWorker` 仅注册 editor worker）+ `components/mcsrc/SourceViewer.vue`（只读 Java 查看器，跟随站点明暗主题） | `tests/mcsrc-decompiler.spec.ts`（5 例，mock Worker/comlink）：jar 只下载一次、按侧别分别缓存、失败不缓存可重试、并发只下载一次、缓存键与类名转换（含内部类 `Level$ExplosionInteraction`） |
| **前端：页面接入** | `McsrcView.vue` 类列表与类详情面板均新增「查看源码」→ 源码视图三态（反编译中 / 失败 / Monaco 源码）+「返回列表」；切换版本 / 侧别 / 翻页自动关闭；`SourceViewer` 用 `defineAsyncComponent`、Worker 懒创建——**不打开源码面板不载入 Monaco 与 Worker** | `tests/mcsrc-source.spec.ts`（5 例，mock 反编译客户端与 Monaco 查看器）：列表入口触发 `decompileClass(version, side, className)` 并渲染源码、反编译中加载态、失败态可返回列表、详情面板入口被源码面板替换、切换版本关闭面板 |
| **构建配置** | `vite.config.ts` 新增 `worker: { format: 'es' }`（Worker 内有动态 import 回退，产生代码分割，默认 iife 不支持） | `vue-tsc` 通过；`vitest run` **54 文件 / 517 例**全绿；`vite build` 通过（`SourceViewer` 3.96 MB / `vf.wasm` 3.77 MB / `vf.runtime` 1.35 MB 均为懒加载分块） |
| **真实上游端到端** | 临时程序：真实下载 1.8.9 客户端 jar → httptest 起 handler 验证 jar 端点 → 用与 Worker 相同的引擎与调用形状在 Node 24 反编译 | jar 端点：`size=8461484`、`sha1=3870888a…`、`GET /jar` 200 且 sha1 与缓存摘要一致、`Range: bytes=0-3` → 206（`504b0304`）、`If-None-Match` → 304、未缓存版本 → 404；**反编译**：2507 个类，反编译 `net.minecraft.server.MinecraftServer` 成功（约 1 s，产出含 `class MinecraftServer` 与 `tick` 方法的真实 Java 源码） |

> **验证边界**：本机无浏览器 / Playwright，界面交互只做到单测层面；但**反编译引擎本身**已用真实 1.8.9 jar 在 Node 上跑通（同一 `@run-slicer/vf` 与相同 `decompile` 调用形状），后端 jar 端点用真实 8.46 MB jar 端到端验证（含 Range / ETag）。
> **运行环境**：`go run ./cmd/api`（`:8080`）已重启并注册 `GET /mcsrc/jar`；实测缺 `version` → 422、未缓存版本 → 404。

### 4.0.10 本轮已完成与验证（2026-10-19，mcsrc 切片 3-D2：token 着色 + 点击跳转）

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **上游语义对齐** | 逐文件核对 `mcsrc-main` 的 `src/logic/{Tokens,FindDeclaration}.ts`、`src/ui/{Code,CodeUtils,CodeExtensions}.tsx`，确认 token 模型（`class` / `field` / `method` / `parameter` / `local` + `declaration` 标记 + `className` / `name` / `descriptor`）、装饰方式（`inlineClassName` 分色 + 可跳转手型）与跳转机制（Monaco **定义提供器 + 编辑器打开器** ⇒ Ctrl/⌘ + 点击 / F12 / 右键「转到定义」）；字段与方法的声明定位为「先看引用点所属类，未命中再沿祖先链逐级向上」 | 结论：**不需要新增后端接口**——声明定位所需数据（`classes/detail` / `classes/hierarchy`）切片 2 已交付，本切片是纯前端 |
| **Worker：token 收集** | `decompile.worker.ts` 向引擎传 `tokenCollector`（`start` / `visitClass` / `visitField` / `visitMethod` / `visitParameter` / `visitLocal` / `end`）按源码字符区间产出 token，另为 import 行补 class token，排序后随源码返回；`decompiler.ts` 新增**按 `(version, side, className)` 的反编译结果缓存**（跳转来回不重复反编译，`clearJarCache` 同时清空 jar 与结果缓存） | `tests/mcsrc-decompiler.spec.ts` 新增「同一类重复反编译命中结果缓存，`clearJarCache` 同时清空两类缓存」；`tests/mcsrc-source.spec.ts` 断言 token 数量透传给查看器 |
| **类名对齐（关键修正）** | 实测确认：Vineflower `visitClass` 对内部类给出**内部名 + `$`**（`adp$a`），而**源码文本**里内部类写作 `adp.a`（Java 语法），jar 条目与后端索引是 `adp$a`。故新增 `names.ts#classLookupCandidates()`（原名 → 最外层类 → 逐级把包分隔符换成 `$`），Worker 用 jar 条目集合探测后把 token 的 `className` 统一到后端约定，并据此标注 `inJar`（仅 jar 内类可跳转） | `tests/mcsrc-decompiler.spec.ts` 新增类名探测序列用例；**真实 jar 验证**：内部类引用 `adp.a` → `adp$a`、`MinecraftServer$1` 均对齐，`inJar=true` 的类名 100% 能在 jar 中命中 |
| **跳转解析** | 新增 `src/mcsrc/navigation.ts#resolveDeclaration()`：类 token 直接返回自身；字段 / 方法 token 先用 `classes/detail` 查引用点所属类是否声明该成员（方法按 `name + descriptor` 区分重载、字段按名），未命中再按 `classes/hierarchy` 祖先链逐级向上，返回**声明类 + 定位目标**；类不在 jar 内（索引 404）视为「不声明」继续向上，最终回退到引用点所属类；成员集合按类缓存 | `tests/mcsrc-navigation.spec.ts` 6 例：类 token 直通不查索引、成员在引用点所属类命中、沿祖先链定位、字段按名匹配、全部未命中回退、索引缺失不抛异常且成员集合命中缓存只查一次 |
| **前端接入** | `SourceViewer.vue`：token 装饰（`inlineClassName` 分色 + 可跳转 token 手型光标与点状下划线）、Java 定义提供器（返回 `mcsrc-goto://` 自定义 URI，片段为 `method:name:descriptor`）、悬停提示（显示跳转目标与快捷键）、编辑器打开器（解析 URI → 抛 `jump` 事件）、成员声明定位（`setSelection` + `revealRangeInCenter`）；`McsrcView.vue`：收到 `jump` 后解析声明类并打开源码（目标即当前类时只重新定位、不重复反编译）+ 面板顶部跳转提示；`style.css`：明暗双主题 token 配色 | `tests/mcsrc-source.spec.ts` 新增 3 例（类 token 打开目标类、方法引用沿继承链定位到父类声明并传递落点、同类内跳转不重复反编译）+ 跳转提示断言；`tests/mcsrc-tokens.spec.ts` 6 例（偏移命中与越界、行列 ⇄ 偏移、跳转片段编解码、声明定位含重载区分） |
| **真实上游验证** | Node 24 + 真实 1.8.9 客户端 jar（2507 类）+ 真实 Vineflower 纯 JS 运行时，逐段复刻 Worker 逻辑跑 token 全链路 | 反编译 `net.minecraft.server.MinecraftServer`（31,093 字符）产出 **1493 个 token**（引擎 1457 + import 36：class 309 / method 450 / field 311 / parameter 156 / local 267）；抽样 8 类 token 的区间文本与源码一致、**越界 0**、`inJar` 标注与 jar 条目完全一致（**1231 个可跳转**）、类名无残留斜杠、`inJar=true` 的类名均命中 jar、方法 / 字段描述符格式全部合法、内部类引用已对齐 `$` 形式 |
| **横向** | `Function.md` §2.26（标题 + 新增「源码 token 与着色」「点击跳转」「类名对齐」行 + 前端页面 / 分期路线 / 与上游差异说明）、§7.1 测试表、§九 状态表；`todo.md` §22（切片 3-D2 验收块 + 勾选）、§18 计数；`PROJECT.md` §4.1 | `vue-tsc --noEmit` 通过；`vitest run` **56 文件 / 534 例**全绿；`vite build` 通过；本切片**未改后端代码**，`openapi.yaml` 无新增 path |

> **验证边界**：本机无浏览器 / Playwright，Monaco 装饰与点击交互只做到单测层面（`SourceViewer` 在单测中被替换为桩组件，断言 token 透传、跳转事件与落点）；但 token 收集链路已用真实 jar + 真实引擎逐段复刻验证（见上表），跳转解析逻辑为纯函数级单测覆盖。

### 4.0.11 本轮已完成与验证（2026-10-19，页脚「常用工具」收敛）

| 批次 | 交付内容 | 验收要点 |
|---|---|---|
| **页脚「常用工具」** | `AppFooter.vue` 原先 `v-for` 渲染全部内置工具（19 项，页脚被撑得过长），改为只列 `BUILTIN_TOOLS.slice(0, 4)`（合成配方 / 坐标换算 / 物品 ID / 药水与酿造）；完整清单走工具中心 | 新增 `tests/app-footer.spec.ts` 2 例：常用工具恰好 4 项且与清单前 4 项一致、第 5 项起不出现在页脚；另断言站点信息 4 项与非官方声明未被破坏 |

## 4.1 结论速览（按优先级）

| 级别 | 事项 | 归属 | 依据 |
|---|---|---|---|
| **P1** | ✅ §21 开发栏目（开发文档 / 版本映射查询）**已验收**：迁移 `0013`、`cmd/seed -type devdocs`（40 条）、六个接口 curl、前端 `typecheck` / `vitest` / `build` 均通过 | 工程 | §4.0.4 / §4.0.5 |
| **P1** | ✅ 全站搜索已接入 `tsvector`（生成列 + GIN 索引），保留 `ILIKE` 兜底 | 后端 | §5.1 / §7.11 |
| **P1** | ✅ `sitemap.xml` 已动态收录资源/百科/工具详情页（含内置工具页） | 后端 | §9.3 |
| **P1** | 无 CI 流水线；后端集成测试默认跳过，未纳入自动化（集成测试已可手动注入 `TEST_DATABASE_URL` 实跑） | 工程 | §13.2 / §14 |
| **P1** | ✅ 开发文档已接入全站搜索（`type=devdoc`）：迁移 `0020` 建 `dev_docs.search_vector` + GIN，`internal/search` 命中标题 / 摘要 / 正文 / 标签，仅返回已发布 | 后端 | §5.1 / §21 |
| **P1** | ✅ 迁移（干净库 22 个 / 43 表）与备份恢复（14 表行数一致）**已实测**；Docker Compose 全链路未在真机验收 | 部署 | §15 / §20 |
| **P2** | ✅ MC 源码浏览器已实现切片 1 + 2 + 3-D1 + **3-D2**（版本选择 + jar 缓存 + 类索引 / 检索 + 类结构 / 继承关系 + 源码浏览：`GET /mcsrc/jar` 下发缓存 jar、浏览器内 Vineflower WASM 反编译、Monaco 展示 + **token 着色与点击跳转**：Ctrl/⌘ 点击 / F12 跳转定义，字段与方法沿继承链定位声明类，**无需部署侧 JVM**）；切片 3 余项（查找引用 / 字节码指令 / 映射重命名 / Javadoc）待办 | 后端 + 前端 | §22 / §4.0.6–§4.0.10 |
| **P2** | ✅ 统一账号体系（合并 `admins` / `users` 为 `accounts` + `role` 外键引用 `roles.name` + 8 个外键重指 + 删旧表 + 单一会话 Cookie）**已验收**（迁移 0022） | 后端 | §23 / §4.0.7 |
| **P2** | 标签仅存 JSONB，未建 `resource_tags/tool_tags/wiki_tags` 关联表 | 后端 | §7.8 |
| **P2** | 迁移无回滚命令（仅注释给出反向 SQL） | 后端 | §7 |
| **P2** | 组件 `AdminTable.vue`、`SearchResults.vue` 缺失 | 前端 | §4.3 |
| **P2** | ✅ editor 前端已隐藏 admin 专属入口（按 `auth.isAdmin` 过滤），后端仍 403 兜底 | 前端 | §6.2 |
| **P2** | ✅ 前端测试已补齐（搜索/筛选/分页状态、表单校验与 API 错误、管理路由未登录等），**47 文件 / 476 项**全绿 | 前端 | §13.3 |
| **P2** | ✅ 进度/待办文档已同步（`PROJECT.md` / `Function.md` / `todo.md`）；根目录遗留 `1.py`、`go.mod.root.bak` 仍未清理 | 文档/清理 | — |
| **P3** | 前端镜像用 `npm install` 而非 `npm ci`；Caddyfile 仍为 `:80` | 部署 | §11.4 |

## 4.2 需求维度未完成项

### P1-1. 全站搜索未落地 PostgreSQL 全文检索
- **现状**：`backend/internal/search/search.go` 全部使用 `ILIKE '%kw%'`，无 `tsvector`/`tsquery`，无 GIN/BTREE 搜索索引。
- **要求**：§5.1「首期使用 PostgreSQL `tsvector`/`tsquery` 与必要的 `ILIKE` 别名匹配；建立相应索引」；§7.11。
- **影响**：功能可用，但数据量增长后性能不达标（§11.3 列表 API P95 ≤ 500ms 存疑）。
- **建议**：迁移新增 `search_vector tsvector`（生成列/触发器）+ `GIN` 索引；查询改用 `search_vector @@ plainto_tsquery(...)`，保留 `ILIKE` 兜底；补 `tools(name)`、`resources(name)`、`wiki_entries(title)`、`items(item_id,name_zh)` 索引。

### P1-2. `sitemap.xml` 未覆盖动态详情页
- **现状**：`backend/internal/http/handler/seo.go` 的 `SitemapIndex` 硬编码 11 条静态路径，不含 `/search`、`/versions`、`/block-ids`、`/seed-map`、`/seed-tools`、`/region-inspect`，也不含资源/百科/工具详情页。
- **要求**：§9.3。
- **影响**：详情页无法被搜索引擎发现。
- **建议**：改为基于数据库查询（已发布且未软删除的 slug）+ 静态页。

### P1-3. 无 CI，集成测试未自动化
- **现状**：无 `.github/`、无 Makefile；`integration_test.go` 未设 `TEST_DATABASE_URL` 时 `t.Skip`，`go test ./...` 实际跳过全部集成测试。
- **要求**：§13.2 / §14 阶段 F。
- **影响**：未登录拦截、editor 越权、CSRF、草稿可见性、SSRF 等关键回归不在流水线中。
- **建议**：新增 CI：起 PostgreSQL → `go vet` / `go test ./...`（注入 `TEST_DATABASE_URL`）/ 前端 `npm ci && npm run typecheck && npm run test && npm run build`。

### P2-4. 标签关联表未建立
- **现状**：`migrations/0002_content.sql` 标签为 `JSONB`，无 `resource_tags/tool_tags/wiki_tags`。
- **要求**：§7.8。
- **影响**：按标签可靠筛选/统计退化为 `tags::text LIKE`，易误匹配。
- **建议**：新增关联表 + 迁移；admin 保存时同步写入。

### P2-5. 迁移无回滚命令
- **现状**：`cmd/migrate/main.go` + `internal/repository/db.go` 仅支持向前迁移；各 SQL 文件仅在注释中描述反向 SQL，无 `down`/`--to`。
- **要求**：§7「提供回滚策略」。
- **建议**：补 `*.down.sql` + `migrate --down N` / `--to <version>`；或在 README 明确"手工反向 SQL"策略。

## 4.3 前端未完成项

### P2-6. 通用组件缺失（§4.3 清单）
- `components/admin/AdminTable.vue` 不存在（各 admin 视图内联表格）。
- `components/search/SearchResults.vue` 不存在（由 `views/SearchResultsView.vue` 内联实现）。
- **建议**：抽出 `AdminTable`；`SearchResults` 若仅一处使用可保留内联并在文档注明偏离。

### P2-7. editor 角色前端入口未收敛
- **现状**：`components/admin/AdminSidebar.vue` 的"操作日志"对所有登录用户可见；`stores/auth.ts` 已有 `isAdmin` 未用于隐藏。
- **要求**：§6.2。
- **建议**：侧边栏按 `auth.isAdmin` 过滤；或点击后给出友好"无权限"提示。

### P2-8. 前端测试覆盖缺口（§13.3）
- 现有 11 个 spec：`coordinate / seed / seed-map / seed-map-view / region-file / markdown / storage / theme / copy-button / block-ids / routes`。
- **缺失**：① 搜索/筛选/分页状态同步（URL query ↔ 列表）；② 表单校验失败与 API 错误态；③ 管理路由未登录体验。
- **建议**：补 `tests/search-state.spec.ts`、`tests/form-validation.spec.ts`，扩展 `routes.spec.ts`。

## 4.4 部署与运维未完成项

### P1-9. 未做实机验收（§15 清单基本未勾选）
- Compose 全链路未验证；迁移"从空库完整执行"仅由集成测试间接覆盖；备份/恢复无演练记录。
- **建议**：按 `README.md` 完整跑一遍 Compose 部署 + 迁移 + `create-admin` + 备份恢复演练，逐项勾选 §15。

### P2-10. 构建与网关细节
- `frontend/Dockerfile` 使用 `RUN npm install`（非 `npm ci`）。
- `deploy/Caddyfile` 仍为 `:80`（生产需替换域名并启用 HTTPS）。
- **建议**：改 `npm ci`；部署手册固化域名/HTTPS 步骤。

## 4.5 文档与仓库清理

### P2-11. 进度/待办文档滞后
- 本次整合已将 `开发需求.md` / `PROGRESS.md` / `BACKEND_TODO.md` / `todo.md` 合并为 `PROJECT.md`，原文件归档至 `bak/`。
- `frontend/SEED_TODO.md` 仍有未完成项：出生点近似算法（`utils/seed/` 无实现，需实现或删条）、sitemap 收录（依赖 P1-2/P1-3）、headless 浏览器渲染验证。
- `README.md` 的"运行测试"段仍写"前端（待前端就绪）"，实际已就绪。

### P2-12. 根目录遗留文件
- `1.py`：需求文档生成脚本 → 移入 `scripts/` 或删除。
- `go.mod.root.bak`：根目录 Go 模块备份残留（真实模块在 `backend/go.mod`）→ 删除。
- `.env` 已存在且被 `.gitignore` 忽略（合规），注意不要提交。

## 4.6 建议执行顺序

1. **P1-2 / P1-3**：sitemap 动态化 + 补 CI（收益高、改动小）。
2. **P1-1**：搜索 tsvector 化 + 建索引。
3. **P1-9**：Compose 全链路验收 + 备份恢复演练，勾选 §15。
4. **P2-6 / P2-7 / P2-8**：组件补齐、角色入口收敛、前端测试补全。
5. **P2-4 / P2-5**：标签关联表、迁移回滚能力。
6. **P2-11 / P2-12**：文档同步与仓库清理（收尾）。

## 4.7 验收清单状态（对应需求 §15）

| 验收项 | 状态 | 说明 |
|---|---|---|
| `docker compose up --build` 可启动 | 🟡 部分验证 | postgres 容器在跑；后端 / 前端 / Caddy 全链路未在真机验收 |
| 迁移可从空库完整执行 | ✅ | 全新库 `cmd/migrate`：20 个迁移全部应用，43 张表 |
| 安全初始化管理员（无默认密码） | ✅ | `cmd/create-admin` |
| 未登录无法访问管理写接口 | ✅ | 集成测试 + 中间件 |
| editor 无法执行 admin 专属操作 | ✅ | 后端 403 + 前端入口按 `auth.isAdmin` 收敛 |
| 首页/工具/资源/详情响应式 | ✅ | 全部页面就绪（移动端真机走查待人工） |
| 坐标换算正确处理各类输入 | ✅ | 前后端均有测试 |
| 物品 ID / 配方展示版本与来源 | ✅ | 数据字段 + 页面 |
| 资源类型/版本/加载器筛选 | ✅ | `ResourcesView` |
| 来源链接可访问、失效可维护 | ✅ | 链接检查 + 后台复查与人工确认 |
| URL 检查阻止内网/回环/元数据/危险重定向 | ✅ | `linkcheck_test` + 逐跳重定向校验 |
| 全站搜索分页与空结果提示 | ✅ | `tsvector` + GIN 索引 + `ILIKE` 兜底 |
| 收藏/最近使用本地损坏不崩溃 | ✅ | `utils/storage.ts` |
| API 统一错误格式/请求 ID/超时/限流 | ✅ | — |
| 日志无密码/令牌/密钥 | ✅ | 审计过滤 + `HandleError` 服务端日志（不含敏感值） |
| SEO 标题/描述/canonical/sitemap/robots | ✅ | 动态 sitemap 收录已发布详情页 |
| 版权/隐私/反馈入口可访问 | ✅ | — |
| 备份与恢复步骤经验证 | ✅ | 实测备份 → 独立库恢复，14 张核心表行数一致 |
| README 含开发/测试/部署/初始化/备份/排障 | ✅ | 已补齐导入章节与 `--type all` 依赖顺序 |
| 移动端真机走查 | ⬜ 待人工 | 模拟视口已走查，真机未验证 |

## 4.8 本次已完成（2026-10-15，功能优先）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **P1-2 sitemap 动态化** | 新增 `internal/service/sitemap`；`SEOHandler` 改为「静态页 + 数据库已发布详情页（资源/百科/工具，含软删除过滤、去重、lastmod）」；新增 `seo_test.go`（3 例） | ✅ 单测通过；待实机 |
| **P1-1 搜索全文检索** | 迁移 `0004_search_vector.sql`：为 tools/resources/wiki_entries/items 增加 `search_vector` 生成列 + GIN 索引；`search.go` 查询改为 `search_vector @@ plainto_tsquery('simple', ?) OR (ILIKE ...)` 兜底 | ✅ 编译/vet/单测通过；⚠️ **需执行迁移**，待实机 |
| **P2-7 editor 入口收敛** | `AdminSidebar.vue` 导航项标记 `adminOnly`，按 `auth.isAdmin` 过滤；新增 `tests/admin-sidebar.spec.ts`（2 例） | ✅ 单测通过 |
| **出生点近似算法** | 新增 `utils/seed/spawn.ts`（基于后端 1.18+ 群系采样，取最近有效出生群系）；`SeedMapView` 请求 `include_biomes` 并渲染近似出生点；新增 `tests/seed-spawn.spec.ts`（8 例） | ✅ 单测通过 |
| **§5.1 搜索关键词高亮** | 新增 `utils/highlight.ts`（纯函数切分，视图用 `<mark>` 渲染，不走 v-html，防 XSS）；`SearchResultsView`、`GlobalSearch` 接入；清理后端从不返回的 `highlighted_*` 死字段；新增 `tests/highlight.spec.ts`（9 例） | ✅ 单测通过 |
| **§5.7 资源版本/来源平台筛选** | `ResourcesView` 增加「游戏版本」筛选面板与「来源平台」输入，URL query 同步 + 重置页码；后端来源平台改为 `ILIKE` 部分匹配；新增 `tests/resources-filter.spec.ts`（3 例） | ✅ 单测通过 |

> 全量回归：后端 `go vet`/`go test` 通过；前端 `vue-tsc` 通过、`vitest` 15 文件 / 115 项全绿、`vite build` 通过。
> 未覆盖项（环境限制）：`0004` 迁移与动态 sitemap 需连库实机验证（本机 Docker/PostgreSQL 未运行）。

## 4.9 本次已完成（2026-10-16，功能优先）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **管理员账号管理 API/UI** | 后端新增 `internal/service/adminaccount`（列表/创建/改角色与启用状态/重置密码/删除，含「不可停用/删除自己」「不可移除最后一名 admin」「用户名唯一」「角色枚举」「密码≥12位」守卫）+ `handler/adminaccount.go` + `/api/v1/admin/admins/{list,create,update,reset-password,delete}`（`RequireRole(admin)` + CSRF）；前端 `api/admin-admins.ts`、`views/admin/AdminAdminsView.vue`、侧边栏「系统」分组、路由 `/admin/admins` | ✅ 集成测试 4 例 + 前端 API 契约测试 5 例通过 |
| **改密失效全会话** | `SessionStore` 新增 `DeleteByAdminID`；`auth.Service.ChangePassword` 改密后清理该管理员全部会话；`DeleteSessionsForAdmin` 供账号停用/改角色/重置密码/删除复用；前端新增 `views/admin/AdminProfileView.vue`（`/admin/profile`，自助改密成功后清本地状态并回登录页） | ✅ `auth` 单测 + 集成测试通过 |
| **文档/规范同步** | `openapi.yaml` 补 5 个 `/admin/admins/*` 端点、`AdminAccount` schema 与 `Conflict` 响应；`README` 补端点说明与账号管理排障 | ✅ 人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 通过；`go test ./...` 通过；连库集成测试 `TEST_DATABASE_URL` 全套通过。
> 前端 `vue-tsc -b --noEmit` 通过、`vitest` 16 文件 / 122 项全绿、`vite build` 通过。

## 4.10 本次已完成（2026-10-16，P0 §2 版本化资料 + §4 数据导入与质量监控）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **§2 版本化资料规则** | 新增纯函数包 `internal/versioning`：单版本（`1.21.1` / `24w14a` / `1.21-pre1`）与**范围声明**（`1.16-1.17` 闭区间、`1.18+` 开上界）的解析 / 比较 / 排序 / 校验（格式非法、重复、起点大于终点）；迁移 `0006_versioning_quality.sql` 增加 `recipes.supported_versions`、`recipes.last_verified_at`、`commands.source_url`、`commands.last_verified_at`；`domain.Recipe` / `domain.Command` 同步字段 | ✅ 表驱动单测 5 例（含范围与不合理范围）通过 |
| **§4 统一导入入口** | `cmd/seed` 新增 `--type all`（以 `--file` 为目录，按依赖顺序 items → recipes → commands → tools 导入）；四类导入器统一做必填 / 枚举 / 数值 / 时间 / 版本校验，失败逐行汇总；`--dry-run` 输出「新增 / 更新」动作词 | ✅ 单测 + 实机 `--type all` 全绿（12/5/3/9 条，0 失败） |
| **§4 导入任务与准确计数** | 新增表 `data_import_jobs`（来源 / 时间 / 总数 / 新增 / 更新 / 跳过 / 失败 / 错误摘要 / 执行者 / dry-run）；**修正 `RowsAffected` 无法区分插入与更新的问题**，改为显式预查业务主键，`created_count` / `updated_count` 现为准确值 | ✅ 实机重跑显示 `新增 0 / 更新 N`；`buildImportJob` 单测覆盖摘要截断 |
| **§4 数据质量概览** | 新增 `internal/service/dataquality`（客观检查项：缺失来源 / 缺失适用版本 / 超期未校验 / 版本非法 / 配方结果物品**悬空引用** / 近 30 天失败导入）；`handler/dataquality.go` + `/api/v1/admin/data-quality/overview`、`/api/v1/admin/data-import-jobs/list`（`RequireRole(admin)` + CSRF） | ✅ 集成测试 2 例（含 editor 403、未登录 401）+ curl 冒烟 200 通过 |
| **§4 后台页面** | 前端 `types/data-quality.ts`、`utils/data-quality.ts`（纯函数：严重级别、占比、分组、失败判定）、`api/admin-data-quality.ts`、`views/admin/AdminDataQualityView.vue`；侧边栏「运营 → 数据质量」（admin 专属）、路由 `/admin/data-quality` | ✅ `vue-tsc` + `vitest` 9 例 + `vite build` 通过 |
| **文档同步** | `Function.md` 新增 2.4 版本规则行、4.6 数据质量监控（原 4.6 权限模型顺延为 4.7）、API / 数据表 / 命令行 / 测试 / 状态表；`openapi.yaml` 新增 2 端点 + `DataQualityCheck` / `DataQualityOverview` / `DataImportJob` schema + `Forbidden` 响应；`todo.md` §2 / §4 勾选；`README` 导入章节与 `.env.example`（`IMPORT_OPERATOR`） | ✅ 人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 通过；`go test ./...` 通过（含 `TEST_DATABASE_URL` 集成：`cmd/api` 5 例新增断言全绿）；前端 `vue-tsc -b --noEmit` 通过、`vitest` 18 文件 / 142 项全绿、`vite build` 通过。
> 实机冒烟（本地 Docker PostgreSQL + `go run ./cmd/api`）：登录 → 数据质量概览 `200`、导入任务列表 `200`、未登录 `401`、物品详情 `200`（含编码冒号）。
> 未完成：版本切换的统一前端交互、结构资料独立版本字段（`todo.md` §2 保留未勾选，避免过度声明）。

---

## 4.11 本次已完成（2026-10-16，P0 §3 跨模块资料关联 + §5 搜索体验）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **§3 关联模型（有依据才直链）** | 明确策略：配方↔物品**数据驱动**直链；指令 / 百科 / 资源与物品之间无可靠元数据 → 只给「按名称检索」入口（`/search?q=…&type=…`、`/commands?q=…`），**不伪造关联** | ✅ `Function.md` 2.9 记录；人工核对 |
| **§3 配方 → 物品解析** | `internal/service/catalog` 新增 `RecipeItems` / `RecipeRefItems` / `ResolveItemRefs`（批量一次查询）；`/recipes` 每项返回 `resolved_items`，物品详情 `related_recipes.resolved_items` 同源；前端 `utils/item-link.ts` 渲染链接 | ✅ 单元测试 4 例 + 集成测试 1 例 + 前端 3 例通过 |
| **§3 有效性检查（防断链）** | 关联解析只返回 `items` 表中**真实存在**的物品；未收录 / 已删除物品不进入映射，前端回退为纯文本 ID；检索类入口只命中 `status=published` 内容 | ✅ 集成测试断言「未收录物品不得出现在 resolved_items」通过 |
| **§3 跨模块检索入口** | 物品详情、百科详情、资源详情新增检索入口区块（资源页保留版本 / 加载器 / 依赖展示；「相关整合包」以检索入口呈现，因 `dependencies` 为自由 JSONB 无规范结构） | ✅ 前端测试（含修正：指令入口此前误指向 `type=tool`，现指向 `/commands?q=`） |
| **§5 相关度与排序** | `internal/search` 新增 `rankInput` 与权重梯度（标题 100/80/60、命名空间 ID 95/70、别名 75/55、slug 45、摘要 20、兜底 5）；`Filter.Sort` 支持 `relevance`（默认）/ `updated_desc`；`rankItems` + `pageSlice` 纯函数 | ✅ 表驱动单测（含排序、分页切片）通过 |
| **§5 匿名搜索统计** | 迁移 `0007_search_terms.sql` 新建 `search_terms`（唯一 `term` ≤64、`hits`、`no_result_hits`、`last_result_count`、首末时间）+ 索引；`internal/service/searchstats` 提供 `Normalize` / `Record`（单条 UPSERT）/ `Hot` / `NoResult` / `Suggestions` / `List` | ✅ 单测（归一化 / 超长丢弃 / LIKE 转义）+ 集成测试通过 |
| **§5 无结果引导** | 无结果时 `meta.suggestions` 返回可尝试关键词（字面子串关系 + 热门词兜底，**不做语义猜测**）；前端搜索页展示可点击关键词并重搜 | ✅ 集成测试 + 前端 6 例通过 |
| **§5 输入防护** | 关键词上限 80 字符（超长 422 `VALIDATION_ERROR`）、查询 3 秒超时、异常请求体 400；前端输入框 `maxlength=80` | ✅ curl 冒烟 422 + 集成测试通过 |
| **§5 类型 / 版本筛选** | 搜索页新增**游戏版本**筛选与排序切换，状态同步 URL（可分享 / 可回退） | ✅ 前端 3 例通过 |
| **§5 查询失败统计（补充）** | 迁移 `0008_search_failures.sql` 增加 `failed_hits`；`RecordFailure` 与「无结果」**分开计数**（避免把故障误报为资料缺失）；管理端 `kind=failed` 视图 | ✅ 集成测试（失败 2 次 + 成功 1 次计数隔离）通过 |
| **§5 后台页面** | 前端 `types/search-term.ts`、`utils/search-term.ts`（占比 / 高频缺口判定）、`api/admin-search-terms.ts`、`views/admin/AdminSearchTermsView.vue`；侧边栏「运营 → 搜索词统计」、路由 `/admin/search-terms`；后端 `handler/searchterms.go` + `/api/v1/admin/search-terms/list`（admin + CSRF） | ✅ `vue-tsc` + `vitest` 6 例 + `vite build` 通过 |
| **文档同步** | `Function.md` 新增 2.9 跨模块关联、4.8 搜索词统计、搜索体验行、API / 数据表 / 测试 / 状态表；`openapi.yaml` 新增 `/admin/search-terms/list` 与 `ItemRef` / `Recipe` / `RecipeListEnvelope` / `SearchTerm` schema，`/search` 与 `/recipes` 契约更新；`todo.md` §3 / §5 勾选 | ✅ YAML 解析校验 + 人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 通过；`go test -count=1 ./...` 通过（含 `TEST_DATABASE_URL` 集成：新增 5 例并强化物品详情断言）。
> 前端：`vue-tsc -b --noEmit` 通过、`vitest` **22 文件 / 168 项**全绿、`vite build` 通过。
> 实机冒烟（本地 Docker PostgreSQL + `go run ./cmd/api`）：无结果建议 `200`（含 `suggestions`）、超长关键词 `422`、`sort=updated_desc` `200`、配方 `resolved_items` 仅含已收录物品、未登录管理端 `401`。
> 设计取舍：关联展示**宁缺毋滥**——无可靠元数据时只给检索入口，不生成可能失效的直链；搜索词统计**只存聚合计数**，不存 IP / 会话 / 用户标识 / 逐次时间序列。
> 未完成：`todo.md` §2 的「统一版本切换前端交互」「结构资料独立版本字段」仍保留未勾选。

---

## 4.12 本次已完成（2026-10-16，P0 §6 资源兼容性与来源信息）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **§6 统一字段与校验** | 资源类型 / 加载器 / 适用版本 / 游戏版本类型（java·bedrock）在**写入前校验与规范化**；`internal/service/resource/meta.go` 提供 `NormalizeGameEdition` / `NormalizeGameVersions`（复用 `versioning`）/ `NormalizeLoaderTypes` / `ValidResourceType` / `ValidResourceStatus` / `ValidHTTPURL`，导入器与后台接口**共用同一套规则** | ✅ 表驱动单测 5 组 + 集成测试（非法加载器 / 版本 / 重复版本 → 422）通过 |
| **§6 依赖 / 前置 / 已知冲突** | `dependencies` 规范为 `{name, type, resource_url?, required, note?}`，`type ∈ dependency·prerequisite·conflict`；缺省补 `dependency`（兼容历史数据），冲突项强制 `required=false`；上限 50 条；详情页按「前置 Mod → 依赖 → 已知冲突」分组，冲突项独立配色 | ✅ 单测（缺省补齐 / 非法 type / 非法链接 / 超限 / 脏数据降级）+ 集成测试（缺省 type 补齐断言）+ 前端 3 例通过 |
| **§6 发布版本关联** | 迁移 `0009_resource_releases.sql` 新建 `resource_releases`（`resource_id` 级联删除、`version_label` 资源内唯一、`game_versions` / `loader_types` / `source_url` / `released_at` / `notes` / `is_latest`）；`is_latest` 用**部分唯一索引**保证每个资源至多一条；服务层 `ListReleases` / `ReplaceReleases`（事务内删除 + 重建，幂等） | ✅ 单测（排序 / 重复标签 / 缺来源 / 多条最新 / 非法日期）+ 集成测试（整体替换 + 幂等重跑数量稳定）+ curl 冒烟（replace 200、多条 latest 422）通过 |
| **§6 兼容性「未验证」原则** | 结论**不落库**，`BuildCompatibility` 由资源级字段与发布版本实时推导；有数据 → `verified=true` + 合并去重排序后的版本 / 加载器 + `sources`（`resource` / `releases`）；无数据 → `verified=false` + `unverified_hint`，界面展示**未验证**徽标；切片恒为 `[]`（非 `null`）保证契约稳定 | ✅ 单测 6 例（含空切片契约）+ 集成测试（两种场景）+ 前端 4 例 + curl 冒烟通过 |
| **§6 来源 / 许可 / 作者展示** | 详情页新增「来源与许可信息」区块：作者、来源平台、许可信息、原始来源链接、**外链检查状态 + 最后检查时间**（未检查显示「未检查」）；仅客观呈现检查结果，不推断「安全」 | ✅ 前端 2 例通过 |
| **§6 发布版本前端** | 详情页「发布版本」区块（版本标签、最新标记、发布时间、适用版本 / 加载器徽标、备注、发布页链接）；后台编辑页新增发布版本行编辑器（「最新」互斥）+ 依赖行编辑器（关系类型下拉） | ✅ `vue-tsc` + `vitest` 3 例 + `vite build` 通过 |
| **§6 元数据批量导入** | `cmd/seed --type resources`（`--type all` 已纳入，顺序 items → recipes → commands → tools → resources）；按 `slug` 幂等 upsert，发布版本随资源替换；**校验复用服务层规范化函数**；`releases` 字段缺省 = 不触碰，显式空数组 = 清空；新增 `seeds/resources_demo.json`（4 条，含「无版本数据」样例用于演示未验证） | ✅ `cmd/seed` 单测 4 组 + 实机导入（4 条 → 幂等重跑 4 更新，发布版本 3/2/1/0 稳定）通过 |
| **文档同步** | `Function.md` 新增 2.6 资源兼容性与来源信息（并重排 2.7–2.10）、4.3 / 6.1 / 6.2 / 7.1 / 7.2 / 八 / 九 同步；`openapi.yaml` 新增 `Resource` / `ResourceDetail` / `ResourceRelease` / `ResourceReleaseInput` / `ResourceDependency(Type)` / `ResourceCompatibility` schema 与 2 个发布版本端点，`/resources/{slug}` 契约更新；`todo.md` §6 勾选 7 项并标注未完成项；`README.md` 导入说明更新 | ✅ YAML 解析 + 26 个 schema 引用校验通过；人工核对 |
| **未引入新环境变量** | 未新增配置项，`.env.example` 无需变更 | ✅ 人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 干净；改动文件 `gofmt -l` 干净；`go test -count=1 ./...` 全包 ok（含 `TEST_DATABASE_URL` 集成：新增 2 例）。
> 前端：`vue-tsc -b --noEmit` 通过、`vitest` **25 文件 / 213 项**全绿（新增 3 文件 45 项）、`vite build` 通过。
> 实机冒烟（本地 Docker PostgreSQL + `go run ./cmd/api`）：`/resources/jei` 返回 `compatibility.verified=true` + 3 条发布版本 + 依赖 type 规范化；`/resources/chunky` 返回 `verified=false` + `unverified_hint` 且切片为 `[]`；草稿资源 `404`；`releases/list` 200、`releases/replace` 200（含日期解析）、多条 `is_latest` 422、非法依赖 type 422、非法加载器 422；未登录 401、无 CSRF 403。
> 设计取舍：兼容性结论**宁缺毋滥**——无数据依据即「未验证」，绝不把缺失数据描述为兼容；发布版本采用**整体替换**而非增量合并，避免残留陈旧版本；导入器与服务层共用校验函数，杜绝两条写入路径规则漂移。
> 未完成（下一批 §6-B）：链接检查的**任务并发限制**与批量任务；**失效链接后台复查与人工确认流程**（超时与重定向上限已具备）。

---

## 4.13 本次已完成（2026-10-16，P0 §6-C 版本覆盖）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **§6-C「所有版本」通配** | `internal/versioning` 新增 `KindAll` + `AllKeyword = "all"`，`*` / `ALL` 归一化为 `all`；`CanonicalSpec` / `IsAll` / `ParseSpec` / `CompareSpec` / `Validate` / `Sort` 全链路支持，排序时 `all` 恒排最前；**必须独占出现**，与具体版本并列即 422 | ✅ 表驱动单测 6 例 + `meta_test` 通配规范化用例通过 |
| **§6-C 兼容性与筛选语义** | `Compatibility` 新增 `all_versions`（资源级或发布版本声明 `all` 即为 `true`）；资源列表与搜索的版本筛选新增 `versionFilter`：**具体版本命中「声明该版本」与「声明通配」两类**，`game_version=all` **仅命中声明通配者**（区别于「不限版本」= 不传参数） | ✅ 单测（`BuildCompatibility` 通配）+ 集成测试 5 断言（归一化 / 独占 422 / 具体筛选双命中 / all 筛选精确 / 搜索通配）通过 |
| **§6-C 版本选项覆盖全部 MC 版本** | 前端新增 `utils/mc-versions.ts`：`loadReleaseVersions`（取 `POST /versions` 正式版清单，**内存缓存 + 并发合并**，失败回退 `FALLBACK_RELEASE_VERSIONS`）、`majorVersions`（主版本归并去重）、`buildVersionOptions`（`includeAll` 时「全部 MC 版本」置顶）；资源中心 / 搜索结果改为动态主版本 + 通配项，配方查询改为完整正式版清单（精确匹配），后台表单提供 `<datalist>` 候选 | ✅ `vue-tsc` 通过；`vitest` 新增 `mc-versions.spec.ts` 9 例通过 |
| **§6-C 详情页展示** | 声明通配时详情页展示 **全部 MC 版本** 徽标（不再渲染为普通版本徽标，也不再误显示「未标明」），并提供「查看完整正式版清单」懒加载展开（首次点击才请求清单） | ✅ 前端 2 例（展开 / 不声明时不渲染入口）通过 |
| **§6-C 后台录入体验** | 「支持版本」与发布版本「适用游戏版本」提示通配 `all` 用法 + `datalist` 候选；新增 `validateVersionText`（**只做语义校验**——通配独占；格式交由后端统一裁决，避免前后端正则漂移误拦截），接入资源级与发布版本表单校验 | ✅ `resource-form.spec.ts` 新增 4 例通过 |
| **§6-C 演示数据与导入** | `seeds/resources_demo.json` 新增 AppleSkin 样例（资源级与发布版本均声明 `all`，用于演示通配与展开） | ✅ 实机导入 5 条 + 幂等重跑 5 更新通过 |
| **文档同步** | `Function.md` §2.4 / §2.6 补充通配与版本选项说明、§6.1 / 八 / 九 同步；`openapi.yaml` 新增 `ResourceCompatibility.all_versions`、`supported_versions` / `game_versions` / 版本筛选参数说明补充通配语义；`todo.md` §6 勾选 2 项；`README.md` 说明更新 | ✅ YAML 解析 + **38 个** `$ref` 校验通过（68 条路径）；人工核对 |
| **未引入新环境变量** | 复用既有 `POST /versions` 与兜底清单，未新增配置项 | ✅ 人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 干净；改动文件 `gofmt -l` 干净；`go test -count=1 ./...` 全包 ok（含 `TEST_DATABASE_URL` 集成：新增 1 例 5 断言）。
> 前端：`vue-tsc -b --noEmit` 通过、`vitest` **26 文件 / 233 项**全绿（新增 1 文件 9 项，补充 11 项）、`vite build` 通过。
> 实机冒烟：`/resources/appleskin` 返回 `compatibility.all_versions=true` 且 `game_versions=["all"]`；列表 `game_version=all` 仅命中通配资源、`game_version=1.20.1` 同时命中通配与具体声明；`supported_versions=["all","1.20.1"]` → 422。
> 设计取舍：通配声明**不落库为「已展开的版本列表」**（避免版本清单变动导致数据陈旧），而是作为**语义声明**存储、由前后端按需展开；筛选不做范围语义展开，仍以子串匹配 + 通配命中为准（`LIKE '%1.20%'` 天然可命中 `1.20.1` 声明，反向不命中，属可接受行为）。
> 未完成（下一批 §6-B）：链接检查的**任务并发限制**与批量任务；**失效链接后台复查与人工确认流程**。

---

## 4.14 本次已完成（2026-10-17，P1 §7 可视化指令生成器 + §8 附魔与装备计算器）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **§7 可视化指令生成器** | 新增内置工具 `/command-generator`：`utils/command-generator.ts` 纯函数（六类指令构建器 + 版本裁决 + 校验原语）+ `views/CommandGeneratorView.vue`；`/give` 按 `1.20.5` 区分旧式 NBT 与数据组件，`/effect` 按 `1.19` 区分 `give` 子命令，实体数据始终 NBT；快照 / 预览版按最新语法并标注；一键复制 / 清空 / 示例 / 逐字段错误定位 | ✅ 单测 29 例通过；`vue-tsc` / `vitest` / `vite build` 通过 |
| **§8 附魔与装备计算器** | 新增内置工具 `/enchant-calculator`：`utils/enchant-calculator.ts`（41 项附魔资料表 + 互斥分组 + 组合校验 + 铁砧 Java 花费公式 + 结果附魔推演 + 先作惩罚）+ `views/EnchantCalculatorView.vue`（组合校验 / 花费明细 / 结果附魔 / 惩罚对照 / 附魔资料表 / **方案对比快照**）；规则依据 Minecraft Wiki《Anvil mechanics》，未覆盖边界显式标注 | ✅ 单测 22 例通过（含 Wiki 公开示例 16 / 20 复算）；`vue-tsc` / `vitest` / `vite build` 通过 |
| **注册与 SEO** | 两个工具接入 `BUILTIN_TOOLS` 单一事实源（首页 / 页脚 / 工具中心 / 路由守卫自动生效）、`router/index.ts`、`seo.go` 静态 sitemap 页、`frontend/public/sitemap.xml`、`seeds/tools_builtin.json`（后台工具表种子） | ✅ `routes.spec` 自动校验新路由可解析；后端 `seo` 测试通过 |
| **文档同步** | `Function.md` 新增 2.11 / 2.12、内置工具计数 11 项、测试矩阵补两行、状态表补两行；`todo.md` §7 / §8 勾选与「推荐实施顺序」5 / 6 勾选 | ✅ 人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 干净、`go test ./...` 全包 ok；前端 `vue-tsc -b --noEmit` 通过、`vitest` **28 文件 / 286 项**全绿、`vite build` 通过。
> 未完成：`todo.md` §7「移动端验证表单操作与结果展示」仍保留未勾选（需真机 / 设备模拟走查）；§8 未覆盖的耐久修复量与附魔台花费等边界已在页面与代码显式标注。
> 设计取舍：两个工具均为**前端纯函数 + 原生控件**实现，不新增后端接口与依赖；无法精确裁决的版本（快照 / 预览版）按最新语法生成并**显式标注**，不把不确定结论写成确定事实。

## 4.15 本次已完成（2026-10-17，P1 §10 世界分析工具扩展）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **版本支持梳理（§10-1/8）** | `mcstructure/capabilities.go`：各能力（结构 / 群系 / 史莱姆 / 要塞 / 种子搜索）在四个版本区间的支持矩阵与说明；新增 `POST /mc/capabilities`；`PlacementSupported`（1.13+ 起分布算法与实现一致，`1.12` 及更早标注「不可作为依据（未核实）」） | ✅ `version_test.go` 覆盖矩阵与限制；curl 实跑 200 |
| **结构接口版本提示（§10-8）** | `POST /mc/structures` 返回 `available` / `placement_supported` / `supported` / `dimension` / `probability`；不支持时 `note` 明确提示「尚未加入 / 算法不同」，不把结果当确定结论 | ✅ `handler` 单测覆盖三类文案；curl 实跑（`1.8-1.12` village → `supported:false`） |
| **群系查询（§10-2）** | 新增 `POST /mc/biome`：按种子与方块坐标返回地表群系（`1.18+`，cubiomes `SAMPLE_NO_DEPTH`）；其他版本返回 `supported=false` 与说明 | ✅ `handler` 单测版本门控；curl 实跑 1.18+ 返回 `beach`、1.8-1.12 返回 `supported:false` |
| **更多结构（§10-3）** | 结构表新增**埋藏的宝藏**（`1/0/0`，1% 频率）；`Descriptor` 新增 `dimension`（下界：要塞 / 堡垒 / 化石；末地：末地城）与 `probability`（前哨站 20% / 宝藏 1% / 矿井 0.4%）；`/mc/structures/types` 返回描述；`seedStructureMap` 扩充至沙漠神殿 / 丛林神庙 / 雪屋 / 女巫小屋 / 海底废墟 / 沉船 / 古迹废墟 / 试炼密室 | ✅ `DescribeAll` 自洽测试；curl 实跑 types 与 `/seed/analyze`（版本过滤正确，`1.16-1.17` 无古迹废墟 / 试炼密室） |
| **理论候选 vs 实际生成（§10-5）** | 候选点区分「理论候选 / 群系可行 / 邻近偏移 / 群系不符」并在前端展示，另展示维度与频率门槛；`spacing=1` 的埋藏的宝藏**不纳入地图聚合**（避免逐区块候选噪声） | ✅ curl 实跑候选点带 `biome`/`feasible`；`/seed/analyze` 半径 6 仅 23 点（移除 169 个噪声点） |
| **超时、并发与独立限流（§10-6）** | `Structures` / `SlimeChunks` 单次 3s 上下文超时（超时返回部分结果并标注 `timed_out`）；新增 `MC_HEAVY_RPS` / `MC_HEAVY_BURST`（默认 3 / 6）独立限流与 `MC_HEAVY_CONCURRENCY`（默认 2）并发上限（`middleware.ConcurrencyLimit`，超出 429） | ✅ 配置默认值与 `.env.example` 同步；`middleware` 单测覆盖上限与释放；curl 6 并发实跑 2×200 + 4×429 |
| **路线与传送门提示（§10-4）** | `utils/coordinate.ts` 新增 `portalPair`（主世界 ↔ 下界 1:8）与 `routeHint`（水平距离 / 八方位 / 经下界理论距离）；`/coordinate` 新增传送门配对卡片与线路规划区 | ✅ `coordinate.spec` 新增 6 例（配对双向、正东 / 正北 / 西南方位） |
| **算法回归基线（§10-7）** | `regression_test.go`：固定种子 → 期望结构坐标与史莱姆区块；表内显式标注「由本实现产出，尚未与官方游戏逐格比对，只能防回归、不作为一致证据」 | ✅ 基线自洽，`go test` 通过 |
| **文档与规范** | `Function.md` 新增 2.13、MC 能力表 / 限流 / 超时 / API 一览 / 测试矩阵 / 状态表更新；`openapi.yaml` 新增 `/mc/capabilities`、`/mc/biome` 与 `TooManyRequests` 响应、补 429 与限时说明；`todo.md` §10 勾选 | ✅ YAML 校验通过；人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 干净、`go test ./...` 全包 ok（`mcstructure` 新增回归 / 版本测试、`handler` 新增文案与门控测试、`middleware` 新增并发上限测试）；前端 `vue-tsc -b --noEmit` 通过、`vitest` **28 文件 / 292 项**全绿、`vite build` 通过。运行中的 API 已用 `curl` 实测 `/mc/capabilities`、`/mc/structures`、`/mc/structures/types`、`/mc/biome`、`/mc/slime-chunks`、`/seed/analyze`，并以 6 并发请求验证结构接口的并发上限（2×200 + 4×429）。
> 已核实的取舍：结构分布算法自 `1.13` 起为 `random_spread`，`1.12` 及更早算法不同，故该区间标为不支持；群系查询仅 `1.18+`（服务端为 1.18+ 多噪声引擎）；频率门槛（前哨站 20% / 宝藏 1% / 矿井 0.4%）仅作提示，未在候选中模拟；**下界废弃传送门**独立参数未在 wiki 默认表中单列，未加入以免使用未核实数值。
> 未完成：`todo.md` §10 全部 8 项已勾选；§11 存档分析、§12 收藏工作台、§13 统计看板、§14 分享易用性仍待办。

---

## 4.16 本次已完成（2026-10-17，P1 §11 存档分析与诊断）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **支持范围与格式（§11-1）** | 明确支持 Minecraft **Java 版**：`level.dat`（gzip 的 NBT，兼容未压缩）与 Anvil `r.X.Z.mca`（头部 4096 位置表 + 4096 时间戳表，位置项 3 字节偏移 + 1 字节扇区数）；不支持基岩版；写入 `Function.md` 2.14 | ✅ 文档 + 端到端实跑 |
| **`level.dat` 基础信息（§11-2）** | 新增 `POST /mc/level/parse`：世界名称 / DataVersion / 版本 / 最近游玩 / 出生点 / 游戏时间 / 游戏模式 / 难度 / 极限与作弊 / 生成器 / 游戏规则；int64 种子以**字符串**返回避免 JS 丢精度；`nbt.ParseFile` + `DecompressGzip`（`MaxDecompressedSize` 16MiB） | ✅ `handler/level_test.go`（HTTP 端到端 + 直接 `buildWorldInfo`）；curl 实跑种子 `9007199254740993` 原样返回 |
| **区域与区块统计（§11-3）** | `region.Diagnose` 返回 `stats`：文件大小、区块数 / 空位置项、声明扇区数与容量、末尾残余字节、压缩方式分布、最早 / 最近时间戳；`/mc/region/inspect` 一并返回 | ✅ `region/diagnose_test.go`；curl 实跑 `stats` 字段齐全 |
| **区块坐标与顶层信息（§11-4）** | 区块清单（全局/本地坐标、时间戳、压缩、数据版本、生成状态）与单区块顶层字段摘要（白名单标量），沿用 `/mc/region/chunk` | ✅ 既有测试 + curl 实跑 |
| **异常区分与只读报告（§11-5/6）** | `region.Diagnose` 定义**确定错误** 10 类与**警告** 5 类（各带代码、文案、本地区块坐标），`/mc/region/inspect` 返回 `diagnosis`；前端分区展示并注明「只读、不代表已修复、不提供自动修复」 | ✅ `diagnose_test.go` 覆盖越界 / 头部重叠 / 长度非法 / 未知压缩 / 扇区重叠 / 未来与零时间戳 / 未对齐 / 扇区超预算；curl 实跑 `errors:[] warnings:[]` |
| **资源与安全限制（§11-7/8）** | 上传上限 64MiB；区域单区块解压 64MiB、`level.dat` gzip 解压 16MiB；NBT 递归深度 64；`/mc/region/inspect` 逐区块解析限时 5s（超时置 `timed_out`）；上传类接口叠加 `MC_HEAVY_*` 限流与 `MC_HEAVY_CONCURRENCY` 并发上限；默认不落盘，`region_token` 缓存有容量 / TTL / LRU 上限 | ✅ curl 6 并发实跑 2×429 之外全部 200；配置与 `.env.example` 同步 |
| **安全测试（§11-9）** | 新增畸形区域文件、超大解压输出、非法 NBT / 非法 `level.dat` 与两条端到端 multipart 测试 | ✅ `go test ./...` 全绿 |
| **文档与规范** | `Function.md` 新增 2.14，更新内置工具表 / API 一览 / 安全与超时 / 测试矩阵 / 状态表；`openapi.yaml` 新增 `/mc/level/parse` 并补 `/mc/region/inspect` 描述与 429；`todo.md` §11 全勾选；内置工具显示名由「区块文件检查」更新为「存档诊断」（slug / 路径不变） | ✅ YAML 校验通过；人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 干净、`go test ./...` 全包 ok（`region` 新增诊断测试、`nbt` 新增 gzip/ParseFile 测试、`handler` 新增 level/region 端到端测试）；前端 `vue-tsc -b --noEmit` 通过、`vitest` **28 文件 / 303 项**全绿、`vite build` 通过。运行中的 API 已用 `curl` 实测 `/mc/level/parse`（世界信息 + 种子精度）与 `/mc/region/inspect`（`stats` + `diagnosis` + `region_token`），并以 6 并发请求验证上传类接口并发上限。
> 已核实的取舍：诊断只读、不修改文件、**不提供自动修复**；仅检查区域文件**头部结构与区块数据头**（位置表 / 时间戳 / 长度 + 压缩类型），不解压校验压缩流完整性、不判断存档语义；空集合统一返回 `[]`（避免前端 `null` 处理）；`level.dat` 缺字段以**告警**提示而非报错。
> 未完成：`todo.md` §11 全部 10 项已勾选；§12 收藏工作台、§13 统计看板、§14 分享易用性仍待办（推荐顺序第 9 项起）。

---

## 4.17 本次已完成（2026-10-17，P1 §12 收藏与个人工作台）

| 事项 | 交付内容 | 验证状态 |
|---|---|---|
| **可选账号体系（§12-1）** | 在「公开工具免登录可用」前提下引入普通用户账号：注册 / 登录 / 退出 / 当前用户；用户名 3-40 位字母开头，密码 ≥ 8 位，Argon2id 哈希、不落明文；会话复用 `auth` 原语（`RandomToken` / `MemorySessionStore` / `LoginThrottle`） | ✅ `internal/service/user` 单测 + `handler` 端到端 |
| **收藏（§12-2）** | 支持 `tool` / `item` / `resource` / `wiki` 四类；`POST /user/favorites/{list,toggle,remove}`；按 `(user, kind, object)` 去重，上限 500 条；前端 `FavoriteButton` 登录走服务端、未登录「工具 / 资源」走本地、物品 / 百科提示登录；详情页（工具 / 资源 / 物品 / 百科）均已接入 | ✅ 单测（toggle / 用户隔离）+ curl |
| **最近使用同步（§12-3）** | `POST /user/recents/{list,record,clear}`；内置工具打开时登录用户自动上报；按对象去重并前置，服务端保留最近 50 条（`PruneRecents`） | ✅ 单测（裁剪 / 去重 / 清空）+ 前端 `user-account.spec.ts` |
| **常用种子 / 查询记录（§12-4）** | `POST /user/seeds/{list,save,delete}`；名称 + 种子 + 备注，种子以字符串保存避免大整数丢精度，上限 200 条，增删改均限本人 | ✅ 单测（CRUD + 大种子原样保存） |
| **取消收藏 / 清理 / 删号（§12-5）** | 取消收藏（幂等）、一键清空最近使用、逐条删除种子；`POST /user/delete`（CSRF + 密码二次确认）级联清除账号及全部个人数据 | ✅ 单测（删号级联 + 密码错不删除）+ curl |
| **保留期限与隐私（§12-6）** | 最近使用「最近 50 条」，收藏 / 种子保留至用户删除，删号即时清除；不采集手机号 / 邮箱 / 设备标识；`PrivacyView.vue` 改写（明确可选注册 + 保留期限 + HttpOnly 会话 + CSRF） | ✅ 文档 + 页面 |
| **安全评估（§12-7）** | 用户会话独立 Cookie（`USER_SESSION_COOKIE_NAME`，默认 `mc_user_session`）与独立 context key；`RequireUserAuth` 仅接受 `role=user`（管理员会话访问用户接口 → 401）；写操作 `UserCSRF`（缺失 / 不符 → 403）；登录按 `(用户名, IP)` 限流 5 次 / 15 分钟；请求体上限 8 KiB；用户与管理员表体系完全分离 | ✅ `handler/user_test.go`（401 / 403 / 管理会话 401）+ curl 实测 |
| **文档与规范** | `Function.md` 新增 2.15 并更新认证安全 / API 一览（新增 6.3 普通用户 API）/ 数据表 / 测试矩阵 / 状态表；`openapi.yaml` 新增 `user` tag、15 条 `/user/*` 路径与 `UserAccount` schema；`todo.md` §12 全勾选；`PROJECT.md` §0.1 用户体系表述更新；`.env.example` 新增 `USER_SESSION_COOKIE_NAME` | ✅ YAML 校验通过；人工核对 |

> 全量回归（本轮）：后端 `go vet ./...` 干净、`go test ./...` 全包 ok（新增 `internal/service/user` 与服务端鉴权 / CSRF 端到端测试）；前端 `vue-tsc -b --noEmit` 通过、`vitest` **29 文件 / 317 项**全绿、`vite build` 通过；`openapi.yaml` YAML 校验通过。
> 已核实的取舍：普通用户为**可选**能力，公开工具仍免登录；用户会话与管理员会话**完全隔离**（Cookie / context key / 角色校验三方分离）；收藏与最近使用按对象去重，服务端容量上限（收藏 500 / 最近 50 / 种子 200）；种子沿用字符串传输避免 int64 丢精度。
> 未完成：`todo.md` §13 使用统计与运营看板、§14 分享与易用性仍待办（推荐顺序第 9 项中「使用统计」部分、第 10 项）。

## 4.18 后续批次（2026-10-17 ~ 2026-10-19）

> 承接 §4.17，之后依次完成 §13 使用统计与运营看板、§14 分享与易用性、§9-B 村民交易、§9-C 生物资料、§6-B 链接健康、§21 开发栏目验收、§17 用户投稿与纠错、§15 红石资料库、§16 Mod 兼容性，以及 §18–§20 横向工程与验收。
> **逐项交付与验收证据见 §4.0.5 与 §4.0.1–§4.0.4**；功能状态见 `Function.md §九`；待办勾选见 `todo.md`。

| 批次 | 完成日期 | 状态 |
|---|---|---|
| §13 使用统计与运营看板 | 2026-10-17 | ✅ 已验收（§4.0.1） |
| §14 分享与易用性 / 可访问性 | 2026-10-17 | ✅ 已验收（§4.0.2） |
| §9-A 药水与酿造 | 2026-10-17 | ✅ 已验收（§4.0.3） |
| §9-B 村民交易 | 2026-10-18 | ✅ 已验收（§4.0.5） |
| §9-C 生物资料 | 2026-10-18 | ✅ 已验收（§4.0.5） |
| §6-B 链接健康 | 2026-10-18 | ✅ 已验收（§4.0.5） |
| §21 开发栏目（验收补做） | 2026-10-19 | ✅ 已验收（§4.0.4 更新） |
| §17 用户投稿与纠错 | 2026-10-18 | ✅ 已验收（§4.0.5） |
| §15 红石与机器设计 | 2026-10-18 | ✅ 已验收（§4.0.5） |
| §16 Mod 与整合包 | 2026-10-18 | ✅ 已验收（§4.0.5） |
| §18 API / 测试 / 文档 | 2026-10-19 | ✅ 已验收（§4.0.5） |
| §19 安全与可靠性 | 2026-10-19 | ✅ 已验收（§4.0.5） |
| §20 交付验收 | 2026-10-19 | 🟡 除「移动端真机走查」外全部完成 |
| 开发文档接入全站搜索（补做） | 2026-10-19 | ✅ 已完成（迁移 `0020` + `devdoc` 类型 + 前端类型筛选；§4.0.5） |
| §23 后台账号与权限（角色底座 / 角色管理页 / 普通用户管理） | 2026-10-19 | ✅ 已验收（§4.0.6） |
| §22 MC 源码浏览器切片 1（版本选择 / jar 缓存 / 类索引与检索） | 2026-10-19 | ✅ 已验收（§4.0.6） |
| §22 MC 源码浏览器切片 2（类结构视图 / 继承关系，纯 Go 无 JVM） | 2026-10-19 | ✅ 已验收（§4.0.8；切片 3 待办） |

**仅剩待办**：移动端真机走查（`todo.md` §7 第 132 行、§20 第 391 行）；MC 源码浏览器切片 3（反编译源码浏览需部署侧提供 Java 运行时；引用查找 / 重命名映射 / 字节码指令 / Javadoc）；根目录遗留文件清理（`1.py` / `go.mod.root.bak`）；CI 流水线（可选增强）。

---

# 第五部分 · 历史待办归档

> 原文件：`BACKEND_TODO.md` ｜ **状态：已过时**，所列 P0–P2 事项**实测均已修复**。
> 保留此节仅为追溯，**请勿据此重复开发**；新待办以第四部分为准。

## 5.1 原结论速览（含最新状态）

| 原级别 | 事项 | 影响面 | 最新状态 |
|---|---|---|---|
| P0 | 64 位种子以 JSON number 返回，JS 静默丢精度 | `/mc/*` 种子字段 | ✅ 已修复 |
| P0 | 生物群系采样（biomes）未实现 | 种子地图无法渲染群系 | ✅ 已修复 |
| P1 | 5 个内置工具缺 `tools` 表记录 | 搜索/工具库/sitemap 找不到 | ✅ 已修复 |
| P1 | `/mc/structures/types` 返回顺序不确定 | 前端下拉顺序每次变化 | ✅ 已修复 |
| P1 | `/seed/analyze` 的 `version` 参数被忽略 | 版本选择器无效 | ✅ 已修复 |
| P2 | `/mc/region/chunk` 每次重传整个文件 | 大文件反复上传 | ✅ 已修复（region_token 缓存） |
| P2 | `/mc/seeds/find` 与全局限流不匹配 | 并发占用 worker | ✅ 已修复（单独限流） |
| P2 | 网关请求体上限（部署阻断） | nginx 默认 1MB → 413 | ✅ 已修复（`client_max_body_size 64m`） |

## 5.2 已核对为"已完成"的证据

| 旧编号 | 事项 | 现状证据 |
|---|---|---|
| P0-1 | 64 位种子精度丢失 | `handler/mc.go` 全部改用 `mcstructure.FormatSeed`（字符串）；前端 `SeedValue = number \| string` 兼容 |
| P0-2 | 生物群系采样未实现 | `internal/mcbiome/`（cubiomes 移植）+ `/seed/analyze` 支持 `include_biomes` 返回 `biomes` |
| P1-3 | 内置工具缺 `tools` 表记录 | `backend/seeds/tools_builtin.json` 已存在；`cmd/seed` 支持 `--type tools` |
| P1-4 | `/mc/structures/types` 顺序不定 | `mcstructure.ListKnown()` 已 `sort.Slice` |
| P1-5 | `/seed/analyze` 忽略 `version` | `seed.go` 使用 `NormalizeVersion` / `VersionFromString` |
| P2-6 | `/mc/region/chunk` 重传整文件 | `handler/regioncache.go` + `REGION_CACHE_*` 环境变量 |
| P2-7 | `/mc/seeds/find` 限流不匹配 | `MC_FIND_RPS` / `MC_FIND_BURST` + `middleware.RateLimit` 单独挂载 |
| P2-8 | 网关请求体上限 413 | `frontend/nginx.conf` 已设 `client_max_body_size 64m` + 流式转发 + 120s 超时 |

**其他已确认到位**：安全响应头含 `CSP` / `X-Frame-Options` / `nosniff` / `Referrer-Policy`；Argon2id + Cookie 会话 + CSRF + 角色中间件 + 审计日志；SSRF 链接检查（含重定向逐跳校验）；前端全部需求路由与页面（含 5 个内置工具页）已接线；`localStorage` schema 版本化与异常安全。

## 5.3 原文件附注（前端侧仍需，非后端）

- `frontend/src/components/AdminTable.vue` 不存在（需求 §4.3 清单项）→ 已并入第四部分 P2-6。
- `PROGRESS.md` 严重滞后 → 已由本次整合更新（第三部分）。

---

> 本文件为项目总览与需求规格的事实来源；**功能与验收状态以 `Function.md` 为准，实时待办与勾选以 `todo.md` 为准**。后续新增待办请直接更新 `todo.md`，不再新建独立 TODO 文件。
