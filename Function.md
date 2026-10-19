# MC 工具站 · 功能规格说明（Function.md）

> 本文档描述系统**当前已具备**的功能，作为功能清单与验收基线。
> 技术栈：Go 1.27 + Chi + GORM + PostgreSQL 16 · Vue 3 + TypeScript + Vite。
> 更新时间：2026-10-16。

---

## 一、系统概览

MC 工具站是面向 **Minecraft Java 版**玩家、整合包玩家、服务器管理者与 Mod 开发者的**工具与资源导航站**，由两大部分组成：

| 部分 | 说明 | 访问方式 |
|---|---|---|
| **公开前台** | 无需登录，工具查询、资源导航、百科、反馈、SEO | 浏览器 / 公开 REST API |
| **管理后台** | 登录后维护内容、处理反馈、查看审计、管理账号 | `/admin`，会话 Cookie 鉴权 |

核心设计约定：

- 所有**查询 / 搜索 / 列表**接口统一使用 `POST`，参数置于 JSON 请求体（`robots.txt` / `sitemap.xml` 例外为 `GET`）。
- 统一响应结构：成功 `{"data": ..., "meta": ...}`，错误 `{"error": {code, message, details}, "request_id": ...}`。
- 分页参数：`page` 从 1 开始，`page_size` 默认 20、最大 50（管理端部分列表最大 100）。

---

## 二、公开前台功能

### 2.1 首页与全站搜索

| 功能 | 说明 |
|---|---|
| 首页 | 全站搜索框、内置工具快捷入口、热门工具、资源分类导航、最近更新（资源/工具） |
| 全站搜索 | 跨 **工具 / 资源 / 百科 / 物品 / 开发文档** 统一检索，支持类型过滤、分页；英文标识走 `tsvector` 加权（GIN 索引），中文子串走 `ILIKE` 兜底 |
| 相关度权重 | 标题 > 命名空间 ID > 别名 > slug > 摘要（精确 / 前缀 / 包含三档梯度）；物品把英文名并入别名参与匹配 |
| 排序与筛选 | 排序可选 **相关度**（默认）或 **最近更新**；可按类型与游戏版本筛选，状态同步到 URL（可分享 / 可回退） |
| 无结果引导 | 无结果时返回 `meta.suggestions`（依据已记录搜索词的字面关系 + 热门词兜底，**不做语义猜测**），前端以可点击关键词呈现 |
| 输入防护 | 关键词上限 80 字符（超长 422）、查询 3 秒超时、异常请求体 400 |
| 最近使用 | 本地记录最近访问的工具（`recent-tools` store） |

### 2.2 工具中心

- 展示数据库中的工具（按分类、特性、状态筛选），支持搜索与分页。
- 工具详情页展示名称、摘要、说明、标签、支持版本、输入/输出 Schema、外链。
- **内置工具重定向**：为避免「同一功能两套界面」，内置工具 slug 访问工具详情时会自动重定向到其专属落地页（见 2.3）。

### 2.3 内置工具（前端自带页面，单一事实源）

清单见 `frontend/src/utils/builtin-tools.ts`，同时作为首页入口、工具中心「内置工具」区与页脚链接的唯一来源。页脚「常用工具」只列清单**前 4 项**（避免 19 项把页脚撑长），并在下方给出「查看全部工具 →」入口指向 `/tools`。

| 工具 | 路由 | 分类 | 功能 |
|---|---|---|---|
| 合成配方 | `/recipes` | 查询类 | 按物品/版本/配方类型查询有序/无序合成、熔炼、切石、锻造等配方，展示材料网格 |
| 坐标换算 | `/coordinate` | 计算类 | 主世界 ↔ 下界坐标按 8:1 互转，给出传送门对应坐标与误差范围 |
| 物品 ID | `/items` | 查询类 | 按中文名/英文名/别名查物品标准命名空间 ID；行可进入独立详情页 `/items/:item_id` |
| 药水与酿造 | `/potions` | 查询类 | 按名称/类别/游戏版本查药水，展示效果与时长（普通/延长/增强）、酿造步骤链与通用酿造规则；行可进入独立详情页 `/potions/:slug` |
| 村民交易 | `/villagers` | 查询类 | 按职业/类别/游戏版本查村民职业，展示工作站、等级规则与分组交易表（村民想要 → 玩家获得、库存、经验、价格浮动）；行可进入独立详情页 `/villagers/:slug` |
| 生物资料 | `/mobs` | 查询类 | 按名称/类别/游戏版本查生物，展示生命值、掉落（数量区间与概率）、生成条件与驯服 / 繁殖方式；行可进入独立详情页 `/mobs/:slug` |
| 红石资料 | `/redstone` | 查询类 | 元件 / 设计双 tab：按类别/用途/难度/游戏版本查红石元件与机器设计，元件给出工作机制与信号说明，设计给出难度、材料成本（区间）、作者与授权；行可进入独立详情页 `/redstone/components/:slug`、`/redstone/designs/:slug` |
| Mod 兼容性 | `/mods` | 查询类 | Mod / 整合包双 tab：按加载器/类别/游戏版本查 Mod 与整合包，Mod 给出发布版本、已记录的前置依赖与冲突（逐条标注来源与验证状态）与反向依赖，整合包给出包内 Mod 清单；行可进入独立详情页 `/mods/:slug`、`/modpacks/:slug` |
| 指令辅助 | `/commands` | 辅助类 | 命令模板、参数说明与示例查询 |
| 指令生成器 | `/command-generator` | 生成类 | 表单化生成 `/give` `/summon` `/effect` `/setblock` `/tp` `/gamemode`，按版本区分旧式 NBT 与新版数据组件 |
| 附魔计算器 | `/enchant-calculator` | 计算类 | 校验附魔组合合法性，按官方公式估算铁砧合并经验花费与先作惩罚，方案对比 |
| 版本信息 | `/versions` | 查询类 | 各版本下载地址、启动命令、Java 要求、运行库（来自 Mojang 元数据，磁盘缓存） |
| ID 对照表 | `/block-ids` | 查询类 | 方块/物品的中文名 ↔ 命名空间 ID 对照，支持按分类过滤（源自 `data/物品id.md`） |
| 种子地图 | `/seed-map` | 查询类 | 输入种子分析世界：史莱姆区块、结构候选坐标（含沙漠神殿 / 丛林神庙 / 雪屋 / 女巫小屋 / 海底废墟 / 沉船 / 古迹废墟 / 试炼密室）、可选生物群系底色渲染 |
| 种子与结构工具 | `/seed-tools` | 计算类 | 结构候选坐标（含群系可行性与版本提示）、生物群系查询、要塞估算、随机种子生成、按目标坐标搜索种子、版本支持矩阵 |
| 存档诊断 | `/region-inspect` | 辅助类 | 解析 `level.dat` 世界信息与 `.mca` 区域文件结构诊断（仅内存、不落盘，只读、不含自动修复） |
| 开发文档 | `/dev` | 查询类 | NeoForge / Forge / Fabric / 原版数据包开发知识库：按名称、平台、主题与版本检索，详情含要点正文、关键接口速查、官方文档与可核验来源；行可进入独立详情页 `/dev/:slug` |
| 版本映射查询 | `/mappings` | 查询类 | 查询类 / 方法 / 字段在混淆名（obf）/ 中间名（mid）/ 可读名（named）三列命名空间下的对照；数据集由 `cmd/import-mappings` 按官方来源导入，未导入时展示导入指引 |

### 2.4 统一物品资料库（已交付）

物品 ID 查询页的每一行可进入**独立详情页** `/items/:item_id`（`item_id` 含冒号，路径中为合法字符）。

| 能力 | 说明 |
|---|---|
| 资料类型 | `item_category`：方块 / 物品 / 工具 / 武器 / 防具 / 食物 / 材料 / 其他；**决定可选属性（如耐久度）是否展示**，避免对不适用类别展示空值 |
| 可选属性 | 堆叠上限、**耐久度**（仅工具 / 武器 / 防具）、获取方式、用途、图片、别名 |
| 来源与可信度 | 记录来源 URL、数据版本与**最后校验时间**；未校验时明确显示「未经校验」 |
| 获取指令 | 由 `item_id` 生成 `/give @s <item_id> 1` 模板，一键复制，不依赖未收录数据 |
| 关联配方 | **数据驱动**：`recipes.result_item_id`（产出）与 `recipes.ingredients` JSONB 包含匹配（作为材料）；无关联时明确提示「暂未收录」，不伪造 |
| 关联物品解析 | 关联配方引用到的物品会**批量解析为可跳转摘要**（`related_recipes.resolved_items`，键为 `item_id`）；只包含 `items` 表中**真实存在**的物品，未收录物品不进入映射，前端回退为纯文本 ID（无断链） |
| 跨模块检索 | 指令 / 百科 / 资源无可靠元数据关联，改为提供「按物品名检索」入口（百科 / 资源走 `/search?q=…&type=…`，指令走 `/commands?q=…`），不伪造关联、不直链未发布内容 |
| 检索 | 中文名 / 英文名 / 标准 ID / 别名统一检索（沿用既有 `search_vector` + `ILIKE` 兜底） |
| 适用版本 | `supported_versions` 支持**单版本**（`1.21.1`、`24w14a`、`1.21-pre1`）、**范围**（`1.16-1.17` 闭区间、`1.18+` 开上界）与**所有版本通配**（`all`，等价 `*`，必须独占）；写入前校验格式、重复与「起点大于终点」的不合理范围，并按升序归一化 |
| 数据导入 | `cmd/seed --type items` 扩展新字段，具备**校验**（类别枚举、数值非负、时间 RFC3339、版本格式与范围）、失败记录与汇总报告、幂等 upsert |

### 2.5 资源中心

- 资源列表：按资源类型（Mod / 整合包 / 资源包 / 光影 / 存档 / 数据包 / 插件）、加载器（Forge / NeoForge / Fabric / Quilt / 原版 / Paper / Spigot / Purpur）、游戏版本、来源平台、关键词筛选，支持排序与分页。
- 版本筛选选项**动态取自官方版本清单**（`POST /versions`，服务端带缓存，失败回退离线兜底清单），资源与搜索按主版本归并并附带「全部 MC 版本」，配方按完整正式版清单精确匹配。
- 资源详情：封面、作者、来源平台与链接、许可、**兼容性结论**、**发布版本**、依赖 / 前置 / 冲突、安装指引、标签、**外链检查状态与最后检查时间**。
- 资源链接检查：后台触发，记录可达性状态（正常 / 已重定向 / 失效 / 被拦截 / 出错）与检查时间。
- 链接健康与人工确认（TODO §6）：`POST /admin/link-health/run` 后台**受限并发**批量复查资源来源链接（`LINK_CHECK_MAX_CONCURRENCY`，1–16，默认 4；带超时与重定向上限，并对操作者限流），结果落库 `resource_link_checks` 并同步资源链接状态；`POST /admin/link-health/list` 复核列表（默认只看失败项）；`POST /admin/link-health/{id}/confirm` 人工确认「确认失效 / 确认可用 / 暂不处理」，记录审核人、时间与备注。已确认可用 / 已忽略的资源在后续自动复查中跳过。

### 2.6 资源兼容性与来源信息（本次新增）

**统一字段与校验**：资源类型、加载器、适用游戏版本、游戏版本类型（java / bedrock）、来源平台在**写入前统一校验与规范化**，导入器与后台接口共用同一套枚举与版本规则（`internal/service/resource/meta.go`），避免两条写入路径产生不一致数据。

- 加载器：仅允许 Forge / NeoForge / Fabric / Quilt / 原版 / Paper / Spigot / Purpur / 其他，去重后入库。
- 适用版本：复用 `internal/versioning`，支持**单版本**（`1.21.1` / `1.21-pre1` / `24w14a`）、**范围**（`1.16-1.17` 闭区间、`1.18+` 开上界）与**所有版本通配**（`all`，等价写法 `*`）；非法格式、重复、起点大于终点、通配与具体版本并列一律 422。
- 游戏版本类型：缺省 `java`，非法值拒绝。

**「所有版本」通配（本次新增）**：资料的适用版本声明为 `all` 时表示适用于**全部 MC 版本**，排序时排在最前，且**必须独占出现**（与具体版本并列属于冗余，直接拒绝）。该声明对资源级字段与发布版本均生效。

- 兼容性结论中返回 `all_versions=true`，详情页展示 **全部 MC 版本** 徽标，并可**按官方正式版清单展开完整版本列表**（清单来自 `POST /versions`，服务端带缓存）。
- **版本筛选语义**：声明通配的资源对**任何具体版本**筛选都命中；`game_version=all` 则只命中**声明了通配**的资源（区别于「不限版本」= 不传该参数）。

**版本选项覆盖全部 MC 版本（本次新增）**：资源中心 / 搜索结果 / 配方查询的版本筛选项不再硬编码，改为从 `POST /versions`（官方 Mojang 清单）**动态获取**；资源与搜索按主版本归并（如 `1.21.4` → `1.21`，与后端子串匹配口径一致）并附带「全部 MC 版本」，配方按完整正式版清单精确匹配。清单拉取失败时回退**离线兜底清单**，保证筛选始终可用。

**依赖 / 前置 Mod / 已知冲突**：`resources.dependencies` 规范为对象数组 `{name, type, resource_url?, required, note?}`，`type ∈ dependency | prerequisite | conflict`。历史数据缺 `type` 时按 `dependency` 处理；冲突项不携带 `required`，避免语义矛盾。详情页按「前置 Mod → 依赖 → 已知冲突」分组展示。

**发布版本关联**：新增 `resource_releases` 表，记录同一资源在不同游戏版本 / 加载器下的发布记录（版本标签、适用版本、加载器、发布页链接、发布时间、备注、是否最新）。同一资源内版本标签唯一，「最新」通过部分唯一索引保证至多一条；后台以**完整列表整体替换**（幂等，含清空）。

**兼容性结论（未验证原则）**：结论**不落库**，由服务端按资源级字段与发布版本实时推导：

| 场景 | 结论 |
|---|---|
| 资源级字段或发布版本提供了版本 / 加载器（含通配 `all`） | `verified=true`，返回合并去重后的适用版本与加载器，并标注依据来源（`resource` / `releases`）与 `all_versions` |
| 两者均无数据 | `verified=false`，返回明确提示「兼容性尚未验证」，界面展示 **未验证** 徽标 |

人工核验备注 `resources.compatibility_note` 可选，用于说明结论依据或例外，详情页原样展示。

**来源与许可展示**：详情页「来源与许可信息」区块集中展示作者、来源平台、许可信息、原始来源链接与**外链检查状态 / 最后检查时间**（未检查时明确显示「未检查」），仅客观呈现检查结果，不额外推断「安全」。

**元数据批量导入**：`cmd/seed --type resources`（`--type all` 已纳入），按 `slug` 幂等 upsert，发布版本随资源一并替换；校验复用服务层规范化函数。`releases` 字段缺省表示「不触碰已有发布版本」，显式空数组表示「清空」。

### 2.7 百科内容

- 百科详情页：Markdown 渲染（**禁用原始 HTML**，防 XSS），支持目录、标题、表格、代码等。
- 内容类型：指南 / 概念 / 教程 / 参考 / 其他；保留来源 URL 与许可信息。
- 支持从中文 Minecraft Wiki 抓取公开条目（见 §7.3）。

### 2.8 问题反馈（公开提交）

- 反馈类型：错误反馈（bug）/ 资源失效（resource_broken）/ 侵权通知（infringement）/ 其他。
- 提交字段：类型、标题、内容、相关 URL、联系方式。
- **服务端限流与反垃圾**：公开反馈接口带独立限流。

### 2.9 静态信息页与 SEO

| 页面/能力 | 路由/路径 | 说明 |
|---|---|---|
| 关于本站 | `/about` | 站点介绍与免责声明 |
| 隐私政策 | `/privacy` | 隐私说明 |
| 版权声明 | `/copyright` | 版权与内容授权说明 |
| 404 | `*` | 兜底页面 |
| robots.txt | `/robots.txt` | 搜索引擎协议（GET） |
| sitemap.xml | `/sitemap.xml` | 动态站点地图：静态页 + 数据库中的资源/百科/工具/**物品详情页/药水详情页/开发文档详情页**；查询失败降级为仅静态页 |

- 每页动态设置 `title`；管理页统一 `noindex`。

### 2.10 跨模块资料关联（上一批交付）

关联原则：**只对有可靠元数据的关系给出直链；无法确认时只给「按名称检索」入口，绝不伪造关联或链接未发布内容。**

| 关系 | 依据 | 呈现 |
|---|---|---|
| 配方 → 物品（结果 / 材料） | `recipes.result_item_id`、`recipes.ingredients`（JSONB 包含匹配） | `/recipes` 每项返回 `resolved_items`，结果物品与材料渲染为物品详情页链接 |
| 物品 → 配方 | 同上（产出 / 作为材料） | 物品详情页「关联配方」区块，并附材料链接 |
| 物品 / 百科 / 资源 → 指令 | 无可靠元数据 | 检索入口 `/commands?q=<名称>` |
| 百科 → 物品 / 工具 / 资源 | 无可靠元数据（仅 `tags`） | 检索入口 `/search?q=<标题>&type=…` |
| 资源 → 相关整合包 | `dependencies` 已规范为 `{name, type, resource_url?, required, note?}`，但**无反向索引**，无法可靠反查「哪些整合包依赖此资源」 | 检索入口 `/search?q=<名称>&type=resource`（同时展示版本 / 加载器 / 依赖分组） |

**有效性检查**：关联物品解析只返回 `items` 表中真实存在的记录；未收录或已删除的物品不会生成链接，前端回退为纯文本 ID。搜索类关联入口仅命中**已发布**内容（`status=published`）。

### 2.11 可视化指令生成器（本批新增）

内置工具 `/command-generator`：把「拼指令字符串」变成「填表单」，全部逻辑在浏览器本地完成（纯函数 `frontend/src/utils/command-generator.ts`），**不执行命令、不连接服务器**。

| 能力 | 说明 |
|---|---|
| 指令类型 | `/give`、`/summon`、`/effect`、`/setblock`、`/tp`、`/gamemode` 六类表单化配置 |
| 版本化语法 | 物品：`1.20.5` 起为**数据组件** `item[组件]`，更早为**旧式 NBT** `{…}`；`/effect`：`1.19` 起为 `give` 子命令；实体数据始终为 NBT |
| 版本裁决 | 快照 / 预览版 / 无法识别时按最新语法生成并显式标注「按最新语法」 |
| 校验 | 目标选择器 / 玩家名、命名空间 ID、数量范围、坐标（绝对 / `~` / `^`）、附魔等级上限、方块状态名值 |
| 输出 | 生成结果预览 + 一键复制 + 清空表单 + 填入示例 + 逐字段错误定位 |

### 2.12 附魔与装备计算器（本批新增）

内置工具 `/enchant-calculator`：附魔资料、组合校验与铁砧花费估算。规则依据 **Minecraft Wiki《Anvil mechanics》（Java Edition）**，仅实现有公开依据的部分。

| 能力 | 说明 |
|---|---|
| 附魔资料表 | 41 项附魔：等级上限、适用装备、**铁砧合并乘数**（物品 / 附魔书两列）与定性效果；随所选装备过滤 |
| 组合校验 | 适用性（该附魔能否用于该装备）、等级范围、重复项、**互斥冲突**（锋利 / 亡灵杀手 / 节肢杀手、时运 / 精准采集、四种保护、深海探索者 / 冰霜行者、无限 / 经验修补等） |
| 版本语义 | 互斥附魔自 `1.12.2` 起**可合并但牺牲方附魔丢失**（更早版本视为错误） |
| 铁砧花费 | Java 公式：目标先作惩罚 + 牺牲先作惩罚 + 重命名 1 + 修复 2 + 附魔花费（Σ 结果等级 × 乘数，互斥每项 +1）；`≥40` 判定「过于昂贵」 |
| 结果推演 | 更高取高、相同且未满级 +1、更低不变、缺失则获得（互斥则丢失）；先作惩罚 `2ⁿ−1` 对照表 |
| 方案对比 | 将当前「装备 + 附魔 + 花费」快照加入对比表，横向比较不同方案 |
| 未覆盖边界 | 耐久修复量、附魔台花费、铁砧损耗概率、Bedrock 版差异、`1.8` 之前机制与模组附魔——均在页面与代码显式标注，不假装支持 |

### 2.13 世界分析工具扩展（本批新增）

围绕结构、群系、路线与版本支持，把「理论候选坐标」升级为「带依据与限制的定位结果」，并补齐版本支持梳理与回归测试。

| 能力 | 说明 |
|---|---|
| 版本支持矩阵 | 新增 `POST /mc/capabilities`：逐能力（结构 / 群系 / 史莱姆 / 要塞 / 种子搜索）在 `1.8-1.12 / 1.13-1.15 / 1.16-1.17 / 1.18+` 的支持状态与说明；前端 `/seed-tools` 可展开查看 |
| 版本明确提示 | 结构候选接口新增 `available`（该结构是否在所选版本生成）、`placement_supported`（分布算法是否与实现一致，`1.12` 及更早为 `false`）与合并的 `supported`；不支持时给出明确文案（`note`），不把结果描述为确定结论 |
| 群系查询与定位 | 新增 `POST /mc/biome`：按种子与方块坐标返回地表群系（仅 `1.18+`，cubiomes `SAMPLE_NO_DEPTH` 语义）；其他版本返回 `supported=false` 与说明，而非报错或伪造 |
| 更多结构 | 结构表新增**埋藏的宝藏**（`spacing=1/separation=0/salt=0`，wiki 标注 1% 频率）；结构参数描述新增 `dimension`（主世界 / 下界 / 末地）与 `probability`（频率门槛：前哨站 20%、埋藏的宝藏 1%、废弃矿井 0.4%）；种子地图结构覆盖扩充至沙漠神殿 / 丛林神庙 / 雪屋 / 女巫小屋 / 海底废墟 / 沉船 / 古迹废墟 / 试炼密室 |
| 理论候选 vs 实际生成 | 候选点区分「理论候选位置 / 群系可行 / 邻近偏移 / 群系不符」，并展示频率门槛（`probability`）；`spacing=1` 的高频结构（埋藏的宝藏）不纳入地图聚合，避免逐区块候选造成噪声误读 |
| 路线与传送门提示 | `/coordinate` 新增**传送门理论配对**（主世界 ↔ 下界 1:8）与**线路规划**（两点水平直线距离、八方位、经下界理论距离）；明确标注理论对应、实际连接会就近寻找已有传送门 |
| 超时、并发与独立限流 | `/mc/structures`、`/mc/slime-chunks` 单次计算限时 `3s`（超时返回部分结果并标注 `timed_out`），并新增独立限流 `MC_HEAVY_RPS` / `MC_HEAVY_BURST`（默认 3 / 6）与**并发上限** `MC_HEAVY_CONCURRENCY`（默认 2，超出返回 429） |
| 算法回归基线 | 新增固定种子 → 期望坐标的**回归基线**（含史莱姆区块），用于锁定行为、防止坐标漂移；表内显式注明「尚未与官方游戏逐格比对，不能作为与游戏一致的证据」 |

---

### 2.14 存档分析与诊断（本批新增）

工具「存档诊断」（`/region-inspect`）把原来的「区块文件检查」扩展为存档分析与只读诊断：上传 `level.dat` 输出世界信息，上传 `r.X.Z.mca` 输出区块清单、统计与结构诊断报告。**只读**：不修改存档、不落盘、不提供自动修复。

| 能力 | 说明 |
|---|---|
| 支持范围与格式 | 支持 Minecraft **Java 版**存档：`level.dat`（gzip 压缩的 NBT，也兼容未压缩）与 Anvil 区域文件 `r.X.Z.mca`（头部 8192 字节：4096 位置表 + 4096 时间戳表；位置项 3 字节偏移 + 1 字节扇区数）；不支持基岩版格式 |
| `level.dat` 基础信息 | 新增 `POST /mc/level/parse`：世界名称、DataVersion、版本名、最近游玩时间、种子（int64 以**字符串**返回，避免 JS 丢精度）、出生点、游戏时间、游戏模式、难度、极限/作弊标记、生成器、游戏规则 |
| 区域与区块统计 | `/mc/region/inspect` 响应新增 `stats`：文件大小、区块数 / 空位置项、声明扇区数与容量、末尾残余字节、压缩方式分布、最早 / 最近时间戳 |
| 区块坐标与顶层 NBT | 沿用区块清单（全局/本地坐标、时间戳、压缩方式、数据版本、生成状态）与单区块顶层字段摘要（`POST /mc/region/chunk`），仅展示白名单标量字段，避免响应过大 |
| 异常区分（错误 vs 警告） | 新增 `region.Diagnose`：**确定错误**（`file_too_small`、`offset_in_header`、`offset_beyond_file`、`chunk_header_truncated`、`length_invalid`、`length_beyond_file`、`sectors_insufficient`、`sectors_beyond_file`、`unknown_compression`、`sector_budget_exceeded`）与**警告**（`entry_inconsistent`、`future_timestamp`、`zero_timestamp`、`sector_overlap`、`trailing_bytes`）；每条含代码、文案与（区块级问题的）本地区块坐标 |
| 只读诊断报告 | `/mc/region/inspect` 与 `/mc/level/parse` 均返回结构化报告（`diagnosis` / `warnings`），前端分区展示；文案明确「只读检查、不代表已修复、不提供自动修复」 |
| 资源与安全限制 | 上传上限 64MiB；区域单区块解压输出上限 64MiB、`level.dat` gzip 解压输出上限 16MiB（`nbt.MaxDecompressedSize`，防压缩炸弹）；NBT 递归深度上限 64（`nbt.maxDepth`）；`/mc/region/inspect` 逐区块解析总限时 5s（超时置 `timed_out`，结构诊断不受影响）；上传解析类接口叠加独立限流（`MC_HEAVY_RPS`/`MC_HEAVY_BURST`）与并发上限（`MC_HEAVY_CONCURRENCY`，超出 429） |
| 不持久化 / 有界缓存 | 默认不落盘；`/mc/region/inspect` 返回的 `region_token` 仅在内存缓存原始字节，供 `/mc/region/chunk` 免重传，容量上限 `REGION_CACHE_MAX_MB`（默认 256）、有效期 `REGION_CACHE_TTL_SECONDS`（默认 600），超限按 LRU 淘汰、过期惰性回收 |
| 安全测试 | 新增畸形区域文件（偏移越界、落在头部、长度越界、未知压缩、扇区重叠、未来/零时间戳、未对齐）、超大解压输出、非法 NBT / 非法 `level.dat` 的测试；`level.dat` 与区域检查各覆盖一条端到端 multipart 请求 |

> 说明：诊断基于区域文件**头部结构**（位置表 / 时间戳）与区块数据头（长度 + 压缩类型），用于发现明显的结构损坏；不解压校验压缩流完整性，也不判断存档语义正确性。

---

### 2.15 收藏与个人工作台（本批新增）

在保留「公开工具免登录可用」的前提下，引入**可选的普通用户账号**：登录后收藏、最近使用与常用种子改为服务端按账号存储，跨设备同步；未登录时收藏与最近使用仍沿用浏览器本地存储，功能不降级。

| 能力 | 说明 |
|---|---|
| 可选账号体系 | 注册/登录（`/login`）、个人工作台（`/workspace`，`requiresUser` 路由守卫）；用户名 3-40 位、字母开头，密码至少 8 位（管理员为 12），Argon2id 加盐哈希、不落明文 |
| 收藏（登录用户） | 支持工具 / 物品 / 资源 / 百科四类（`kind` = `tool`/`item`/`resource`/`wiki`）；收藏按钮登录时走服务端、未登录时「工具 / 资源」走本地、物品 / 百科提示先登录；收藏对象按 `(user, kind, object)` 去重，上限 500 条 |
| 最近使用同步 | 内置工具被打开时，登录用户自动上报「最近使用」（`recent-tools.record` 同步服务端）；按 `(user, kind, object)` 去重并前置，服务端保留**最近 50 条**，可一键清空 |
| 常用种子 / 查询记录 | 工作台内维护「名称 + 种子 + 备注」，种子以**字符串**存储避免大整数丢精度；新建/更新/删除均限本人，上限 200 条 |
| 取消收藏与清理 | 工作台与详情页均可取消收藏；最近使用可一键清空；收藏与种子逐条删除 |
| 账号数据删除 | `/user/delete`（需 CSRF + 密码二次确认）级联删除账号及其收藏 / 最近使用 / 常用种子，立即永久清除、不可恢复 |
| 保留期限与隐私 | 最近使用保留最近 50 条；收藏与种子保留至用户删除；不采集手机号 / 邮箱 / 设备标识等不必要信息；隐私政策已更新（见 `/privacy`） |
| 认证与隔离 | **统一账号**（`accounts`，迁移 0022）：管理员与普通用户同表、共用同一个会话 Cookie（`SESSION_COOKIE_NAME`）与会话绑定 CSRF；用户接口用 `RequireAuth + RequireUserRole` 仅接受 `role=user` 会话（管理员会话访问 → 401），后台接口按权限点判定（`role=user` 不含任何权限点 → 403） |
| 限流 | 登录失败按 `(用户名, IP)` 限流 5 次 / 15 分钟；注册与登录接口受全局与独立限流；请求体上限 8 KiB |

> 说明：普通用户能力为**可选**，所有公开工具、搜索与资料页在未登录时均可正常使用；用户接口全部为 `POST`，读接口仅需登录、写接口叠加 CSRF 校验。

### 2.16 使用统计上报（本批新增）

为运营看板（见 §4.9）提供数据来源：内置工具页在前端**匿名、尽力而为**地上报使用事件。

| 能力 | 说明 |
|---|---|
| 上报接口 | `POST /stats/event`，请求体 `{scope, id, action}`；成功返回 `204`，非法入参或未知对象返回 `422` |
| 事件口径 | `visit` = 进入工具页（路由 `afterEach`）；`use` = 执行核心动作成功，或**复制工具产出**（`CopyButton` 带 `tool` 属性）；`fail` = 核心动作失败（请求失败 / 解析异常） |
| 覆盖范围 | 全部 **18 个内置工具**：`recipes` / `coordinate` / `items` / `potions` / `villagers` / `mobs` / `redstone` / `mods` / `commands` / `command-generator` / `enchant-calculator` / `versions` / `block-ids` / `seed-map` / `seed-tools` / `dev-docs` / `mappings` / `region-inspect` |
| 对象校验 | 后端按白名单校验：内置工具清单 ∪ `tools` 表（`scope=tool`）/ `resources` 表（`scope=resource`），拒绝伪造 slug（返回 `422 STATS_TARGET_NOT_FOUND`），避免脏数据污染统计 |
| 资源访问 | 资源详情访问量由后端在 `POST /resources/{slug}` 成功响应时直接累加，前端无需上报，因此不存在漏报 |
| 可靠性 | 上报失败静默（不重试、不阻塞、不提示用户）；测试环境（`MODE=test`）不发送；上报使用独立 axios 实例，不携带管理端 / 用户会话与 CSRF，也不触发 401 跳转 |
| 隐私 | 只上报「公开 slug + 动作」，**不发送**IP、会话、用户标识、页面参数、输入内容或搜索结果 |

> 口径说明（避免误读）：`use` 是**可观测的使用信号**而非精确调用次数。计算 / 生成 / 查询类工具以「复制产出」为信号（用户可能只查看不复制，故属保守下界）；`seed-tools`、`seed-map`、`region-inspect`、`recipes` 另有显式执行动作，动作成功即计入 `use`；`seed-map` 后端结构数据获取失败（降级为本地史莱姆区块）计入 `fail`，以便运营看板暴露该类接口故障。

### 2.17 分享与稳定链接（本批新增）

目标：**查询结果可复现、可分享**，且分享链接**不含任何会话秘密**。

| 能力 | 说明 |
|---|---|
| 状态同步 URL | 交互状态只保留**非默认值**写入 `route.query`（`utils/share.ts` 的 `changedOnly`），因此「默认状态」等价于「无参数链接」，链接短且可读 |
| 可分享工具（18 个） | 物品（`/items`）、配方（`/recipes`）、指令辅助（`/commands`）、版本（`/versions`）、资源（`/resources`）、搜索（`/search`）、工具（`/tools`，既有）＋ §14 新增：**物品 ID 对照表**（`q`/`kind`）、**坐标换算**（`dir`/`x`/`y`/`z` 与线路规划 `fx`/`fz`/`tx`/`tz`）、**种子地图**（`seed`/`version`/`radius`/`type`）、**种子与结构工具**（各子表单，编解码见 `utils/seed-tools-query.ts`）＋ §9-A 新增：**药水与酿造**（`q`/`category`/`version`/`page`）＋ §9-B 新增：**村民交易**（`q`/`kind`/`version`/`page`）＋ §9-C 新增：**生物资料**（`q`/`category`/`version`/`page`）＋ §15 新增：**红石资料**（`tab`/`q`/`category`/`purpose`/`difficulty`/`version`/`page`）＋ §16 新增：**Mod 兼容性**（`tab`/`q`/`loader`/`category`/`version`/`page`）＋ §21 新增：**开发文档**（`q`/`loader`/`category`/`version`/`page`）、**版本映射查询**（`q`/`set_id`/`mapping_type`/`kind`/`page`） |
| 打开即复现 | 带参数的链接打开后直接还原状态；**种子地图带 `seed` 时自动执行分析**，无需再次点击 |
| 参数纠错 | 路由参数一律类型安全读取（`queryString` / `queryNumber` / `queryBool`），缺失或非法时回退默认值；非法版本、非法结构类型（非 `snake_case`）同样回退，避免坏链接触发后端 422 |
| 秘密信息防护 | 复制内容取自地址栏（Cookie 为 HttpOnly、CSRF 从不进入 URL），并额外用 `sanitizeShareUrl` 剥离疑似机密的查询参数与 hash 片段（`token`/`csrf`/`session`/`cookie`/`password`/`authorization`/`api_key`/`signature` 等），由单测锁定 |
| 分享入口 | 通用组件 `ShareButton`（默认复制当前页链接，支持 `url`/`label` 自定义），置于上述 **18 个工具页**标题右侧 |
| 明确不纳入 URL | **可视化指令生成器 / 附魔与装备计算器**属「表单配置」类，以「复制产出」（`CopyButton`）为分享方式；**存档诊断** 输入为本地文件，无法也不应用 URL 复现 |

### 2.18 界面状态与可访问性规范（本批新增）

| 项目 | 规范 |
|---|---|
| 加载态 | 列表用 `LoadingSkeleton`（作为**单一** `role="status"` 区域播报，占位块 `aria-hidden`）；分栏内的局部加载用带 `role="status" aria-live="polite"` 的文案 |
| 空态 | 用 `EmptyState`（标题 + 说明 + 可选操作），不得只渲染空表格 |
| 错误态 | 用 `ErrorState`（统一 `role="alert"` + 可选重试）；局部错误块同样带 `role="alert"`，字段级错误经 `FormField` 就近展示并带 `role="alert"` |
| 键盘可达 | 全站 `:focus-visible` 焦点环；顶部/侧边导航提供「跳到主要内容」跳转链接；Toast 支持键盘关闭 |
| 表格语义 | 表头 `<th>` 统一 `scope="col"`；宽表置于 `overflow-x-auto` 容器内 |
| 长文本 | `.code-block` 允许任意位置换行（`overflow-wrap: anywhere`），flex 行内的代码块加 `min-w-0`，避免移动端横向溢出 |

### 2.19 药水与酿造资料（本批新增，TODO §9-A）

目标：把「药水怎么酿、喝了什么效果、持续多久、能否延长/增强」变成**可检索、可溯源**的结构化资料。

| 能力 | 说明 |
|---|---|
| 药水资料 | 26 种药水（`water` / `base` / `positive` / `negative` / `mixed` / `other` 六类），含别名、简述、补充说明、`is_brewable` / `is_extendable` / `is_enhanceable`、版本归属（Java / 基岩 / 双端） |
| 效果与时长 | `effects` 按**变体逐条**存储（`normal` / `extended` / `enhanced`），每条含效果 ID、中文名、等级（I/II…）、时长（秒，0 为瞬间）与说明；详情页按「普通 / 延长（红石）/ 增强（荧石）」三组展示，前端不做规则推算 |
| 酿造步骤 | `brewing_recipes` 39 条：**基底**（水瓶＋原料 → 基础药水，含平淡的 12 种原料）、**效果**（粗制的药水＋原料 → 效果药水）、**腐化**（药水＋发酵蛛眼 → 反向效果）、**通用修饰**（红石延长 / 萤石增强）、**形态转换**（火药喷溅 / 龙息滞留）；步骤链在详情页渲染为「基底药水 ＋ 原料 → 成品」 |
| 通用规则 | 独立接口 `POST /api/v1/potions/rules` 返回作用于任意药水的修饰/形态规则（**存库，前端不硬编码**） |
| 检索 | `POST /api/v1/potions` 支持关键词（slug / 中文名 / 英文名 / 别名）、类别、游戏版本与分页；列表页状态写入 URL（`q`/`category`/`version`/`page`）可分享复现 |
| 版本精确语义 | 药水适用版本用「自某版本起」声明（`1.0+` / `1.4.2+` / `1.7.2+` / `1.8+` / `1.9+` / `1.13+` / `1.21+`），查询侧用新增的 `versioning.Satisfies`：筛 `1.20` 命中 `1.8+`、**不命中** `1.21+`；与工具/资源的「声明文本匹配」不同，这里按版本大小比较 |
| 关联与溯源 | 「原料 → 物品详情页」「基底 / 结果 → 药水详情页」「来源 → Minecraft Wiki」；未收录原料回退为纯文本 ID（不生成死链）。每条药水带 `source_url`、`data_version`、`last_verified_at` |
| 数据来源 | Minecraft Wiki `Brewing` / 各 `Potion_of_*` 页面（2026-10-17 抓取）；基岩版差异（再生 2:00、剧毒 2:00 / 0:22）写入说明；**未核验项显式标注**（衰变药水只注明「效果数值未在本站核验」，不填猜测值） |
| 收藏 | 收藏类型白名单新增 `potion`，详情页可收藏并进入个人工作台 |
| 使用统计 | 与其余内置工具同口径（TODO §13）：页面访问计 `visit`（路由级自动上报）；复制「详情链接 / 酿造步骤」计 `use`；检索或详情请求失败计 `fail` |

### 2.20 村民交易资料（本批新增，TODO §9-B）

目标：把「村民卖什么、什么等级解锁、要多少库存经验、价格怎么浮动」变成**可检索、可溯源**的结构化资料。

| 能力 | 说明 |
|---|---|
| 职业资料 | `villager_professions` 16 条：13 个普通职业（盔甲匠 / 屠夫 / 制图师 / 牧师 / 农民 / 渔夫 / 制箭师 / 皮匠 / 图书管理员 / 石匠 / 牧羊人 / 工具匠 / 武器匠）+ 流浪商人 + 傻子 + 失业村民；含别名、工作站物品 ID 与中文名、类别（`normal` / `wandering` / `non_trading`）、简述、说明与交易机制说明 |
| 交易等级 | `villager_trade_levels` 5 级通用规则（新手 / 学徒 / 老手 / 专家 / 大师，经验门槛 0 / 10 / 70 / 150 / 250），**存库不硬编码**，经 `POST /api/v1/villagers/levels` 下发 |
| 交易表 | `villager_trades` 223 条：`cost_items`（村民想要，JSONB 数组，`{item_id,name_zh,count_min,count_max}`，`count_min==count_max` 为固定数量、否则为浮动区间）+ `result_item_id/result_item_name_zh/result_count`（玩家获得）；含 `max_uses`（库存）、`villager_experience`、`price_multiplier`（价格浮动系数）、`variant_note`（颜色 / 群系变体）与 `note`（附魔随机等） |
| 交易分组 | `trade_group ∈ {level,purchase,special,ordinary}`：普通职业按等级（`level` 1–5）；流浪商人分「收购（换绿宝石）/ 特殊 / 普通售卖」三组，`level=0`；CHECK 约束保证分组与等级一致 |
| 检索 | `POST /api/v1/villagers` 支持关键词（slug / 中英文名 / 别名 / 工作站名）、职业类别、游戏版本与分页；列表页状态写入 URL（`q`/`kind`/`version`/`page`）可分享复现 |
| 详情聚合 | `POST /api/v1/villagers/{slug}` 返回职业本体 + 全部等级规则 + 按等级/分组聚合的交易（`groups`）+ 关联物品摘要（`resolved_items`），前端不做规则推算 |
| 版本精确语义 | 交易系统自 1.14 起默认 `1.14+`；个别交易收紧（试炼密室地图 `1.21+`、蜡烛 `1.17+`、滴水石块 `1.17+`），查询侧沿用 `versioning.Satisfies` 精确判断（筛 `1.20` 命中 `1.14+`、不命中 `1.21+`） |
| 关联与溯源 | 「工作站 / 成本 / 结果 → 物品详情页」「来源 → Minecraft Wiki（Trading）」；未收录物品回退为纯文本 ID（不生成死链）。每条职业/交易带 `source_url`、`data_version`、`last_verified_at` |
| 数据来源 | Minecraft Wiki `Trading` 页面（2026-10-18 抓取）；颜色 / 生物群系变体（羊毛、地毯、床、陶瓦、旗帜、船、药箭等）合并为单条交易并在 `variant_note` 说明，不逐一展开；**未发布内容**（Poplar、Sulfur Spike）不收录 |
| 收藏与统计 | 收藏类型白名单新增 `villager`，详情页可收藏并进入个人工作台；使用统计与其余内置工具同口径（页面访问 `visit`、复制链接/交易 `use`、请求失败 `fail`） |

### 2.21 我的世界开发栏目（本批新增，TODO §21）

目标：把「模组 / 数据包怎么开发、用哪些类与方法、混淆名怎么对照」变成**可检索、可溯源**的结构化知识库 + 查询工具。

| 能力 | 说明 |
|---|---|
| 开发文档知识库 | `dev_docs` 共 40 条：NeoForge 11 / Forge 5 / Fabric 9 / 原版数据 8 / 跨平台 1 / 工具链 6；每条含中英文标题、平台、主题、摘要、标签、适用版本声明、正文（Markdown）与**结构化关键接口清单**（合计 189 条） |
| 平台与主题 | 平台 `neoforge` / `forge` / `fabric` / `vanilla` / `tooling` / `multi`（`multi` 用于跨加载器主题）；主题 `getting_started` / `events` / `registry` / `networking` / `datagen` / `resources` / `rendering` / `serialization` / `api_reference` / `mapping` / `toolchain` |
| 关键接口速查 | 接口条目独立于正文结构化存储（`kind`：类 / 方法 / 事件 / 字段 / 命令 / 文件 / 插件，附签名与一句话说明），详情页可「整段复制」；避免 API 名称散落在正文里无法检索 |
| 检索 | `POST /api/v1/dev-docs` 支持关键词（slug / 中英文标题 / 摘要 / 标签 / 接口 / 正文）、平台、主题、游戏版本与分页；筛选项与计数由 `POST /dev-docs/facets` 提供（前端不硬编码枚举），列表页状态写入 URL（`q`/`loader`/`category`/`version`/`page`）可分享复现 |
| 版本精确语义 | 适用版本沿用「声明式版本集合」（`1.20.1+` / `1.13-1.20.1` / `all`），查询侧用 `versioning.Satisfies` 做**精确判断**：筛 `1.21` 命中声明 `1.20.1+` 的条目，不命中声明 `1.22+` 的条目；`all` 对任何版本命中 |
| 版本映射查询 | `mapping_sets`（数据集：版本 + 映射类型 + 渠道 + 来源 + 许可 + 命名空间说明）/ `mapping_classes` / `mapping_members` 两级结构；`obf` / `mid` / `named` 三列名字按映射格式填空（未提供的留空显示 `—`）；同一关键词同时匹配三列，结果按类聚合（类名命中优先，仅成员命中排其后），命中的成员以预览附带，展开可查看成员明细并按类型 / 名称过滤 |
| 映射导入器 | `cmd/import-mappings`（解析包 `internal/mappings`）支持 **ProGuard（Mojang 官方映射）/ TSRG·TSRG2（MCPConfig）/ Tiny v1·v2（Fabric Yarn）**，自动识别格式与 gzip 解压；按 `set_key` **整体替换**导入（单事务），成员批量写入并回填计数 |
| 不预置映射数据 | 真实混淆名随版本变化且无法在本仓库核验，**不预置任何映射数据**；未导入时页面展示**可复制的导入命令**（Mojang / Yarn / TSRG 三类）与使用指南链接，而不是伪造示例数据 |
| 来源与溯源 | 每条文档带 `official_url`（官方文档）与 `source_url`（可核验来源，可为官方仓库的文档源文件——例如官方站在核对网络中不可达时以仓库源文件为锚点并在说明中注明）+ `data_version` + `last_verified_at`；每个映射数据集强制记录 `source_url` 与 `license`，并在 `note` 说明各列对应的命名空间 |
| 收藏与统计 | 使用统计与其余内置工具同口径：页面访问计 `visit`（路由级自动上报）；复制链接 / 接口 / 正文 / 导入命令 / 成员列表计 `use`；检索或详情请求失败计 `fail` |
| 全站搜索接入 | 开发文档已接入全站搜索（`internal/search` 的 `devdoc` 类型）：命中中英文标题 / slug / 摘要 / 正文（`body_md`）/ 标签，仅返回已发布条目，结果直达 `/dev/{slug}`；`dev_docs.search_vector` 生成列 + GIN 索引随迁移 `0020` 建立 |
| 未做 | 仓库内不含任何真实映射文件（需使用者按官方来源导入） |

### 2.22 生物资料（本批新增，TODO §9-C）

目标：把「生物掉什么、在哪里生成、能不能驯服 / 繁殖」变成**可检索、可溯源**的结构化资料，并显式区分「确定数值」与「无法确定、只能文字描述」的字段。

| 能力 | 说明 |
|---|---|
| 生物资料 | `mobs` 48 条：被动型 20（牛 / 猪 / 羊 / 鸡 / 兔子 / 马 / 蘑菇牛 / 鱿鱼 / 发光鱿鱼 / 海龟 / 熊猫 / 狐狸 / 山羊 / 青蛙 / 悦灵 / 嗅探兽 / 骆驼 / 犰狳 / 鹦鹉 / 炽足兽）、中立型 9（狼 / 猫 / 铁傀儡 / 北极熊 / 羊驼 / 末影人 / 僵尸猪灵 / 海豚 / 蜜蜂）、敌对型 16（僵尸 / 骷髅 / 苦力怕 / 蜘蛛 / 女巫 / 史莱姆 / 烈焰人 / 恶魂 / 凋灵骷髅 / 幻翼 / 溺尸 / 守卫者 / 潜影贝 / 唤魔者 / 监守者 / 旋风人）、Boss 2（末影龙 / 凋灵）、功能型 1（雪傀儡）；含中英文名、别名、类别、维度、群系说明、简述 |
| 血量与伤害 | `health` 用 **Java 版生命值**（1 点 = 半颗心，前端换算为「N 颗心」），无法确定时为 `NULL`；攻击伤害因**难度与版本差异过大**改用文字 `attack_note` 说明，**不写死单一数字** |
| 掉落 | `mob_drops` 70 条：`item_id` + 中文名成对保存，数量用区间 `count_min`/`count_max`（`count_min==count_max` 为固定数量），概率用 `chance`（0–1，**1 表示必定掉落**）；`conditions` 记录触发条件（如「用刷子刷取」「被玩家击杀」「幼年海龟成长为成年时」），`note` 补充说明（抢夺提高概率、击杀不掉落等） |
| 生成 / 驯服 / 繁殖 | `spawn_note`（生成条件）、`behavior_note`（行为特征）、`taming_note`（驯服方式）、`breeding_note`（繁殖方式）均以**文字**保存，不虚构数值；`dimensions` 数组表达出现维度（主世界 / 下界 / 末地） |
| 类别枚举 | `category ∈ {passive,neutral,hostile,boss,utility,other}`，DB CHECK 约束 + 前端展示顺序与后端 `domain.MobCategories` 一致 |
| 检索 | `POST /api/v1/mobs` 支持关键词（slug / 中英文名 / 别名）、生物类别、游戏版本与分页；列表页状态写入 URL（`q`/`category`/`version`/`page`）可分享复现 |
| 详情聚合 | `POST /api/v1/mobs/{slug}` 返回生物本体 + 掉落清单（每条附 `item_ref`）+ 关联物品摘要（`resolved_items`），前端不做推断 |
| 版本精确语义 | 按生物实际加入版本声明（兔子 `1.8+`、北极熊 `1.10+`、羊驼 `1.11+`、鹦鹉 `1.12+`、海龟 / 幻翼 / 溺尸 / 守卫者 `1.13+`、熊猫 / 狐狸 / 猫 `1.14+`、蜜蜂 `1.15+`、炽足兽 `1.16+`、山羊 / 发光鱿鱼 `1.17+`、悦灵 / 青蛙 / 监守者 `1.19+`、嗅探兽 / 骆驼 `1.20+`、犰狳 `1.20.5+`、旋风人 `1.21+`），查询侧用 `versioning.Satisfies` 精确判断（筛 `1.17` 命中 `1.0+`/`1.17+`、不命中 `1.20.5+`） |
| 关联与溯源 | 「掉落 → 物品详情页」「来源 → Minecraft Wiki」；未收录掉落物回退为纯文本 ID（不生成死链）。每条生物带 `source_url`、`data_version=wiki-2026-10`、`last_verified_at` |
| 数据来源 | Minecraft Wiki 各生物页面（2026-10-18 抓取）；颜色 / 变体（羊毛颜色、山羊角数量、海龟鳞甲获取方式等）合并为单条掉落并在 `conditions`/`note` 说明 |
| 收藏与统计 | 收藏类型白名单新增 `mob`，详情页可收藏并进入个人工作台；使用统计与其余内置工具同口径（页面访问 `visit`、复制链接/掉落 `use`、请求失败 `fail`） |
| 已知取舍 | 未收录嘎枝（Creaking，1.21.4）等较新 / 未充分核验生物；同种生物不同变体的血量差异（如史莱姆体型、马的血量区间）在 `behavior_note`/`note` 中文字说明，`health` 取代表性数值 |

### 2.23 用户投稿与纠错（本批新增，TODO §17）

目标：让用户能就**具体资料条目**提交纠错建议或补充信息，并保证投稿**默认待审核、审核可追溯、内容授权明确、恶意内容可控**。

| 能力 | 说明 |
|---|---|
| 投稿类型 | `feedback_type` 扩展为 `bug`（错误反馈）/ `resource_broken`（资源失效）/ `infringement`（侵权通知）/ `data_correction`（资料纠错）/ `resource_supplement`（资源补充信息）/ `other` |
| 关联资料 | `target_kind ∈ {resource,item,dev_doc,mob,villager,potion,tool,other}` + `target_ref`（slug / item_id），使投稿定位到「哪一条资料」，而非只留一个自由文本链接；DB CHECK 约束 + 前后端契约一致 |
| 默认待审核 | 所有投稿写入 `status=new`，管理端逐条审核为 处理中 / 已解决 / 已忽略 |
| 审核留痕 | 最近一次审核写入 `reviewer`（取自登录会话用户名）/ `reviewed_at` / `review_note`；`feedback_reviews` 表**追加保存**每次状态流转（`from_status` → `to_status` + 审核人 + 结论），不覆盖历史 |
| 反垃圾 | 沿用公开反馈限流（每 IP 30 分钟 5 次）；新增 24 小时时间窗内「类型 + 目标 + 标题 + 正文」SHA-256 摘要查重，重复提交返回 **409** |
| 恶意链接防护 | `related_url` 仅接受 `http`/`https`；拒绝脚本伪协议（如 `javascript:`）与 `localhost` / 私网 / 环回 / 未指定地址 |
| 授权条款 | 投稿类型（资料纠错 / 资源补充）须携带当前授权条款版本（`2026-10-18`）并勾选确认，服务端强校验；授权版本与接受时间落库。投稿须为原创或已获授权，接受后本站可按开源许可整理、转载与再发布 |
| 接口 | `POST /api/v1/feedback`（公开）、`POST /api/v1/admin/feedback/list`、`POST /api/v1/admin/feedback/update`（审核）、`POST /api/v1/admin/feedback/reviews`（历史） |
| 前端 | `/feedback` 投稿表单（类型 / 关联资料 / 授权条款）；管理后台「反馈管理」增加审核备注输入、审核人信息与可展开的审核变更历史 |

### 2.24 红石与机器设计资料库（本批新增，TODO §15）

目标：把「红石元件怎么工作、机器怎么搭、要多少材料」变成**可检索、可溯源、可审核**的结构化资料，并严守**版权红线**（未授权存档不给下载链接）。

| 能力 | 说明 |
|---|---|
| 元件资料 | `redstone_components` 27 条，覆盖五类：信号源 6（红石火把 / 红石块 / 拉杆 / 按钮 / 压力板 / 侦测器类）、传输 3（红石粉 / 红石中继器 / 红石比较器）、逻辑 2、执行 10（活塞 / 粘性活塞 / 发射器 / 投掷器 / 红石灯 / TNT 等）、传感 6（阳光探测器 / 标靶 / 幽匿感测体等）；含中英文名、别名、类别、简述 |
| 工作机制 / 信号 | `mechanism_note`（核心工作机制）与 `signal_note`（信号强度 / 衰减 / 充能规则）均以**文字**描述——信号强度、延迟等数值随版本与连接方式变化，**不写死**未经核验的数字 |
| 设计资料 | `redstone_designs` 15 条，覆盖六用途：农场 3 / 分类机 2 / 运输 2 / 门与机关 3 / 逻辑电路 3 / 实用装置 2；含 `summary` + `steps_md`（搭建步骤，Markdown 安全渲染） |
| 材料成本 | `materials` JSONB 数组，元素 `{item_id,name_zh,count_min,count_max}`；`count_max` 缺省等于 `count_min`（固定数量只显示单值）；`item_id` 为空串表示未收录物品，此时 `name_zh` 必填，前端回退为纯文本，**不生成死链**；数量一律用**区间**，不编造精确配方 |
| 适用版本 / 难度 / 作者 | `supported_versions`（声明式集合）+ `difficulty ∈ {easy,medium,hard}` + `author` / `author_url`；查询侧复用 `versioning.Satisfies` **精确判断** |
| 截图 / 视频 / 存档 | `screenshot_url` / `video_url` / `world_download_url` + `world_download_note`；**版权红线**：`world_download_url` 非空时 `license` 必填（DB CHECK 约束），未授权存档一律不展示下载链接，**不伪造下载直链** |
| 来源与许可 | 每条带 `source_url`（Minecraft Wiki）、`data_version=wiki-2026-10`、`last_verified_at`、`license` / `license_note` |
| 内容审核 | `review_status ∈ {pending,published,rejected}`（默认 `pending`），**仅 `published` 对公开接口可见**；审核人取自登录会话，审核备注 / 时间留痕，并写入审计日志 |
| 类别 / 用途枚举 | `category ∈ {source,transmission,logic,actuator,sensor,other}`、`purpose ∈ {farm,sorter,transport,door,logic,utility,other}`、`review_status` 均有 DB CHECK 约束 + 前端展示顺序与后端 `domain.Redstone*` 一致 |
| 检索 | `POST /api/v1/redstone/components`（关键词 / 类别 / 版本 + 分页）、`POST /api/v1/redstone/designs`（关键词 / 用途 / 难度 / 版本 + 分页，**仅 published**）；列表页状态写入 URL（`tab`/`q`/`category`/`purpose`/`difficulty`/`version`/`page`）可分享复现 |
| 详情聚合 | `POST /api/v1/redstone/components/{slug}` 返回元件本体；`POST /api/v1/redstone/designs/{slug}` 返回设计本体 + 材料清单（每条附 `item_ref`）+ 关联物品摘要（`resolved_items`），前端不做推断 |
| 管理端 | `POST /api/v1/admin/redstone/designs/list`（按审核状态 / 用途筛选，不过滤发布态）、`/admin/redstone/designs/review`（更新审核状态 + 备注，审核人取自登录会话） |
| 导入器 | `cmd/seed --type redstone` 逐行校验（来源 / 版本必填、类别 / 用途 / 难度 / 审核状态枚举、材料数量区间 min ≤ max 且 ≥ 0、空 `item_id` 必须有中文名、未授权存档不得带下载链接）失败逐行汇总不中断；幂等 upsert + 写 `data_import_jobs` |
| 收藏与统计 | 收藏类型白名单新增 `redstone_design`，设计详情页可收藏并进入个人工作台；使用统计与其余内置工具同口径（页面访问 `visit`、复制 `use`、请求失败 `fail`） |
| 已知取舍 | 材料数量为**区间估计**（未收录精确配方），元件信号强度 / 延迟不写死数值；存档下载仅在明确标注授权协议时展示；种子数据仅收录已充分核验的元件与设计 |

### 2.25 Mod 与整合包兼容性查询（本批新增，TODO §16）

目标：把「Mod 要什么前置、和谁冲突、整合包里都有什么」变成**可检索、可溯源、可审核**的结构化资料；只呈现**已记录**关系（不做自动冲突分析），并严守**版权红线**（只链接官方 / 授权分发入口，不镜像分发文件）。

| 能力 | 说明 |
|---|---|
| Mod 资料 | `mods` 17 条（每 Mod ≥2 个发布版本，共 `mod_versions` 34 条）；含中英文名、别名、简述、说明，以及 `categories` / `loaders` / `mc_versions` |
| 发布版本 | `mod_versions` 关联同一 Mod 的不同发布版本：`release_type ∈ {release,beta,alpha}`、适用版本 / 加载器、`file_name`、`published_at`、`verified`（验证状态）+ `verify_note`；按 `(mod_id, version)` upsert 并清理已删版本 |
| 依赖与冲突 | `mod_dependencies` 12 条，`dep_kind ∈ {required,optional,embedded,incompatible}`（DB CHECK 约束）；目标 Mod 已收录则引用 `target_mod_id`，未收录回退 `target_mod_name`（二者必居其一）；逐条带 `source_url` 与 `verified` |
| 不做自动冲突分析 | 只呈现**已记录**的依赖 / 冲突，**不推断**未记录关系；自动冲突分析列为后续评估项（见 TODO §16） |
| 整合包 | `modpacks` 7 条（`mc_version` 目标版本精确匹配 + `loader` + `author`）；`modpack_mods` 57 条清单项（每包 ≥6 条），已收录解析为可跳转引用，未收录回退纯文本名，**不生成死链** |
| 版权红线 | `download_url` 只允许官方 / 作者授权分发入口，**不镜像分发**；非空时所属 Mod 的 `license` / `license_note` 必须已说明（导入器强校验），无授权说明时前端回退 `download_note` 文字说明，**不伪造下载直链** |
| 来源与许可 | 真实 `source_url`（Modrinth / GitHub / 官方站点）、`data_version=curated-2026-10`、`last_verified_at`、`license` / `license_note`；不确定许可的 Mod 留空 `license` 并用 `license_note` 说明「见官方仓库 LICENSE」 |
| 内容审核 | `review_status ∈ {pending,published,rejected}`（默认 `pending`），**仅 `published` 对公开接口可见**；审核人取自登录会话，审核备注 / 时间留痕，并写入审计日志 |
| 检索 | `POST /api/v1/mods`（关键词 / 加载器 / 类别 / 版本 + 分页）、`POST /api/v1/modpacks`（关键词 / 加载器 / 目标版本 + 分页，**仅 published**）；列表页状态写入 URL（`tab`/`q`/`loader`/`category`/`version`/`page`）可分享复现 |
| 详情聚合 | `POST /api/v1/mods/{slug}` 返回 Mod 本体 + 发布版本 + **已记录**依赖与冲突（每条附 `target_mod` 摘要）+ 反向依赖（谁依赖了它）；`POST /api/v1/modpacks/{slug}` 返回整合包本体 + 包内 Mod 清单（每条附 `mod_ref`），前端不做推断 |
| 管理端 | `POST /api/v1/admin/mods/list`（按审核状态 / 加载器筛选，不过滤发布态）、`/admin/mods/review`、`/admin/modpacks/list`、`/admin/modpacks/review`（更新审核状态 + 备注，审核人取自登录会话） |
| 导入器 | `cmd/seed --type mods` 两阶段导入（先落全部 Mod 再写依赖，避免依赖引用列表中靠后的 Mod 时悬空），逐行校验（来源 / 版本必填、加载器 / 类别 / 发布类型 / 依赖类型 / 审核状态枚举、缺授权说明的下载入口、悬空引用）失败逐行汇总不中断；幂等 upsert + 写 `data_import_jobs` |
| 收藏与统计 | 收藏类型白名单新增 `mod` / `modpack`，Mod / 整合包详情页可收藏并进入个人工作台；使用统计与其余内置工具同口径（页面访问 `visit`、复制 `use`、请求失败 `fail`） |
| 已知取舍 | 只呈现已记录关系，不做自动冲突分析；下载入口只指向官方 / 授权分发，不镜像分发文件；种子数据仅收录已充分核验的 Mod 与整合包 |

### 2.26 MC 源码浏览器（mcsrc 整合 · 切片 1 + 2 + 3-D1 + 3-D2）

目标：把 `mcsrc-main`（浏览器内 MC 反编译源码浏览器）的能力**整合到 mckit 后端**，由服务端统一下载并缓存官方 jar，避免每个访客各自从 Mojang 下载数百 MB。

| 能力 | 说明 |
|---|---|
| 版本选择 | 复用既有 `POST /api/v1/versions`（Mojang 清单），页面左侧按版本号搜索 + 类型（正式版 / 快照 / 旧测试版 / 旧内测版）过滤 |
| jar 获取与缓存 | `POST /api/v1/mcsrc/prepare`（**需 `ops.mcsrc` 权限**）：按版本取官方下载地址，下载 jar 并**校验官方 sha1**，落盘为 `<version>-<side>.jar`；幂等（摘要一致时直接返回，不重复下载） |
| 下载安全 | 版本号白名单正则校验（防路径穿越）、单 jar 体积上限（`MCSRC_JAR_MAX_MB`，默认 512）、临时文件 + 校验通过后原子改名（不产生半成品缓存）、同一 `version+side` **单飞锁**（并发准备只下载一次） |
| 类索引 | 只读取 zip 中央目录（不解析 class 字节码），因此数百 MB 的 jar 也能秒级建索引；索引落盘为 `<version>-<side>.index.json`，类数量上限 `MCSRC_INDEX_MAX_CLASSES`（默认 60000，超出截断并标记 `truncated`） |
| 类检索 | `POST /api/v1/mcsrc/classes/list`：关键词（大小写不敏感）、包名前缀过滤、是否含内部类、分页；相关度排序为「简单名相等 > 简单名前缀 > 简单名包含 > 全限定名包含」，同档按全限定名升序（稳定可复现） |
| 类结构视图（切片 2） | `POST /api/v1/mcsrc/classes/detail`：纯 Go 解析**单个 class 字节码**（不反编译、不依赖 JVM），返回修饰符、种类（class / interface / enum / annotation）、class 文件版本、父类与直接接口、字段与方法（JVM 描述符渲染为 Java 类型：`(Ljava/lang/String;I)V` → `void m(java.lang.String, int)`）；`in_jar_super` / `in_jar_interfaces` 标记父类 / 接口是否也在本 jar 内，供前端继续下钻 |
| 继承关系（切片 2） | `POST /api/v1/mcsrc/classes/hierarchy`：祖先链（直接父类逐级向上，直至跳出 jar 的终点，如 `java.lang.Object`）+ 直接子类（含实现本接口的类，上限 500，超出标 `truncated`）+ 直接 / 递归子类计数；首次调用扫描本 jar 内全部 class 建立关系索引并按 **jar 摘要缓存于内存**（只留最近 4 个版本），之后毫秒级 |
| 状态与列表 | `POST /api/v1/mcsrc/status`（该版本是否已缓存 jar / 已建索引、体积、类数量、时间）、`POST /api/v1/mcsrc/prepared`（全部已缓存版本）；均为公开只读 |
| jar 下发（切片 3-D1） | `GET /api/v1/mcsrc/jar?version=&side=`（**公开只读**）：流式返回已缓存的 jar，支持 `Range` 与条件请求（`ETag` = 官方 sha1）；未缓存 → 404 且不触发下载。**本模块唯一的 GET 接口**——二进制大文件下载需要 `Range` / `ETag` 语义 |
| 源码浏览（切片 3-D1） | 前端在**浏览器内**用 Vineflower 的 WASM 移植版（npm `@run-slicer/vf`，别名 `vf-1.12.0`）反编译，**不依赖部署侧 JVM**；jar 按 `(version, side)` 只下载一次并缓存在浏览器内存；zip 由 `@katana-project/zip` 在 Web Worker 内解析（Worker 内按 jar 缓存条目表）；源码用 Monaco 只读展示（Java 高亮、跟随站点明暗主题），Monaco 与 Worker 均**懒加载**；WASM 不可用时自动回退包内纯 JS 运行时 |
| 源码 token 与着色（切片 3-D2） | 反编译时向引擎传 `tokenCollector`，把类 / 字段 / 方法 / 参数 / 局部变量的**字符区间**随源码一并返回（实测 1.8.9 客户端 jar 的 `MinecraftServer` 产出 1493 个 token）；Monaco 按类型着色（类 / 方法 / 字段 / 变量分色，明暗双主题各一套配色），**jar 内**的引用 token 显示手型光标 + 点状下划线 |
| 点击跳转（切片 3-D2） | 注册 Monaco 定义提供器 + 编辑器打开器：**Ctrl/⌘ + 点击**、**F12**、右键「转到定义」均可跳转。类 token → 打开该类源码；字段 / 方法 token → 先沿**继承链**定位真正的声明类（先查引用点所属类，未命中则按祖先链逐级向上；数据取**已建好的类索引**，不重新下载、不重新反编译），再打开该类并**选中成员声明**；目标即当前类时只重新定位、不重复反编译 |
| 类名对齐（切片 3-D2） | 反编译源码里内部类写作 `Outer.Inner`（Java 语法），而 jar 条目与后端索引写作 `Outer$Inner`；Worker 用 jar 条目集合做候选探测（原名 → 最外层类 → 逐级把包分隔符换成 `$`）把 token 的类名对齐到后端约定，因此内部类引用同样可跳转 |
| 权限与限流 | 只读接口公开（含 `status` / `prepared` / `classes/list` / `classes/detail` / `classes/hierarchy` / **`jar`**）；**触发下载 / 建索引为管理员专属**（`ops.mcsrc` 权限点 + CSRF），并叠加重接口限流（`MC_HEAVY_RPS` / `MC_HEAVY_BURST`）与并发上限 1（禁止并发叠加下载） |
| 前端页面 | `/mcsrc`（工具中心内置工具「MC 源码浏览器」，`slug=mcsrc`）：版本列表 + 侧别（客户端 / 服务端）切换 + 状态卡片 + 类检索表格；点击任一类进入**类详情**（结构视图 + 继承关系，父类 / 接口 / 祖先 / 子类均可点击继续下钻，合成方法默认折叠）；类列表与详情面板均有「查看源码」进入**源码视图**（反编译中 / 失败 / 源码三态，切换版本 / 侧别 / 翻页自动关闭；面板顶部提示可跳转）；管理员可直接「准备该版本」，非管理员提示联系管理员 |
| 版权与取舍 | **jar 不纳入仓库 / 数据库**：仅在服务端缓存目录按版本落盘，且只向浏览器下发**已缓存**的官方 jar 原件（不改造、不打包分发）；反编译在访客浏览器内完成，服务端不产出、不存储源码正文 |
| 分期路线 | 切片 1（已实现）：版本选择 + jar 获取 + 类索引 / 搜索；切片 2（已实现）：类结构视图 + 继承关系（纯 Go 解析 class 字节码，**无需 JVM**）；切片 3-D1（已实现）：源码浏览（浏览器内 Vineflower WASM 反编译 + Monaco 展示）；**切片 3-D2（已实现）：token 着色 + 点击跳转（跳转定义，沿继承链定位声明类）**；切片 3 余项：查找引用、字节码指令视图、映射重命名展示、Javadoc 备注 |

> 与 `mcsrc-main` 的差异：原项目自行在浏览器内下载 jar、用 IndexedDB 缓存反编译结果；mckit 侧改为**后端统一下载与缓存 jar**（访客不各自从 Mojang 拉取，且只下发已缓存的官方原件），但**反编译引擎与调用形态与上游一致**——同样使用 `@run-slicer/vf`（Vineflower 的 WASM/JS 移植）在 Web Worker 内完成，因此**不需要部署侧 JVM**。
>
> 跳转能力同样**对齐上游语义**（token 类型、`declaration` 标记、定义提供器 + 编辑器打开器、沿继承链定位声明类）；两处有意的差异：① import 行的 class token 覆盖**整个导入名**（含包路径，上游只覆盖简单名），点击更省事；② 类名用 jar 条目集合做**候选探测**对齐到 `Outer$Inner`，因此内部类引用也可跳转（上游对内部类引用会因 `Outer.Inner` 无法命中 `Outer$Inner.class` 而放弃跳转）。上游的「查找引用」依赖其 Java 索引器（ASM），mckit 侧需要新增引用索引，故仍列在切片 3 余项。

---

## 三、Minecraft 专用计算能力（公开 API）

服务端实现的 MC 算法能力（`internal/mcstructure`、`mcbiome`、`mcrand`、`mcversion`、`nbt`、`region`）：

| 能力 | 接口 | 说明 |
|---|---|---|
| 结构坐标 | `POST /mc/structures` | 计算指定种子下目标结构的理论坐标，可指定 region 范围；1.18+ 附群系可行性校验（`feasible`）；返回 `available` / `placement_supported` / `supported` / `dimension` / `probability`；单次限时 3s、独立限流 |
| 结构类型清单 | `POST /mc/structures/types` | 返回支持的结构类型及其参数描述（版本、维度、频率门槛、群系约束） |
| 能力版本矩阵 | `POST /mc/capabilities` | 各世界分析能力在四个版本区间的支持状态与说明 |
| 群系查询 | `POST /mc/biome` | 按种子与方块坐标返回地表群系（仅 `1.18+`，其他版本 `supported=false`） |
| 要塞估算 | `POST /mc/strongholds` | 估算前 N 个要塞位置 |
| 史莱姆区块 | `POST /mc/slime-chunks` | 查询指定范围内的史莱姆区块（单次限时 3s、独立限流，超时标注 `timed_out`） |
| 随机种子生成 | `POST /mc/seeds/generate` | 生成随机种子 |
| 种子搜索 | `POST /mc/seeds/find` | 搜索满足「结构在目标坐标附近」的种子（单次限时 5s，独立限流） |
| 种子解析 | `POST /mc/seeds/parse` | 将文本种子解析为 Java 种子数值（支持数字与文本） |
| 种子分析 | `POST /seed/analyze` | 综合：世界种子、史莱姆区块、结构候选坐标、（可选）生物群系采样 |
| 版本元数据 | `POST /versions`、`POST /versions/detail` | Mojang 版本清单与详情（磁盘缓存，TTL 可配） |
| 物品对照表 | `POST /block-ids` | 命名空间 ID / 中文名检索与分类过滤 |
| 区块文件检查 | `POST /mc/region/inspect`、`POST /mc/region/chunk` | 解析上传的 `.mca`：区块清单 + 统计 + 只读诊断（上限 64MiB，仅内存 + 有限缓存） |
| 世界存档解析 | `POST /mc/level/parse` | 解析上传的 `level.dat`：世界名称 / 种子 / 版本 / 出生点 / 游戏规则 + 只读告警（上限 64MiB，解压输出上限 16MiB） |

> 安全约束：区块与存档文件仅内存解析、不落盘；上传大小、解压输出与 NBT 解析深度均受限；`region_token` 缓存有总量上限与有效期；上传解析类接口叠加独立限流与并发上限（超出返回 429）。诊断仅做只读检查，不提供自动修复。

---

## 四、管理后台功能

### 4.1 认证与登录

| 功能 | 说明 |
|---|---|
| 登录 | 用户名 + 密码；服务端校验凭证并种下会话 Cookie；返回 `csrf_token` |
| 登录限流 | 失败 5 次 / 15 分钟（按 IP 隔离） |
| 会话 | 内存会话存储，默认 TTL 72h；`HttpOnly` + `SameSite`，生产强制 `Secure` |
| 当前用户 | `me` 接口返回当前管理员信息与刷新后的 `csrf_token` |
| 登出 | 使当前会话失效 |
| 修改密码 | 需旧密码校验，新密码 ≥8 位；**成功后该管理员全部会话立即失效** |
| 账号设置页 | `/admin/profile`：自助改密，成功后清本地状态并回登录页 |

### 4.2 仪表盘

- `/admin`：资源总数、工具总数、百科总数、待处理反馈（new）等概览统计。

### 4.3 内容管理

| 模块 | 路由 | 能力 |
|---|---|---|
| 资源管理 | `/admin/resources` | 列表（筛选/分页）、新建、编辑、软删除、链接检查；**发布版本维护**（同一资源不同发布版本整体替换）、**依赖 / 前置 / 已知冲突编辑**、兼容性核验备注 |
| 工具管理 | `/admin/tools` | 列表、新建、编辑、下架/删除；白名单字段更新；分类为中文枚举 |
| 百科管理 | `/admin/wiki` | 列表、新建、编辑、下架/删除；Markdown 正文 |
| 分类管理 | `/admin/categories` | 按内容类型（tool/resource/wiki）维护分类，含排序 |
| 标签管理 | `/admin/tags` | 标签增删改查 |

- 所有写操作走 `POST` 动作子路径（并兼容 REST 风格），统一挂 `RequireAuth + CSRF`。
- 工具分类白名单：查询类 / 计算类 / 生成类 / 编辑类 / 辅助类。
- 发布状态：`draft` / `published` / `hidden`（资源另有 `removed`）。

### 4.4 运营管理

| 模块 | 路由 | 能力 |
|---|---|---|
| 反馈管理 | `/admin/feedback` | 列表（按类型/状态筛选）、审核（状态 + 备注，记录审核人）与审核变更历史；投稿默认待审核 |
| 操作日志 | `/admin/audit-logs` | **admin 专属**；审计日志查询（操作者、动作、对象、请求 ID、来源 IP、结果、时间） |
| 红石设计审核 | `/admin/redstone` | **admin 专属**；机器设计审核列表（按审核状态 / 用途筛选，默认只看待审核），逐条「通过 / 驳回」并留审核备注 |
| Mod 审核 | `/admin/mods` | **admin 专属**；Mod / 整合包双 tab 审核（按审核状态 / 加载器筛选，默认只看待审核），逐条「通过 / 驳回」并留审核备注 |

> 机器设计内容审核（TODO §15）：提供管理端接口 `POST /api/v1/admin/redstone/designs/list`（按审核状态 / 用途筛选）与 `/admin/redstone/designs/review`（更新审核状态 + 备注，审核人取自登录会话并写入审计日志），并配套后台审核页 `/admin/redstone`（默认只看待审核项，逐条通过 / 驳回）；审核状态流转与留痕完整落库（`review_status` / `reviewer` / `reviewed_at` / `review_note`）。

> Mod 与整合包内容审核（TODO §16）：提供管理端接口 `POST /api/v1/admin/mods/list`、`/admin/mods/review`、`/admin/modpacks/list`、`/admin/modpacks/review`（按审核状态 / 加载器筛选 + 更新审核状态 + 备注，审核人取自登录会话并写入审计日志），并配套后台审核页 `/admin/mods`（Mod / 整合包双 tab，默认只看待审核项，逐条通过 / 驳回）；审核状态流转与留痕完整落库（`review_status` / `reviewer` / `reviewed_at` / `review_note`）。

### 4.5 管理员账号管理（需 `accounts.admins` 权限，已交付）

路由导航：`系统 → 管理员账号`（`/admin/admins`，无权限时自动隐藏，后端 403 兜底）。

| 功能 | 说明 |
|---|---|
| 账号列表 | 按用户名模糊搜索、分页；**响应不含任何密码信息** |
| 创建账号 | 用户名（1-80 位，唯一）、密码（**≥12 位**）、角色（从 `roles` 表动态加载） |
| 改角色 | 角色变更后该账号现有会话立即失效，需重新登录 |
| 启用 / 停用 | 停用后该账号现有会话立即失效，无法登录 |
| 重置密码 | 不需旧密码；密码 ≥12 位；成功后该账号全部会话失效 |
| 删除账号 | 物理删除；成功后该账号全部会话失效 |

**安全守卫**：用户名唯一（409）；角色必须存在于 `roles` 表（自定义角色可立即分配）；**不可停用/删除自己**；**不可降级或移除最后一名启用状态的 admin**（避免系统失去可管理性）。

### 4.6 数据质量监控（admin 专属，已交付）

路由导航：`系统 → 数据质量`（`/admin/data-quality`，editor 自动隐藏，后端 403 兜底）。

| 能力 | 说明 |
|---|---|
| 质量概览 | 客观检查项（**不做主观评分**）：字段缺失、版本格式非法 / 重复 / 起点大于终点、配方结果物品**悬空引用**、近 30 天失败导入任务 |
| 严重 / 提醒分级 | `error` = 数据不一致或引用断裂；`warning` = 字段缺失（历史数据可能尚未补齐，不阻断使用）；`info` = 条目统计 |
| 占比展示 | 每项展示「命中 / 总数」与百分比，便于判断影响范围 |
| 导入任务 | 分页查询（可按 `data_type` 过滤）每次导入的**来源文件、总数、新增 / 更新 / 跳过 / 失败、执行者、错误摘要**，失败摘要可展开 |
| 唯一性说明 | `item_id` / `recipe_key` / `slug` 的唯一性由数据库约束保证，监控页不重复校验 |

> 新增 / 更新数量是**准确值**：PostgreSQL 的 `INSERT ... ON CONFLICT DO UPDATE` 无论插入还是更新都返回 1 行，`RowsAffected` 无法区分；因此导入器显式预查业务主键来区分新增与更新。

### 4.7 权限模型（RBAC：角色 + 权限点）

角色存于 `roles` 表（`name` 为主键、**不可改名**），权限以 JSONB 数组存放；账号通过 `accounts.role` 外键引用角色（迁移 0022 统一账号后管理员与普通用户同表）。

| 角色 | 权限范围 |
|---|---|
| `admin`（内置） | **全量权限**；权限可调整但**必须保留全部权限点**（削权返回 422） |
| `editor`（内置） | 内容管理（资源/工具/百科/分类/标签）、反馈处理、账号设置 |
| `user`（内置） | 无后台权限；即统一账号中的**普通用户**（`accounts.role = 'user'`） |
| 自定义角色 | 由具备 `roles.manage` 权限的管理员创建，权限点任意组合（如只给「角色管理」或只给「MC 源码库」） |

- **内置角色**（`is_system=true`）不可删除、不可改名，但权限可调整（`admin` 例外，见上）。
- 自定义角色**仍被账号引用时不可删除**（409），需先改走该角色的账号。
- 路由鉴权由硬编码 `RequireRole(admin)` 改为 **`RequirePermission(roleSvc, <权限点>)`**，内置角色行为与改造前等价（editor 的 403 边界不变）。
- 权限点目录（**17 项，4 组**，见 `POST /api/v1/admin/roles/permissions`）：

| 分组 | 权限点 |
|---|---|
| 内容管理 | `content.resources` / `content.tools` / `content.wiki` / `content.categories` / `content.tags` / `content.feedback` |
| 内容审核 | `review.redstone` / `review.mods` |
| 账号与权限 | `accounts.admins` / `accounts.users` / `roles.manage` |
| 系统与运营 | `ops.audit` / `ops.dataquality` / `ops.searchstats` / `ops.analytics` / `ops.linkhealth` / `ops.mcsrc` |

- 登录 / `me` 接口返回当前账号的 `permissions`，前端据此按权限点显隐侧边栏入口；**前端显隐仅为体验优化，真正校验在后端**（403 兜底）。
- 权限点变更后，受影响账号的**下一次请求即生效**（权限实时查表，不缓存到会话）。

### 4.8 搜索词统计（需 `ops.search_terms` 权限，已交付）

路由导航：`运营 → 搜索词统计`（`/admin/search-terms`，无权限时自动隐藏，后端 403 兜底）。

| 能力 | 说明 |
|---|---|
| 热门搜索 | 按被搜索总次数降序，反映真实关注点 |
| 高频无结果词 | 搜索后**没有结果**的关键词，代表**资料缺口**（无结果次数达 3 次标记「高频缺口」） |
| 查询失败词 | 查询本身失败（超时 / 服务错误）的关键词，需排查链路问题；与「无结果」**分开计数**，避免把故障误报为资料缺失 |
| 关键词过滤 | 支持按关键词模糊过滤与分页（`page_size` 上限 100） |
| 隐私立场 | 只存**归一化关键词 + 计数**：不存 IP、会话、用户标识，也不存逐次时间序列；归一化 = 去空白 + 折叠空白 + 转小写 + 限长 64 字符 + 丢弃纯符号 / emoji |

### 4.9 运营看板（需 `ops.analytics` 权限，已交付）

路由导航：`运营 → 运营看板`（`/admin/analytics`，无权限时自动隐藏，后端 403 兜底）。数据为**按天聚合**的计数与比率，服务于「哪些工具真被用、哪些环节在失败、API 是否健康」。

| 能力 | 说明 |
|---|---|
| 时间范围 | 预设近 7 / 30 / 90 天，或自定义起止日期（`YYYY-MM-DD`，含首尾）；未显式指定时默认近 30 天，单次查询跨度上限 366 天 |
| 概览卡片 | 工具访问量 / 使用量 / 失败率、资源详情访问量与活跃资源数、搜索次数（含无结果与失败拆分）、API 请求量与错误率、P95 延迟（含均值）、限流触发与当前失效链接数 |
| 趋势折线 | 两张图：① 工具访问 / 使用 / 失败 + 资源访问 + 搜索次数；② API 请求量 / 5xx / 限流 429；区间内缺失日期由后端补零，保证曲线连续可读 |
| 工具明细 | 按 slug 列出访问 / 使用 / 失败与失败率（失败率进度条按阈值着色：≥10% 警示、≥30% 危险） |
| 资源明细 | 资源详情访问量 Top 榜（由后端在资源详情接口内自动累计） |
| API 明细 | 路由模板级请求量 / 5xx / 429 / 均延迟（含 `max_ms`）与**近似 P95**；`route` 只保留模板（如 `/api/v1/resources/{slug}`），不含具体参数值 |
| 高频无结果词入口 | 一键跳转 `运营 → 搜索词统计` 查看资料缺口 |
| 延迟口径 | 延迟以**固定分桶**（上界 `50 / 200 / 500 / 1000 / 3000 ms`，另含溢出桶）近似，P95 取**桶上界的保守值**；不存逐次耗时明细 |
| 采集与落库 | 前端匿名上报（见 §2.16）+ 后端在业务成功时直接累计；内存聚合后按周期（`STATS_FLUSH_SECONDS`，默认 60 秒）批量 UPSERT 落库，进程优雅退出时强制刷盘 |
| 保留期限 | `STATS_RETENTION_DAYS`（默认 180 天），后台按天自动清理过期聚合行 |
| 隐私立场 | 只存**按天聚合计数**：不存逐次明细、IP、会话、用户标识、UA、`request_id`、请求体与完整查询内容；API 只留路由模板 |
| 计数口径 | `errors` = 5xx；`client_errors` = 4xx（**不含** 429）；`rate_limited` = 429（与 `client_errors` 不重复计数）；工具失败率 = 失败数 / 使用数 |

> 说明：统计开启与保留期限由后端配置控制（`STATS_ENABLED` / `STATS_FLUSH_SECONDS` / `STATS_RETENTION_DAYS`）；看板全部为只读接口，无任何写入或修复动作。

### 4.10 角色管理（需 `roles.manage` 权限，本批新增）

路由导航：`系统 → 角色管理`（`/admin/roles`，无权限时自动隐藏，后端 403 兜底）。这是权限底座的可视化入口：**先有角色与权限点，再有账号分配**。

| 能力 | 说明 |
|---|---|
| 角色列表 | 一次性返回（内置 3 个 + 自定义少量），不分页；展示角色标识、展示名、说明、权限数量、是否内置 |
| 权限点勾选 | 权限点按 5 组展示（组名 + 组内中文名），逐项勾选；组内全选 / 全不选 |
| 新建角色 | `name` 为主键（不可改名，仅小写字母 / 数字 / 下划线 / 中划线）；权限点必须是目录中的合法值（非法值 422） |
| 编辑角色 | 仅更新传入字段；内置 `admin` 削权返回 422（防止系统失去可管理性） |
| 删除角色 | 内置角色不可删除（422）；仍被账号引用的角色不可删除（409，需先改走该角色的账号） |
| 即时生效 | 权限存表实时查询，改动后受影响账号**下一次请求即生效**；角色变更 / 删除写入审计日志 |

### 4.11 普通用户管理（需 `accounts.users` 权限，本批新增）

路由导航：`系统 → 普通用户`（`/admin/users`，无权限时自动隐藏，后端 403 兜底）。**只读列表**（统一账号后普通用户即 `accounts.role = 'user'` 的账号，本页一律按角色过滤，绝不返回后台账号）：

| 能力 | 说明 |
|---|---|
| 列表 | 按用户名模糊搜索（`q`）、按状态筛选（`status ∈ {active, disabled}`）、分页（`page_size` 上限 100） |
| 个人数据概览 | 每条附带 `favorite_count` / `recent_count` / `seed_count`（该用户收藏 / 最近使用 / 常用种子数量） |
| 仪表盘口径 | 后台仪表盘「用户总数」取自本接口 `total`（`page_size=1`），保证与列表口径一致 |

> 接口：`POST /api/v1/admin/users/list`（`RequireAuth + RequirePermission(accounts.users) + CSRF`）。查询固定叠加 `role = 'user'`，管理员账号不会混入列表。

---

## 五、认证与安全能力

| 能力 | 实现 |
|---|---|
| 密码存储 | Argon2id 加盐哈希（不落明文） |
| 会话 | Cookie 会话（HttpOnly + SameSite，生产 Secure）；内存存储（多实例需替换共享存储） |
| 用户会话隔离 | 统一账号（迁移 0022）：管理员与普通用户共用同一个会话 Cookie（`SESSION_COOKIE_NAME`，原 `USER_SESSION_COOKIE_NAME` 已废弃）；`/user/*` 用 `RequireAuth + RequireUserRole` 仅接受 `role=user` 会话，后台会话访问 → 401；后台接口按权限点判定，`role=user` 会话 → 403 |
| CSRF | 写操作双重提交校验（`X-CSRF-Token` 必须与会话一致）；统一会话后前后台共用同一套校验中间件，token 随会话独立 |
| 登录限流 | 管理员与普通用户均为 5 次 / 15 分钟（按 `用户名 + IP` 隔离） |
| 接口限流 | 全局 RPS 限流；种子搜索 `find` 独立小配额；结构候选 / 史莱姆区块等遍历型接口 `MC_HEAVY_*` 独立小配额；反馈接口独立限流 |
| 计算超时与并发 | `/mc/structures`、`/mc/slime-chunks` 单次计算限时 3s（超时返回部分结果并标注 `timed_out`）且并发上限 `MC_HEAVY_CONCURRENCY`（超出 429）；`/mc/seeds/find` 限时 5s；`/mc/region/inspect` 逐区块解析限时 5s 且与 `/mc/level/parse` 共用上传类限流与并发上限 |
| SSRF 防护 | 链接检查拒绝内网/回环/云元数据地址，重定向逐跳校验 |
| 输入校验 | 全部输入后端校验，SQL 参数化查询 |
| XSS | Markdown 渲染禁用原始 HTML |
| 安全响应头 | 统一 `SecurityHeaders` 中间件 |
| 审计 | 记录操作者/动作/对象/来源 IP/结果；**不记录密码、Cookie、Authorization、连接串** |
| 会话失效 | 改密、停用、改角色、重置密码、删除账号均使对应账号全部会话立即失效 |
| 统一错误 | 错误响应不泄漏内部信息（goroutine/SQL/堆栈等），带 `request_id` |

**中间件链**：`RequestID → Recover → AccessLog → Metrics → SecurityHeaders → CORS → RateLimit`；管理路由叠加 `RequireAuth → RequirePermission（权限点）→ CSRF`；`/mcsrc/prepare` 额外叠加 `RateLimit(MC_HEAVY_*) → ConcurrencyLimit(1)`。

---

## 六、API 一览

### 6.1 公开 API（`/api/v1`）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/health` | 健康检查 |
| POST | `/search` | 全站搜索（类型含工具 / 资源 / 百科 / 物品 / 开发文档；相关度 / 最近更新排序、类型与版本筛选、无结果建议；关键词上限 80 字符） |
| POST | `/tools`、`/tools/{slug}` | 工具列表 / 详情 |
| POST | `/resources`、`/resources/{slug}` | 资源列表 / 详情（详情含 `releases` 发布版本与 `compatibility` 兼容性结论；声明通配时 `all_versions=true`，无数据依据时 `verified=false`） |
| POST | `/items`、`/items/{item_id}` | 物品查询 / 详情 |
| POST | `/recipes` | 配方查询（每项附 `resolved_items` 关联物品摘要） |
| POST | `/potions`、`/potions/rules`、`/potions/{slug}` | 药水检索（关键词 / 类别 / 版本 + 分页）/ 通用酿造规则 / 药水详情（效果变体 + 酿出步骤 + 后续变化 + 关联物品摘要） |
| POST | `/villagers`、`/villagers/levels`、`/villagers/{slug}` | 村民职业检索（关键词 / 职业类别 / 版本 + 分页）/ 职业等级通用规则 / 职业详情（等级规则 + 分组交易 + 关联物品摘要） |
| POST | `/mobs`、`/mobs/{slug}` | 生物检索（关键词 / 生物类别 / 版本 + 分页）/ 生物详情（本体 + 掉落清单 + 关联物品摘要） |
| POST | `/redstone/components`、`/redstone/components/{slug}` | 红石元件检索（关键词 / 类别 / 版本 + 分页）/ 元件详情（工作机制 + 信号说明 + 来源与核验） |
| POST | `/redstone/designs`、`/redstone/designs/{slug}` | 机器设计检索（关键词 / 用途 / 难度 / 版本 + 分页，**仅 published**）/ 设计详情（本体 + 材料清单 + 关联物品摘要） |
| POST | `/mods`、`/mods/{slug}` | Mod 检索（关键词 / 加载器 / 类别 / 版本 + 分页，**仅 published**）/ Mod 详情（本体 + 发布版本 + 已记录依赖与冲突 + 反向依赖） |
| POST | `/modpacks`、`/modpacks/{slug}` | 整合包检索（关键词 / 加载器 / 目标版本 + 分页，**仅 published**）/ 整合包详情（本体 + 包内 Mod 清单，已收录才链接） |
| POST | `/dev-docs`、`/dev-docs/facets`、`/dev-docs/{slug}` | 开发文档检索（关键词 / 平台 / 主题 / 版本 + 分页）/ 筛选维度计数 / 文档详情（正文 + 关键接口 + 官方文档与来源 + 同平台相关条目） |
| POST | `/mappings/sets`、`/mappings/search`、`/mappings/class/{id}` | 映射数据集列表 / 映射查询（类聚合 + 成员预览 + 分页与截断标记）/ 类映射详情与成员列表 |
| POST | `/commands`、`/commands/{slug}` | 指令列表 / 详情 |
| POST | `/wiki/{slug}` | 百科详情 |
| POST | `/categories` | 分类列表 |
| POST | `/feedback` | 提交反馈 |
| POST | `/versions`、`/versions/detail`、`/block-ids` | MC 版本元数据 / 物品对照表 |
| POST | `/mcsrc/status`、`/mcsrc/prepared`、`/mcsrc/classes/list`、`/mcsrc/classes/detail`、`/mcsrc/classes/hierarchy` | MC 源码浏览器只读接口（jar 缓存与索引状态 / 已缓存版本 / 类索引检索 / 类结构视图 / 继承关系） |
| GET | `/mcsrc/jar?version=&side=` | 下发已缓存的官方 jar（支持 `Range` / `ETag`），供浏览器内 Vineflower 反编译；未缓存 → 404 |
| POST | `/seed/analyze` | 种子分析 |
| POST | `/mc/structures` `/mc/structures/types` `/mc/capabilities` `/mc/biome` `/mc/strongholds` `/mc/slime-chunks` `/mc/seeds/{generate,find,parse}` | 种子与结构工具 |
| POST | `/mc/region/inspect` `/mc/region/chunk` | 区域文件解析（区块清单 + 统计 + 只读诊断） |
| POST | `/mc/level/parse` | 世界存档 `level.dat` 解析（世界信息 + 只读告警） |
| POST | `/stats/event` | 匿名使用统计上报（`{scope,id,action}`，成功 `204`，非法 / 未知对象 `422`） |
| GET | `/robots.txt` `/sitemap.xml` | SEO（根路径） |

### 6.2 管理 API（需登录 + CSRF）

鉴权已由硬编码角色改为**权限点**（`RequirePermission(roleSvc, <权限点>)`）；下表「权限」列为所需权限点（内置 `admin` 持有全部权限点，`editor` 持有内容管理组）。

| 方法 | 路径 | 权限 |
|---|---|---|
| POST | `/admin/auth/login` | 公开（限流） |
| POST | `/admin/auth/logout` `/me` | 登录即可（返回 `permissions`） |
| POST | `/admin/auth/change-password` | 登录 + CSRF |
| POST | `/admin/admins/{list,create,update,reset-password,delete}` | `accounts.admins` |
| POST | `/admin/roles/{list,permissions,create,update,delete}` | `roles.manage` |
| POST | `/admin/users/list` | `accounts.users` |
| POST | `/admin/resources/{list,create,detail,update,delete,check-link}` | `content.resources` |
| POST | `/admin/resources/releases/{list,replace}` | `content.resources` |
| POST | `/admin/tools/{list,create,detail,update,delete}` | `content.tools` |
| POST | `/admin/wiki/{list,create,detail,update,delete}` | `content.wiki` |
| POST | `/admin/categories/{list,create,update,delete}` | `content.categories` |
| POST | `/admin/tags/{list,create,update,delete}` | `content.tags` |
| POST | `/admin/feedback/{list,update,reviews}` | `content.feedback` |
| POST | `/admin/redstone/designs/{list,review}` | `review.redstone` |
| POST | `/admin/mods/{list,review}`、`/admin/modpacks/{list,review}` | `review.mods` |
| POST | `/admin/audit-logs/{list}` | `ops.audit` |
| POST | `/admin/data-quality/overview` | `ops.dataquality` |
| POST | `/admin/data-import-jobs/{list}` | `ops.dataquality` |
| POST | `/admin/search-terms/list` | `ops.searchstats` |
| POST | `/admin/analytics/{overview,tools,resources,api,trend}` | `ops.analytics` |
| POST | `/admin/link-health/{run,list,confirm}`（含 `/{id}/confirm`） | `ops.linkhealth` |
| POST | `/mcsrc/prepare` | `ops.mcsrc`（另叠加限流 + 并发上限 1） |

### 6.3 普通用户 API（`/api/v1/user/*`）

| 方法 | 路径 | 权限 |
|---|---|---|
| POST | `/user/register` | 公开（全局限流） |
| POST | `/user/login` | 公开（`用户名 + IP` 限流） |
| POST | `/user/logout` `/me` | 登录即可 |
| POST | `/user/change-password` `/delete` | 登录 + CSRF |
| POST | `/user/favorites/{list,toggle,remove}` | 登录（写操作 + CSRF） |
| POST | `/user/recents/{list,record,clear}` | 登录（写操作 + CSRF） |
| POST | `/user/seeds/{list,save,delete}` | 登录（写操作 + CSRF） |

> 完整规范见 `backend/openapi/openapi.yaml`。

---

## 七、数据模型与运维工具

### 7.1 数据表（`backend/migrations`）

`accounts`、`roles`、`categories`、`tags`、`audit_logs`、`tools`、`resources`、`resource_releases`、`wiki_entries`、`items`、`recipes`、`potions`、`brewing_recipes`、`villager_professions`、`villager_trade_levels`、`villager_trades`、`mobs`、`mob_drops`、`redstone_components`、`redstone_designs`、`mods`、`mod_versions`、`mod_dependencies`、`modpacks`、`modpack_mods`、`commands`、`feedback`、`feedback_reviews`、`data_import_jobs`、`search_terms`、`user_favorites`、`user_recents`、`user_seeds`、`tool_stats_daily`、`resource_stats_daily`、`api_stats_daily`、`search_stats_daily`、`dev_docs`、`mapping_sets`、`mapping_classes`、`mapping_members`、`resource_link_checks`。

> MC 源码浏览器（mcsrc）**不落库**：jar 与类索引以文件形式缓存在 `MCSRC_CACHE_DIR`（默认 `<MCMETA_CACHE_DIR>/mcsrc`），因为 jar 体积大且属于可重建的派生物，不入数据库与版本控制。

- 主键统一 UUID；时间字段 `timestamptz`。
- `tools` / `resources` / `wiki_entries` / `items` 具备 `search_vector`（生成列 + GIN 索引）以支持全文加权检索。
- 版本化资料字段：`items.supported_versions` / `items.last_verified_at`；`recipes.supported_versions`（适用版本集合，区别于 `game_version` 目标版本）/ `recipes.last_verified_at`；`commands.source_url` / `commands.last_verified_at`。
- `resources.dependencies` 为规范对象数组 `{name, type, resource_url?, required, note?}`；`resources.compatibility_note` 为人工核验备注。
- `resource_releases` 记录同一资源的不同发布版本：`resource_id`（外键，级联删除）、`version_label`（资源内唯一）、`game_versions`、`loader_types`、`source_url`、`released_at`、`notes`、`is_latest`（部分唯一索引保证每个资源至多一条）。
- `data_import_jobs` 记录每次导入的来源、数量与错误摘要（`error_summary` 为 JSONB，最多保留 50 条 + 省略说明）。
- `search_terms` 记录**匿名化聚合**搜索词统计（`term` 唯一且 ≤64 字符、`hits`、`no_result_hits`、`failed_hits`、`last_result_count`、首末出现时间），不含任何用户标识。
- 使用统计为**按天聚合**的四张表（`tool_stats_daily` / `resource_stats_daily` / `api_stats_daily` / `search_stats_daily`）：维度键 + `day`（按服务器本地日期归组、落库为当日 UTC 零点）唯一，写入为批量 UPSERT **累加**；`api_stats_daily` 额外含延迟分桶计数字段（`bucket_50/200/500/1000/3000/over`）与 `max_ms`，用于近似 P95；均不含逐次明细、IP、会话或用户标识，按 `STATS_RETENTION_DAYS` 定期清理。
- 迁移序号：`0001_basics` → `0002_content` → `0003_admin_contract`（枚举对齐前端）→ `0004_search_vector` → `0005_item_catalog`（统一物品资料库：类别 / 耐久度 / 获取方式 / 用途 / 图片 / 最后校验时间）→ `0006_versioning_quality`（版本化资料规则 + 数据导入任务）→ `0007_search_terms`（匿名搜索词统计）→ `0008_search_failures`（搜索失败计数）→ `0009_resource_releases`（资源发布版本 + 兼容性备注）→ `0010_user_accounts`（普通用户账号 / 收藏 / 最近使用 / 常用种子）→ `0011_usage_stats`（使用统计四张日聚合表，含延迟分桶）→ `0012_potions`（药水与酿造：`potions` 效果变体 JSONB + `brewing_recipes` 步骤）→ `0013_dev_docs`（开发栏目：`dev_docs` 文档知识库（正文 + 结构化 `api_entries`）+ `mapping_sets` / `mapping_classes` / `mapping_members` 映射数据集两级结构，三列名字 `obf` / `mid` / `named`）→ `0014_villagers`（村民交易：`villager_professions` 职业 + `villager_trade_levels` 等级通用规则 + `villager_trades` 交易表（成本 JSONB 区间 + 结果 + 分组 CHECK 约束 + 索引））→ `0015_link_health`（链接健康：`resource_link_checks` 每资源最新一次检查结果 + 人工复核状态（pending / confirmed_broken / confirmed_ok / ignored）+ 审核人 / 时间 / 备注，含状态与复核 CHECK 约束）→ `0016_mobs`（生物资料：`mobs` 生物本体（类别 CHECK 约束 + 维度 / 版本 JSONB + 来源与核验时间）+ `mob_drops` 掉落（数量区间与概率 CHECK 约束 + 条件 / 说明 + 索引））→ `0017_feedback_submissions`（用户投稿与纠错：`feedback` 扩展投稿类型 / 关联资料 / 审核留痕 / 授权条款 / 反垃圾摘要 + `feedback_reviews` 审核变更历史表）→ `0018_redstone`（红石与机器设计：`redstone_components` 元件本体（类别 CHECK 约束 + 别名 / 版本 JSONB + 来源与核验时间）+ `redstone_designs` 设计（用途 / 难度 / 审核状态 CHECK 约束 + 材料 JSONB 区间 + 作者 / 授权 / 存档下载 + 审核留痕），含检索与审核索引）→ `0019_mods`（Mod 与整合包：`mods` 父条目（类别 / 加载器 / 版本 JSONB + 来源 / 许可 / 核验时间 + 审核留痕）+ `mod_versions` 发布版本（发布类型 CHECK + 下载入口 / 授权说明 + 验证状态）+ `mod_dependencies` 依赖与冲突（依赖类型 CHECK + 目标引用或回退名 + 来源 / 验证）+ `modpacks` 整合包父条目 + `modpack_mods` 包内清单项，含检索与审核索引）→ `0020_dev_docs_search_vector`（开发文档接入全站搜索：`dev_docs.search_vector` STORED 生成列（标题 A / 摘要 B / 正文 C）+ GIN 索引）→ `0021_roles`（**角色与权限底座**：`roles` 表（`name` 主键 + `permissions` JSONB + `is_system`）+ 内置 `admin` / `editor` / `user` 种子 + `admins.role` 由 CHECK 改为**外键引用 `roles.name`**，使自定义角色可立即分配；含反向 SQL）→ `0022_accounts`（**统一账号体系**：`admins` / `users` 合并为 `accounts`（`role` 外键引用 `roles.name`、默认 `'user'`），8 个外键（审计 / 内容创建人 / 个人数据）重指 `accounts`，删除旧表；含反向 SQL）。

### 7.2 命令行工具（`backend/cmd`）

| 命令 | 用途 |
|---|---|
| `cmd/api` | 启动 API 服务 |
| `cmd/migrate` | 执行数据库迁移 |
| `cmd/seed` | 导入演示数据（按业务主键幂等更新；`--type items\|recipes\|potions\|villagers\|mobs\|redstone\|mods\|devdocs\|commands\|tools\|resources\|all`，`--type all` 以 `--file` 为目录并按依赖顺序统一导入；支持 `--dry-run`）。物品 / 配方 / 指令 / 工具 / 资源做**枚举、数值、时间格式、版本格式与范围、依赖与发布版本结构**校验，失败逐行汇总；药水额外做**来源 URL 与适用版本必填、类别 / 版本 / 步骤类型枚举、悬空药水引用**校验（悬空引用致命失败）；村民额外做**来源 URL 与适用版本必填、职业类别 / 交易分组枚举、分组与等级一致、成本数量区间（min ≤ max 且 ≥ 1）、悬空职业引用**校验（悬空引用致命失败）；生物额外做**来源 URL 与适用版本必填、类别 / 维度枚举、血量非负、掉落数量区间（min ≥ 0 且 max ≥ min）、概率 ∈ [0,1]、悬空生物引用**校验（悬空引用致命失败）；红石额外做**来源 URL 与适用版本必填、元件类别 / 设计用途 / 难度 / 审核状态枚举、材料数量区间（min ≥ 0 且 max ≥ min）、空 `item_id` 必须有中文名、未授权存档（有下载链接必须有 `license`）**校验（失败逐行汇总，单条失败不影响其余条目）；Mod 额外做**来源 URL 与适用版本必填、加载器 / 类别 / 发布类型 / 依赖类型 / 审核状态枚举、下载入口必须有授权说明（`license` 或 `license_note`）、悬空依赖与清单引用**校验（两阶段导入：先落全部 Mod 再写依赖，悬空引用致命失败，单条失败逐行汇总）；开发文档额外做**平台 / 主题 / 接口类型 / 状态枚举、来源 URL 必填、适用版本格式、正文非空、接口清单结构**校验（失败逐条汇总，单条失败不影响其余条目）；每次执行写入 `data_import_jobs`（新增 / 更新数量为准确值）。资源导入复用服务层规范化函数，`releases` 字段缺省表示「不触碰已有发布版本」，显式空数组表示「清空」 |
| `cmd/import-mappings` | 导入映射数据集（TODO §21）：支持 **ProGuard（Mojang 官方映射）/ TSRG·TSRG2（MCPConfig）/ Tiny v1·v2（Fabric Yarn）**，自动识别格式与 gzip 解压；按 `set_key` **单事务整体替换**（先 upsert 数据集、再删除该数据集旧类、批量写入类与成员并回填计数）；`--file` / `--format` / `--mc-version` / `--mapping-type` / `--channel` / `--build-label` / `--set-key` / `--source-url` / `--license` / `--note` / `--limit` / `--dry-run`；**仓库内不含任何真实映射文件** |
| `cmd/create-admin` | 初始化管理员（密码 ≥12 位，无默认密码） |
| `cmd/wikicrawl` | 抓取中文 Minecraft Wiki 公开条目为 Markdown |

### 7.3 数据抓取（wikicrawl）

- 遵守 robots.txt 与限速（默认间隔 1s，失败线性退避）；仅抓 `/w/` 普通条目，不使用被禁止的 `api.php` / `rest_v1`。
- 自动处理重定向并以真实标题生成 slug；正文转为 Markdown 并过滤噪音。
- 内容授权 CC BY-NC-SA 3.0，保留来源 URL 与许可信息。

### 7.4 部署与运维

- Docker Compose 编排：PostgreSQL + 后端 + 前端 + Caddy（自动 HTTPS）。
- 脚本：`scripts/dev.sh`（一键开发环境）、`scripts/backup.sh` / `scripts/restore.sh`（备份恢复）。
- 配置项见 `.env.example`（数据库、会话、CORS、限流、缓存、链接检查、使用统计等；统计相关为 `STATS_ENABLED` / `STATS_FLUSH_SECONDS` / `STATS_RETENTION_DAYS`）。

---

## 八、质量与测试

| 层面 | 手段 |
|---|---|
| 后端 | `go vet ./...`、`go test ./...`（单元 + 集成） |
| 集成测试 | 提供 `TEST_DATABASE_URL` 时启用，覆盖认证、权限、CSRF、资源生命周期、SSRF、反馈限流、错误不泄漏、账号管理、改密会话失效、**物品详情与关联配方、物品检索、搜索 URL、搜索排序与分页、搜索长度防护、搜索词匿名统计与无结果建议、搜索失败统计、配方关联物品解析（含未收录物品断链防护）、数据质量概览与导入任务、资源发布版本整体替换与幂等、兼容性结论（已验证 / 未验证）、资源元数据非法输入 422 拦截、版本通配（`all` 归一化 / 独占校验 / 列表与搜索筛选语义）**（含 editor 403）等 |
| 版本规则 | `internal/versioning` 表驱动单测：单版本 / 快照 / 预发布解析、排序语义、**范围声明（`1.16-1.17` / `1.18+`）与不合理范围拒绝**、重复检测、**「所有版本」通配（`all` / `*` 归一化、独占约束、排序最前）**、**「自某版本起」精确匹配 `Satisfies`（选 `1.20` 命中 `1.8+`、不命中 `1.21+`；哨兵值不匹配任何版本）** |
| 药水与酿造 | `cmd/seed`：随仓库发布的 `potions_demo.json` 逐行校验（来源 / 版本必填、枚举合法、悬空药水引用致命）与幂等 upsert；`internal/service/catalog`：关键词（slug / 中英文名 / 别名）匹配、类别与**精确版本**过滤、分页边界、通用规则过滤；前端 `tests/potions.spec.ts`：**罗马数字 / 时长格式化、列表页 URL 还原与回写、通用规则原料链接「已收录才链接」、详情页三变体与步骤链渲染、未收录原料不生成死链、未收录药水空态** |
| 开发栏目与映射 | `internal/mappings` 表驱动单测（合成夹具，`parsers_test.go`）：**ProGuard / TSRG·TSRG2 / Tiny v1·v2 解析、gzip 解压、命名空间按名称关键词归类与下标回退**；`internal/service/catalog`：开发文档关键词 / 平台 / 主题 / **精确版本**过滤与分页、接口条数统计、相关条目排除自身、映射按类聚合（类名命中优先）与成员预览截断标记、类详情成员分页；前端 `tests/dev-docs.spec.ts`：**平台 / 主题 / 接口类型标签回退、`all` → 「全版本」展示、映射名字回退 named > mid > obf、列表页 URL 还原与回写、详情页接口速查 / 正文 / 来源与校验 / 相关条目、未收录条目错误态、未导入数据集时展示导入指引（不发查询）、已导入时按类聚合与展开成员、命中截断提示**。⚠️ 本批（§21）**仅完成代码，尚未执行 `go vet` / `go test` / `vue-tsc` / `vitest` / 迁移与种子导入** |
| 搜索与关联 | `internal/search` 表驱动单测：**相关度权重梯度、相关度排序、分页切片**；`internal/service/searchstats` 归一化与 LIKE 转义；`internal/service/catalog` 配方↔物品引用汇总与脏数据容错 |
| 资源元数据 | `internal/service/resource` 表驱动单测：**加载器 / 版本 / 游戏版本类型规范化、依赖结构校验（含前置与冲突）、发布版本校验与排序、兼容性结论（未验证原则、来源标注、空切片契约）**；`cmd/seed` 资源行校验与 releases 指针语义 |
| 指令生成 | `utils/command-generator` 表驱动单测：**版本裁决（NBT / 数据组件、`/effect` give 分界、快照 assumed）、命名空间 ID 与目标 / 坐标校验、六类指令生成结果、错误定位** |
| 附魔计算 | `utils/enchant-calculator` 表驱动单测：**先作惩罚 2ⁿ−1 与合并公式、适用性、组合冲突与 1.12.2 版本语义、结果附魔推演、铁砧花费公式（含 Wiki 公开示例 16 / 20 复算）、过于昂贵判定** |
| 世界分析 | `internal/mcstructure`：**已知种子回归基线（结构坐标 + 史莱姆区块）、埋藏的宝藏零偏移、维度归属、频率门槛、描述表自洽、版本可用性与能力矩阵**；`handler`：版本提示文案与群系查询版本门控；前端 `utils/coordinate`：传送门配对与线路方位 |
| 存档诊断 | `internal/region`：区域文件解析与**只读诊断**（偏移/长度越界、扇区不足与重叠、未知压缩、时间戳异常）；`internal/nbt`：NBT 解析、**gzip 解压上限**与 `ParseFile`（压缩/未压缩兼容）；`handler`：`level.dat` 基础信息与告警、区域检查端到端 multipart；前端 `utils/region-file`：文件类型判定与校验 |
| 用户账号与个人数据 | `internal/service/user`：**注册校验 / 登录限流与锁定 / 收藏 toggle / 最近使用裁剪到 50 / 种子 CRUD / 删号级联**（内存仓储假实现）；`handler`：用户路由**未登录 401、缺 CSRF 403、管理会话访问用户接口 401**；前端 `tests/user-account.spec.ts`：登录态恢复幂等、工作台增删改与最近使用服务端同步 |
| 使用统计与运营看板 | `internal/service/metrics`：**内存聚合与批量 UPSERT 累加、分桶近似 P95（含溢出桶）、`PruneBefore` 保留期清理**（假仓储）；`middleware`：路由归一（chi 模板参数如 `/tools/{slug}` 折叠为模板，未匹配归并 `<unmatched>`）；`handler`：`/stats/event` 白名单校验与 **422**、看板时间范围解析（`days` 上限 366 / 显式 `from`-`to` 优先）；前端 `tests/tool-stats.spec.ts`：路径→slug 反查、测试环境静默、`visit`/`use`/`fail` 上报与 `CopyButton` 复制产出计数；`tests/analytics-chart.spec.ts`：**真实 ECharts（SSR + SVG）渲染图表配置**（node 环境，规避 jsdom 无 canvas） |
| 前端 | `vue-tsc -b --noEmit` 类型检查；`vitest` 单元测试；`vite build` 构建校验 |
| 角色与权限底座 | `cmd/api` 集成测试 `TestRoleManagement`：editor 调用 `/admin/roles/list` **403**、内置角色与全量权限点、权限点目录、新建自定义角色、**非法权限点 422**、**删内置角色 422**、**admin 削权 422**、**自定义角色账号权限真实生效**、**被引用角色删除 409**、删号后可删 204；前端 `tests/admin-roles.spec.ts`：列表渲染、权限点分组勾选、内置角色保护、删除确认 |
| MC 源码浏览器 | `internal/mcsrc`：合成 jar 的索引构建（过滤 `META-INF` / `module-info`、内部类标记、包名拆分、按全限定名排序、**超上限截断**）、检索**相关度排序与分页边界**（简单名相等 > 前缀 > 包含 > 全限定名包含，非法分页参数回落默认值）、**版本号白名单拒绝路径穿越**、Prepare 全流程（sha1 校验、体积上限、临时文件原子改名、**单飞：5 并发只下载一次**、校验失败不留半成品、上下文取消）、**状态 JSON 省略零值时间**；**切片 2**：`classfile.go` 手写 class 文件解析（常量池全 tag 含 Long / Double 双槽位、修饰符 / 父类 / 接口 / 字段 / 方法，属性按长度跳过；**任意截断 / 伪造 tag / 越界属性长度均报错不 panic**）、`descriptor.go` 描述符渲染（基础类型 / 数组 / 泛型内部类名、`void` 仅可作返回类型、方法签名拆分）、`structure.go` 结构视图与继承关系（点分 / 斜杠类名等价、`in_jar_super` 只查 zip 中央目录、**关系索引按 jar 摘要缓存 + 变更失效 + 单飞**、**子类上限截断仍报准确计数**、损坏 class 条目跳过不中断）；真实上游用例 `TestLivePrepareRealJar`（`MCSRC_LIVE_TEST=1` 时启用，实测 1.8.9 客户端 jar：sha1 校验通过、2507 个类、检索到 `net.minecraft.server.MinecraftServer`、二次准备不重复下载、`MinecraftServer` 解析出 53 字段 / 129 方法 / class 版本 50、继承关系实测 `aye extends axu extends avp extends java.lang.Object`、**反向验证父类子类列表包含该子类**、jar 外类返回 `CLASS_NOT_FOUND`）；`handler`：状态 / 校验 422、未建索引 409 `NEED_PREPARE`、准备 → 检索全链路、未知版本 404、缺下载 404、未配置 503、**类详情 / 继承关系 200 + 缺参 422 + 类不存在 404 `CLASS_NOT_FOUND`**；集成测试 `TestMcsrcRoutesAuthGating`：公开只读 200、非法版本 422、匿名准备 401、editor 准备 403、**新增两个只读接口未准备版本 404 / 缺 class 422**；前端 `tests/mcsrc.spec.ts`：管理员可准备并刷新类列表、未登录不显示准备按钮、类表格与内部类标记、`NEED_PREPARE` 空态、状态查询失败提示、**点击类名打开详情（结构 + 继承关系 + 合成方法默认折叠）、勾选显示合成方法、点击祖先下钻、继承关系失败不阻塞结构展示、结构失败可返回列表**；**切片 3-D1**：`handler` 单测新增 `Jar`（未准备 404 / 缺 version 422 / 非法版本 422 / 200 且字节与缓存 sha1 一致 + `Content-Type` + `ETag` / Range 206）、集成测试新增 `GET /mcsrc/jar` 未缓存 404 与缺 version 422；前端 `tests/mcsrc-source.spec.ts`（5 例：列表与详情入口触发反编译并渲染源码、反编译中加载态、失败态可返回列表、源码面板替换详情面板、切换版本关闭面板）与 `tests/mcsrc-decompiler.spec.ts`（5 例：jar 只下载一次、按侧别分别缓存、失败不缓存可重试、并发只去重下载一次、缓存键与类名格式转换）；**切片 3-D2**：前端 `tests/mcsrc-tokens.spec.ts`（6 例：偏移命中与越界、行列 ⇄ 偏移换算、成员跳转片段编解码、声明定位含重载区分）+ `tests/mcsrc-navigation.spec.ts`（6 例：类 token 直通不查索引、成员在引用点所属类命中、沿祖先链定位、字段按名匹配、全部未命中回退、索引缺失不抛异常且成员集合命中缓存）+ `tests/mcsrc-decompiler.spec.ts` 扩展（结果缓存命中、`clearJarCache` 同时清空两类缓存、类名探测序列对齐 `Outer$Inner`）+ `tests/mcsrc-source.spec.ts` 扩展（token 透传与跳转提示、类 token 打开目标类、方法引用沿继承链定位到父类声明、同类内跳转不重复反编译） |
| 分享与易用性 | `tests/share.spec.ts`：**机密参数识别（token / csrf / session / cookie / password / authorization / api_key / signature…）、默认值省略、`buildShareUrl` 丢弃 path 自带查询与 hash、`sanitizeShareUrl` 清理查询与 hash 片段、路由参数类型回退**；`tests/seed-tools-query.spec.ts`：**默认状态 ⇔ 无参数、全子表单往返、非法版本与非法结构类型回退、0 值不当空值**；`tests/share-button.spec.ts`：**复制内容不含机密参数、失败提示、`aria-label` 同步**；`tests/share-url-sync.spec.ts`：**四类工具的 URL 还原 / 变化后回写 / 多字段互不覆盖 / 种子链接打开即分析 / 默认状态链接干净**；`tests/command-generator.spec.ts`：**数字输入（number 型 count / damage / duration / 坐标）回归** |
| 安全回归 | 测试覆盖 CSRF、越权 403、SSRF、限流、错误信息不外泄、**分享链接秘密参数剥离** |

---

## 九、功能状态一览

| 模块 | 状态 |
|---|---|
| 公开前台（首页/搜索/工具/资源/百科/反馈/SEO） | ✅ 已实现 |
| 内置前端工具（19 项，§1–§22 已交付） | ✅ 已实现 |
| 开发栏目内置工具（开发文档 `/dev` / 版本映射查询 `/mappings` / MC 源码浏览器 `/mcsrc`，3 项） | ✅ 已实现（TODO §21 / §22） |
| 药水与酿造资料（效果与时长变体 / 酿造步骤链 / 通用规则 / 精确版本筛选 / 原料与百科关联） | ✅ 已实现（TODO §9-A） |
| 我的世界开发知识库（NeoForge / Forge / Fabric / 原版数据，40 条 + 189 条关键接口） | ✅ 已实现（TODO §21；`cmd/seed -type devdocs` 已导入 40 条） |
| 版本映射查询（ProGuard / TSRG·TSRG2 / Tiny v1·v2 导入 + 三列名字对照检索） | ✅ 已实现（TODO §21；**不预置映射数据**，需使用者按官方来源导入，未导入时页面给出可复制的导入指引） |
| 村民交易 | `cmd/seed`：随仓库发布的 `villagers_demo.json` 逐行校验（来源 / 版本必填、职业类别与交易分组枚举、分组与等级一致、成本数量区间、悬空职业引用致命）与幂等 upsert；前端 `tests/villagers.spec.ts`：**数量区间格式化、列表页 URL 还原与回写、职业类别筛选、等级规则来自后端、工作站/成本/结果物品链接「已收录才链接」、未收录物品不生成死链、详情页分组交易表渲染** |
| 村民交易资料（职业 / 交易等级 / 交易表） | ✅ 已实现（TODO §9-B） |
| 生物资料 | `cmd/seed`：随仓库发布的 `mobs_demo.json` 逐行校验（来源 / 版本必填、类别与维度枚举、血量非负、掉落数量区间与概率、悬空生物引用致命）与幂等 upsert；`internal/service/catalog` 版本过滤复用 `versioning.Satisfies`；前端 `tests/mobs.spec.ts`：**生命值 / 数量区间 / 概率格式化、列表页 URL 还原与回写、生物类别筛选、掉落物链接「已收录才链接」、未收录掉落物不生成死链、详情页掉落表与生成/驯服说明渲染** |
| 生物资料（掉落 / 生成条件 / 驯服与繁殖） | ✅ 已实现（TODO §9-C） |
| 红石资料 | `cmd/seed`：随仓库发布的 `redstone_demo.json` 逐行校验（来源 / 版本必填、类别 / 用途 / 难度 / 审核状态枚举、材料数量区间 min ≤ max 且 ≥ 0、空 `item_id` 必须有中文名、未授权存档不得带下载链接）与幂等 upsert；`internal/service/catalog` 版本过滤复用 `versioning.Satisfies`、公开接口只返回 `published` 设计；前端 `tests/redstone.spec.ts`：**材料数量区间 / 材料文案 / 外链判定、列表页双 tab URL 还原与回写、类别 / 用途 / 难度筛选、详情页元件机制与信号说明渲染、设计材料「已收录才链接」与未收录不生成死链、未授权存档不展示下载链接** |
| 红石与机器设计资料库（元件机制 / 机器设计 / 材料成本 / 授权存档 / 内容审核） | ✅ 已实现（TODO §15） |
| Mod 兼容性 | `cmd/seed`：随仓库发布的 `mods_demo.json` 两阶段导入（先落全部 Mod 再写依赖）并逐行校验（来源 / 版本必填、加载器 / 类别 / 发布类型 / 依赖类型 / 审核状态枚举、下载入口必须有授权说明、悬空依赖与清单引用致命）与幂等 upsert；`internal/service/catalog` 版本过滤复用 `versioning.Satisfies`、整合包目标版本精确匹配、公开接口只返回 `published`；前端 `tests/mods.spec.ts`：**授权下载入口判定、整合包清单展示与链接、依赖目标展示、列表页双 tab URL 还原与回写、加载器 / 类别 / 版本筛选、详情页发布版本表与依赖分组 + 验证徽章渲染、无授权说明不展示下载链接、整合包清单「已收录才链接」与未收录不生成死链** |
| Mod 与整合包兼容性（版本 / 加载器 / 依赖与冲突 / 整合包清单 / 来源与验证 / 内容审核） | ✅ 已实现（TODO §16） |
| MC 种子/结构/区块计算能力 | ✅ 已实现 |
| 管理后台认证与账号设置 | ✅ 已实现 |
| 内容管理（资源/工具/百科/分类/标签） | ✅ 已实现 |
| 反馈管理 | ✅ 已实现 |
| 用户投稿与纠错（资料纠错 / 资源补充 / 待审核 / 审核留痕 / 反垃圾 / 授权条款） | ✅ 已实现（TODO §17） |
| 审计日志（admin） | ✅ 已实现 |
| 管理员账号管理（`accounts.admins`） | ✅ 已实现 |
| 角色与权限底座（`roles` 表 + 17 个权限点 / 4 组 + `RequirePermission` 替换硬编码角色） | ✅ 已实现（本批） |
| 角色管理页（`/admin/roles`：新建 / 编辑 / 权限点分组勾选 / 内置角色保护 / 引用中不可删） | ✅ 已实现（本批） |
| 普通用户管理（`/admin/users`：列表 + 用户名搜索 + 状态筛选 + 分页 + 个人数据概览） | ✅ 已实现（本批，只读） |
| MC 源码浏览器（`/mcsrc` 切片 1：版本选择 + jar 获取与缓存 + 类索引 / 检索） | ✅ 已实现 |
| MC 源码浏览器（`/mcsrc` 切片 2：类结构视图 + 继承关系，纯 Go 解析 class 字节码） | ✅ 已实现（本批） |
| MC 源码浏览器（`/mcsrc` 切片 3-D1：源码浏览——`GET /mcsrc/jar` 下发缓存 jar + 浏览器内 Vineflower WASM 反编译 + Monaco 展示） | ✅ 已实现（**无需部署侧 JVM**） |
| MC 源码浏览器（`/mcsrc` 切片 3-D2：token 着色 + 点击跳转——Ctrl/⌘ 点击 / F12 跳转定义，字段与方法沿继承链定位声明类） | ✅ 已实现（本批） |
| MC 源码浏览器切片 3 余项（查找引用 / 重命名映射展示 / 字节码指令视图 / Javadoc 备注） | ⏳ 待办（见 TODO §22） |
| 统一账号体系（合并 `admins` / `users` 为 `accounts` + 8 个外键重指 + 删旧表 + 单一会话 Cookie） | ✅ 已实现（本批，迁移 0022） |
| 改密/停用/删除会话失效 | ✅ 已实现 |
| 全文检索 | ✅ 已实现（英文加权 + 中文 ILIKE 兜底） |
| 统一物品资料库（详情页 + 关联配方 + 导入校验） | ✅ 已实现 |
| 版本化资料规则（单版本 + 范围校验与归一化） | ✅ 已实现 |
| 版本覆盖（「所有版本」通配 / 版本选项覆盖全部 MC 版本 / 详情展开完整清单） | ✅ 已实现 |
| 数据质量监控与导入任务记录（admin） | ✅ 已实现 |
| 跨模块资料关联（配方↔物品直链 + 有效性检查 + 检索入口） | ✅ 已实现 |
| 搜索体验（相关度权重 / 排序 / 版本筛选 / 无结果建议 / 输入防护） | ✅ 已实现 |
| 搜索词匿名统计（热门 / 无结果 / 失败，admin） | ✅ 已实现 |
| 资源兼容性与来源信息（统一字段校验 / 依赖与冲突 / 发布版本关联 / 未验证原则 / 来源与许可展示 / 元数据批量导入） | ✅ 已实现 |
| 资源链接检查强化（超时 / 重定向次数 / 并发限制 / 批量复查） | ✅ 已实现（TODO §6） |
| 失效链接后台复查与人工确认（确认失效 / 可用 / 暂不处理 + 审核记录） | ✅ 已实现（TODO §6） |
| 版本不支持提示与统一版本交互（版本特性注册表 / 统一提示组件 / 统一版本清单） | ✅ 已实现（TODO §2） |
| 指令生成器（表单化 / 版本化语法 / 校验 / 复制） | ✅ 已实现 |
| 附魔与装备计算器（组合校验 / 铁砧花费 / 方案对比） | ✅ 已实现 |
| 世界分析工具扩展（版本支持矩阵与提示 / 群系查询 / 更多结构与维度频率 / 路线与传送门提示 / 超时限流 / 回归基线） | ✅ 已实现 |
| 存档分析与诊断（level.dat / 区域文件诊断报告） | ✅ 已实现 |
| 收藏与个人工作台（可选账号 / 收藏 / 最近使用同步 / 常用种子 / 账号数据删除 / 隐私与保留说明） | ✅ 已实现 |
| 使用统计与运营看板（工具访问/使用/失败、资源访问与失效链接、搜索量与无结果、API 延迟与限流、时间范围筛选、保留期限） | ✅ 已实现 |
| 分享与稳定链接（14 个工具状态同步 URL / 打开即复现 / 参数纠错 / 秘密参数剥离 / 分享按钮） | ✅ 已实现（14 项） |
| 界面状态与可访问性规范（统一加载·空·错误态语义 / 跳转链接 / 表格 `scope` / 长文本换行 / 数字输入类型归一） | ✅ 已实现 |
| 可视化指令生成器 / 附魔计算器 / 存档诊断的 URL 分享 | ⏳ 未做（表单配置类与本地文件类，见 §2.17；以「复制产出」为分享方式） |
| 移动端真机走查（§14 已做代码级检查与修复，未在真机/模拟器逐页验证） | ⏳ 待办 |

---

*本文件由系统实现核对生成，描述当前已交付能力；后续变更请同步更新。*
