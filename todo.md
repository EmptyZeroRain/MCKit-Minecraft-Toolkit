# MC 工具站 · 功能规划待办清单（TODO.md）

> 目标：在现有 MC 工具站基础上，优先完善数据质量、跨模块关联和高频实用工具，再逐步扩展资源生态。
>
> 当前技术栈：Go + Chi + GORM + PostgreSQL 16；Vue 3 + TypeScript + Vite。
> 说明：本清单是后续规划，不代表相关功能已经实现。完成后请同步更新 `Function.md`、OpenAPI 规范、测试和部署文档。

## 状态说明

- [ ] 待办
- [x] 已完成
- 优先级：`P0` 必须优先、`P1` 高价值、`P2` 后续扩展

---

## 一、P0：数据基础与质量

### 1. 统一物品资料库

- [x] 建立统一物品资料模型，覆盖中文名、英文名、命名空间 ID、别名、堆叠上限等基础字段。
- [x] 根据资料类型补充耐久度、获取方式、用途等可选属性。
- [x] 为物品数据记录来源 URL、来源版本、最后校验时间。
- [x] 设计物品数据导入工具，支持重复执行与幂等更新。
- [x] 建立物品 ID、中文名、英文名和别名的统一检索。
- [x] 为物品资料建立独立详情页。
- [x] 将物品详情与配方、指令、百科和相关资源互相关联。
      - 配方：**数据驱动**（result_item_id 产出 + ingredients 包含匹配）。
      - 指令 / 百科 / 资源：因缺少可信元数据，提供「按名称检索」入口而非伪造直链，避免死链与未发布内容。
- [x] 增加物品数据导入与校验测试。

### 2. 版本化资料管理

> 本批已落地：`items.supported_versions` / `recipes.supported_versions` / `commands.game_versions` 统一采用「单版本 + 范围」声明（`internal/versioning`），并写入 `last_verified_at` / `source_url` / `data_version`；跨版本校验（重复、非法格式、起点大于终点）接入导入流程与数据质量概览。**结构**资料无独立版本字段，由 MC 接口的版本参数区分。

- [x] 明确物品、配方、指令、结构等资料的版本适用规则。
- [x] 为需要版本管理的数据记录适用版本范围或独立版本记录。
- [x] 记录原始数据来源、来源版本和最后验证时间。
- [x] 对版本不支持的功能明确提示，不将理论结果描述为确定结果。（`utils/tool-versions.ts` 统一登记特性生效版本，`checkFeatureSupport` 分「支持 / 不支持 / 不确定」，不支持时明确标注「理论值 / 不代表实际行为」；`components/common/VersionNotice.vue` 统一提示，已接入指令生成器与附魔计算器）
- [x] 增加跨版本数据校验，发现重复、缺失和不合理的版本范围。
- [x] 为版本切换设计统一的前端交互。（版本清单统一走 `utils/mc-versions.ts`（`loadReleaseVersions` 内存缓存 + 并发合并 + 离线兜底，`buildVersionOptions` 统一选项构造）；版本提示统一走 `VersionNotice.vue`；列表类工具统一用 `FilterPanel` + 版本选项，工具类统一用版本输入框 + datalist）
- [x] 为关键版本差异补充自动化测试。

### 3. 跨模块资料关联

> 本批已落地关联模型（**只直链有可靠元数据的关系，其余给检索入口**）：
> ① 配方 ↔ 物品：`/recipes` 每项返回 `resolved_items`（结果物品 + 材料），物品详情 `related_recipes.resolved_items` 同源；
> ② 有效性检查：只解析 `items` 表中真实存在的物品，未收录物品**不进入映射**，前端回退为纯文本 ID（无断链）；
> ③ 百科 / 资源 / 指令与物品之间无可靠元数据，改为「按名称检索」入口（`/search?q=…&type=…`、`/commands?q=…`），不伪造直链；
> ④ 资源详情保留版本 / 加载器 / 依赖展示，「相关整合包」以检索入口呈现（`dependencies` 为自由结构，无法可靠反查）。

- [x] 设计物品、配方、指令、百科、资源之间的关联模型。
- [x] 在物品详情页展示相关配方、获取方式和指令。
- [x] 在配方详情或查询结果中关联涉及的物品资料。
- [x] 在百科页面关联相关物品、工具和资源。
- [x] 在资源详情页展示支持的游戏版本、加载器、依赖及相关整合包。
- [x] 为关联数据增加有效性检查，避免链接到已删除或未发布内容。
- [x] 增加关联关系查询与前端展示测试。

### 4. 数据导入与质量监控

> 本批已落地：`cmd/seed --type all` 统一入口（目录级批量导入）、`--dry-run` 预览、幂等 upsert、逐行校验与失败汇总；每次执行写入 `data_import_jobs`（来源 / 时间 / 新增 / 更新 / 跳过 / 失败 / 错误摘要，新增与更新数为**准确值**）；后台 `系统 → 数据质量`（`/admin/data-quality`，admin 专属）提供质量概览与导入任务查询。

- [x] 建立统一的数据导入命令或任务入口。
- [x] 支持预览模式、幂等更新、失败记录和汇总报告。
- [x] 记录导入来源、执行时间、成功数量、失败数量和错误摘要。
- [x] 对必填字段、重复 ID、无效版本和异常引用进行校验。
- [x] 提供后台数据质量概览。
- [x] 为关键数据导入流程增加单元测试和集成测试。
- [x] 为数据源不可用、格式变化和导入失败设计告警或可见提示。

---

## 二、P0：搜索与资源中心改进

### 5. 搜索体验

> 本批已落地：相关度权重梯度（标题 > 命名空间 ID > 别名 > slug > 摘要）与排序（相关度 / 最近更新）、类型与游戏版本筛选；
> 无结果时返回 `meta.suggestions`（基于已记录搜索词的字面关系，非语义猜测）；
> `search_terms` 表记录匿名化聚合统计（关键词 + 搜索次数 / 无结果次数 / **查询失败次数**，不含 IP / 会话 / 用户标识）；
> 后台 `运营 → 搜索词统计`（`/admin/search-terms`，admin 专属）查看热门词、无结果词与失败词；
> 输入防护：关键词上限 80 字符（超长 422）、搜索查询 3 秒超时、异常请求体 400。

- [x] 对无结果搜索展示相似关键词或可尝试的筛选条件。
- [x] 记录匿名化的搜索词、结果数量和查询失败情况，避免记录不必要的个人信息。
- [x] 增加热门搜索和高频无结果词统计。
- [x] 优化中文名称、英文名称、命名空间 ID 和别名的搜索权重。
- [x] 支持搜索结果按类型、游戏版本和相关性筛选。
- [x] 为搜索排序与分页增加回归测试。
- [x] 检查搜索输入长度、查询超时和异常请求的防护。

### 6. 资源兼容性与来源信息

> 本批已落地：`resources` 统一字段（资源类型 / 加载器 / 适用版本 / 游戏版本类型 / 来源平台）在**写入前校验与规范化**，
> 导入器与后台接口共用同一套枚举与版本规则（`internal/service/resource/meta.go`）；
> `dependencies` 规范为 `{name, type, resource_url?, required, note?}`（`type ∈ dependency | prerequisite | conflict`），详情页分组展示；
> 新增 `resource_releases` 表记录**同一资源的不同发布版本**（版本标签 / 适用版本 / 加载器 / 发布页 / 发布时间 / 备注 / 最新标记），后台以完整列表整体替换（幂等）；
> **兼容性结论不落库**，由资源级字段与发布版本实时推导，两者均无数据时 `verified=false` 并明确展示「未验证」；
> 详情页集中展示作者 / 来源平台 / 许可 / 原始来源链接与外链检查状态（含最后检查时间，未检查时显示「未检查」）；
> `cmd/seed --type resources` 支持资源元数据批量导入与幂等更新（`releases` 缺省表示不触碰，显式空数组表示清空）。
> **版本覆盖（§6-C）已落地**：适用版本新增**「所有版本」通配**（`all`，等价 `*`，必须独占），兼容性结论返回 `all_versions`，详情页展示「全部 MC 版本」并可展开官方正式版完整清单；
> 资源 / 搜索 / 配方的**版本选项不再硬编码**，改为动态取自官方版本清单（`POST /versions`，服务端带缓存，失败回退离线兜底清单）。
> **未完成**：链接检查的并发限制与批量任务、失效链接的后台复查与人工确认流程（下一批 §6-B）。

- [x] 统一资源的游戏版本、加载器、资源类型和来源平台字段。
- [x] 展示资源最后检查时间和外链检查结果。
- [x] 建立同一资源不同发布版本之间的关联。
- [x] 记录资源依赖、前置 Mod 和已知冲突信息。
- [x] 在资源详情页清晰展示来源链接、许可信息和作者信息。
- [x] 设计资源元数据批量导入与更新机制。
- [x] 仅在有可靠数据依据时展示兼容性结论；无法确认时显示“未验证”。
- [x] 支持「所有版本」通配声明（`all` / `*`，必须独占），并在详情页展开完整正式版清单。
- [x] 资源、搜索与配方的版本选项覆盖全部 MC 版本（动态取自官方清单，失败回退兜底）。
- [x] 为资源链接检查增加超时、重定向次数和任务并发限制。（`LINK_CHECK_TIMEOUT_SECONDS` / `LINK_CHECK_MAX_REDIRECTS` / 新增 `LINK_CHECK_MAX_CONCURRENCY`（1–16，默认 4）；`linkcheck.CheckMany` 用信号量做受限并发，并保持结果与输入同序）
      - 现状：超时（`LINK_CHECK_TIMEOUT_SECONDS`）与重定向上限（`LINK_CHECK_MAX_REDIRECTS`）已具备；**任务并发限制缺失**。
- [x] 对失效链接提供后台复查与人工确认流程。（迁移 `0015_link_health.sql` 新增 `resource_link_checks`；`POST /admin/link-health/run` 后台批量复查（受限并发 + 限流），`POST /admin/link-health/list` 复核列表（默认只看失败项），`POST /admin/link-health/{id}/confirm` 人工确认「确认失效 / 确认可用 / 暂不处理」并记录审核人、时间与备注；已确认可用 / 已忽略的资源在后续复查中自动跳过；后台新增「链接健康」页）

---

## 三、P1：高频实用工具

### 7. 可视化指令生成器

> 本批已落地：新增内置工具 `/command-generator`（`utils/command-generator.ts` 纯函数 + 视图），支持 `/give`、`/summon`、`/effect`、`/setblock`、`/tp`、`/gamemode` 六类表单化配置；按 Java 版版本自动区分**旧式物品 NBT** 与**新版数据组件**（1.20.5 分界），`/effect` 区分 `give` 子命令（1.19 分界）；快照 / 预览版按最新语法生成并显式标注。校验覆盖缺失参数、非法 ID / 坐标 / 数量、附魔等级上限。一键复制与清空表单齐备；单测 29 例（含各指令生成结果与版本裁决）。

- [x] 支持 `/give`、`/summon`、`/effect` 等常见指令的表单化配置。
- [x] 根据选择的物品、数量、附魔和属性生成指令。
- [x] 根据 Minecraft Java 版版本区分旧式 NBT 与新版数据组件语法。
- [x] 展示参数解释、使用示例和适用版本。
- [x] 对缺失参数、非法值和不支持的版本进行校验。
- [x] 提供一键复制和清空表单功能。
- [x] 为不同指令类型编写生成结果测试。
- [ ] 在移动端验证表单操作与结果展示。
      - 现状：响应式栅格与表单控件已按移动优先编写；**尚未在真机 / 设备模拟下走查**。

### 8. 附魔与装备计算器

> 本批已落地：新增内置工具 `/enchant-calculator`（`utils/enchant-calculator.ts` 纯函数 + 视图）。**附魔资料表**含 41 项附魔的等级上限、适用装备、铁砧合并乘数（物品 / 书）与定性效果；**组合校验**覆盖适用性、等级范围、重复与互斥冲突（互斥按 1.12.2 版本语义：可合并但牺牲附魔丢失）；**铁砧花费**实现 Java 公式（目标惩罚 + 牺牲惩罚 + 重命名 1 + 修复 2 + 附魔花费），并给出「过于昂贵（≥40）」判定；**结果附魔推演**与**先作惩罚（2ⁿ−1）对照**齐备；支持**方案快照对比**。规则依据 Minecraft Wiki《Anvil mechanics》，未覆盖边界（耐久修复量 / 附魔台花费 / Bedrock / 1.8 前机制 / 模组附魔）已在页面与代码显式标注；单测 22 例（含 Wiki 公开示例，附魔花费 16 / 20 复算通过）。

- [x] 建立附魔资料，记录效果、等级上限、适用装备和兼容关系。
- [x] 支持选择装备与多个附魔。
- [x] 校验附魔组合是否合法。
- [x] 查询铁砧操作、经验消耗和惩罚规则。
- [x] 对比不同装备或附魔组合。
- [x] 根据游戏版本展示规则差异。
- [x] 为计算规则增加测试用例，并注明尚未覆盖的边界情况。

### 9. 酿造与养成资料

> **本批已落地（§9-A：药水与酿造）**：
> - 数据层：迁移 `0012_potions.sql` 新增 `potions`（26 条，类别 `water/base/positive/negative/mixed/other`，效果按普通/延长/增强三种变体逐条存储）与 `brewing_recipes`（39 条步骤：基底 15 / 效果 15 / 腐化 5 / 通用修饰 2 / 形态 2）；`cmd/seed -type potions` 逐行校验 + 幂等 upsert + 写入 `data_import_jobs`。
> - 接口：`POST /api/v1/potions`（关键词 / 类别 / 版本 + 分页）、`POST /api/v1/potions/rules`（通用酿造规则，规则存库不硬编码）、`POST /api/v1/potions/{slug}`（药水 + 酿出步骤 + 后续变化 + 关联物品摘要）；OpenAPI 与后台数据质量检查（`potions_*` / `brewing_*`，含悬空引用 error 项）同步补齐。
> - 前端：新增内置工具「药水与酿造」（`/potions` 检索列表 + `/potions/{slug}` 详情），展示效果与时长、酿造步骤链、后续变化、通用规则、来源与核验信息，支持收藏与分享 URL（`q`/`category`/`version`/`page`）。
> - 版本口径：药水的适用版本用「自某版本起」（如 `1.0+` / `1.13+` / `1.21+`）表述，查询侧用新增的 `versioning.Satisfies` 做**精确判断**（筛 `1.20` 命中 `1.8+`、不命中 `1.21+`），比工具/资源的「声明文本匹配」更准。
> - 数据来源：Minecraft Wiki `Brewing` / `Potion` 系列页面（2026-10-17 抓取），每条药水带 `source_url` 与 `last_verified_at`；基岩版差异（再生/剧毒延长时长等）写入说明；未核验项（衰变药水的效果数值）显式标注「未在本站核验」而不填猜测值。
> - 顺带扩充：`items_demo.json` 补录 20 种酿造原料（地狱疣、红石粉、荧石粉、发酵蛛眼、火药、龙息等），使「药水 → 原料物品详情页」的关联链接真实可跳转；收藏类型白名单新增 `potion`。
>
> **本批已落地（§9-B：村民交易）**：
> - 数据层：迁移 `0014_villagers.sql` 新增 `villager_professions`（16 条：13 普通职业 + 流浪商人 + 傻子 + 失业村民）、`villager_trade_levels`（5 级通用规则：新手/学徒/老手/专家/大师，经验门槛 0/10/70/150/250）、`villager_trades`（223 条交易，含 CHECK 约束与索引）；`cmd/seed -type villagers` 逐行校验 + 幂等 upsert + 写入 `data_import_jobs`。
> - 交易模型：`cost_items`（村民想要，JSONB 数组，`count_min/count_max` 表达浮动定价）+ `result`（玩家获得），方向不单列；`trade_group ∈ {level,purchase,special,ordinary}`，`level` 组对应 1–5 级，其余分组 `level=0`。
> - 接口：`POST /api/v1/villagers`（关键词 / 职业类别 / 版本 + 分页）、`POST /api/v1/villagers/levels`（等级通用规则，存库不硬编码）、`POST /api/v1/villagers/{slug}`（职业 + 等级规则 + 按等级/分组聚合的交易 + 关联物品摘要）；OpenAPI 与后台数据质量检查（`villager_*`，含悬空职业引用 error 项、成本/结果物品未收录 warning 项）同步补齐。
> - 前端：新增内置工具「村民交易」（`/villagers` 检索列表 + `/villagers/{slug}` 详情），展示工作站、等级规则、分组交易表（村民想要 → 玩家获得、库存、经验、价格浮动、变体说明），支持收藏与分享 URL（`q`/`kind`/`version`/`page`）。
> - 版本口径：交易系统自 1.14 起默认 `1.14+`；个别交易收紧（试炼密室地图 `1.21+`、蜡烛 `1.17+`、滴水石块 `1.17+`），查询侧沿用 `versioning.Satisfies` 精确判断。
> - 数据来源：Minecraft Wiki `Trading` 页面（2026-10-18 抓取），每条职业/交易带 `source_url` 与 `last_verified_at`；颜色 / 生物群系变体（羊毛、地毯、床、陶瓦、旗帜、船、药箭等）合并为单条交易并在 `variant_note` 说明，不逐一展开；未发布内容（Poplar、Sulfur Spike）不收录。
> - 顺带扩充：`items_demo.json` 补录至 **206 条**，覆盖全部 223 条交易引用的成本/结果物品（绿宝石、煤炭、纸、书、皮革、各类盔甲/工具/食物、16 色羊毛、染料、树苗与花卉等），使「交易 → 物品详情页」的关联链接**全部可跳转**；收藏类型白名单新增 `villager`。
>
> **本批已落地（§9-C：生物资料）**：
> - 数据层：迁移 `0016_mobs.sql` 新增 `mobs`（48 条：被动 20 / 中立 9 / 敌对 16 / Boss 2 / 功能型 1）与 `mob_drops`（70 条掉落，含 `category` / `count` / `chance` CHECK 约束与索引）；`cmd/seed -type mobs` 逐行校验 + 幂等 upsert + 写入 `data_import_jobs`。
> - 模型取舍：血量 `health` 用 Java 版生命值（1 点 = 半颗心，无法确定时为 NULL）；攻击伤害因难度 / 版本差异过大改用文字 `attack_note`，**不写死单一数字**；生成 / 行为 / 驯服 / 繁殖均以文字说明，不虚构数值；掉落数量用 `count_min`/`count_max` 区间 + `chance`（0–1，1 表示必定掉落），`conditions` 记录触发条件（如「用刷子刷取」「被玩家击杀」「幼年成长为成年时」）。
> - 接口：`POST /api/v1/mobs`（关键词 / 类别 / 版本 + 分页）、`POST /api/v1/mobs/{slug}`（生物本体 + 掉落清单 + 关联物品摘要）；OpenAPI 与后台数据质量检查（`mobs_*` / `mob_drops_*`，含悬空生物引用、数量区间非法、概率越界三个 error 项与掉落物未收录 warning 项）同步补齐。
> - 前端：新增内置工具「生物资料」（`/mobs` 检索列表 + `/mobs/{slug}` 详情），展示基础属性（生命值 / 维度 / 攻击方式 / 群系）、生成条件、掉落表（数量 / 概率 / 说明）、驯服与繁殖、来源与核验，支持收藏与分享 URL（`q`/`category`/`version`/`page`）。
> - 版本口径：沿用 `versioning.Satisfies` 精确判断，按生物实际加入版本声明（如兔子 `1.8+`、山羊 / 发光鱿鱼 `1.17+`、悦灵 / 监守者 `1.19+`、嗅探兽 / 骆驼 `1.20+`、犰狳 `1.20.5+`、旋风人 `1.21+`），筛 `1.17` 命中 `1.0+`/`1.17+`、不命中 `1.20.5+`。
> - 数据来源：Minecraft Wiki 各生物页面（2026-10-18 抓取），每条生物带 `source_url`、`data_version=wiki-2026-10` 与 `last_verified_at`；颜色 / 变体（羊毛颜色、山羊角数量等）合并为单条掉落并在 `conditions`/`note` 说明。
> - 顺带扩充：`items_demo.json` 补录 19 种掉落物（骨头、生羊肉、生猪排、烈焰棒、铜锭、金粒、海晶碎片 / 砂粒、潜影壳、不死图腾、下界之星、凋灵骷髅头颅、幽匿催发体、雪球、海草、鳞甲、犰狳鳞甲、山羊角、龙蛋），使「掉落 → 物品详情页」的关联链接**全部可跳转**（`mob_drops_missing_items` = 0）；收藏类型白名单新增 `mob`。
> - 修复：`mob_drops.count_min` / `count_max` / `chance` 的 GORM `default` 标签会让零值字段被省略而改写为数据库默认值（`count_min=0` 被写成 `1`，导致「0–2 掉落」失真）；已移除结构体上的 default 标签，确保显式值（含 0）如实落库。

- [x] 建立药水配方与酿造步骤资料。
- [x] 展示药水效果、持续时间及增强或延长方式。
- [x] 建立村民职业、交易等级和交易表资料。（§9-B）
- [x] 建立生物掉落、生成条件、驯服与繁殖资料。（§9-C）
- [x] 支持按名称、类别和游戏版本检索。（§9-A 药水：关键词/类别/精确版本；§9-B 村民：关键词/职业类别/精确版本）
- [x] 将相关物品、配方和百科链接到对应资料页。（§9-A 药水：原料 → 物品详情、基底/结果 → 药水详情；§9-B 村民：工作站/成本/结果 → 物品详情；未收录物品回退纯文本 ID，不生成死链）
- [x] 校验资料来源和版本适用范围。（导入器强制 `source_url` 与 `supported_versions`，悬空药水引用致命失败；后台数据质量新增对应检查项）

### 10. 世界分析工具扩展

> 本批已落地：新增 `POST /mc/capabilities`（各能力在 `1.8-1.12 / 1.13-1.15 / 1.16-1.17 / 1.18+` 的支持矩阵与说明）；`POST /mc/structures` 返回 `available` / `placement_supported` / `supported` / `dimension` / `probability` 并对不支持版本给出明确文案（`1.12` 及更早分布算法不同，标注「不可作为依据（未核实）」）；新增 `POST /mc/biome`（仅 `1.18+`，其他版本 `supported=false`）；结构表新增**埋藏的宝藏**、参数描述新增维度与频率门槛（前哨站 20% / 宝藏 1% / 矿井 0.4%），种子地图结构覆盖扩充 8 种；候选点区分「理论候选 / 群系可行 / 邻近偏移 / 群系不符」并展示维度与频率，`spacing=1` 结构不纳入地图聚合；`/coordinate` 新增传送门理论配对（1:8）与线路规划；结构与史莱姆区块单次限时 `3s` 并新增 `MC_HEAVY_*` 独立限流与 `MC_HEAVY_CONCURRENCY` 并发上限；新增固定种子 → 期望坐标**回归基线**（显式标注未与游戏逐格比对）。

- [x] 梳理现有结构坐标、史莱姆区块、要塞估算和种子搜索的版本支持范围。
- [x] 评估并补充群系查询与坐标定位能力。
- [x] 评估并补充更多结构的定位支持。
- [x] 优化主世界与下界路线规划及传送门坐标提示。
- [x] 在结果中区分理论候选位置、群系可行性校验和实际生成情况。
- [x] 为耗时计算设置明确的超时、并发和独立限流。
- [x] 为算法结果建立已知种子对照测试。
- [x] 对不支持的版本或无法确认的结果给出明确提示。

### 11. 存档分析与诊断

- [x] 定义存档分析支持范围和文件格式。
- [x] 解析 `level.dat` 的基础信息。
- [x] 统计区域文件与区块的基础信息。
- [x] 展示区块坐标、时间戳和允许展示的 NBT 顶层信息。
- [x] 检查明显异常的区域文件结构，并区分警告与确定错误。
- [x] 生成只读诊断报告。
- [x] 限制上传文件大小、解压输出、解析深度、处理时间和并发量。
- [x] 默认不持久化上传文件；如需缓存，必须设置容量上限、过期时间和清理机制。
- [x] 增加畸形文件、超大数据和异常 NBT 的安全测试。
- [x] 不提供未经充分验证的自动修复功能。

---

## 四、P1：用户体验与运营

### 12. 收藏与个人工作台

- [x] 评估是否引入普通用户账号体系，避免影响现有公开工具免登录使用方式。
- [x] 支持登录用户收藏工具、物品、资源和百科。
- [x] 支持同步最近使用记录。
- [x] 支持管理常用种子和查询记录。
- [x] 提供取消收藏、清理记录和账号数据删除能力。
- [x] 明确个人数据保留期限与隐私说明。
- [x] 对新增账号能力进行认证、授权、CSRF 和限流安全评估。

### 13. 使用统计与运营看板

- [x] 统计工具访问量、使用量和失败率。
- [x] 统计搜索结果数量与高频无结果词。
- [x] 统计资源详情访问量和失效链接数量。
- [x] 统计关键 API 的延迟、错误率和限流触发情况。
- [x] 对统计数据设置保留期限和访问权限。
- [x] 默认避免采集不必要的个人身份信息、完整查询内容或敏感请求数据。
- [x] 在后台提供按时间范围筛选的运营概览。

### 14. 分享与易用性

> **本批已落地**（`ShareButton` + `utils/share.ts`）：
> - 稳定 URL：新增「物品 ID 对照表（`q`/`kind`）」「坐标换算（`dir`/`x`/`y`/`z`/`fx`/`fz`/`tx`/`tz`）」「种子地图（`seed`/`version`/`radius`/`type`，带种子链接打开即自动分析）」「种子与结构工具（全部子表单，编解码在 `utils/seed-tools-query.ts`）」四类；连同既有 7 类（物品/配方/指令/版本/资源/搜索/工具）共 11 个工具支持可分享 URL，且均在标题右侧提供分享按钮（后续 §9-A 又新增「药水与酿造」，现为 12 个）。
> - 秘密信息防护：分享链接取自地址栏（Cookie 为 HttpOnly、CSRF 从不进入 URL），并在复制前剥离疑似机密查询参数与 hash 片段（`isSecretParam`/`sanitizeShareUrl`），有单测锁定。
> - 明确不纳入 URL 的两类：**表单配置类工具**（可视化指令生成器、附魔与装备计算器）以「复制产出」为主分享方式；**区域文件诊断** 因输入是本地文件、无法也无须用 URL 复现。
> - 顺带修复 2 处真实缺陷：`<input type="number">` 上 `v-model` 会被 Vue 自动转成 number，导致坐标换算与指令生成器在用户输入时抛 `trim is not a function`（已按字符串归一）。

- [x] 为可分享的查询结果设计稳定 URL 或分享参数。
- [x] 分享链接中不包含会话 Cookie、CSRF Token 或其他秘密信息。
- [x] 为工具增加清晰的输入说明、示例和错误提示。
- [x] 检查移动端布局、表格、复制按钮和长文本展示。
- [x] 为主要页面补充键盘操作与可访问性检查。
- [x] 为空状态、加载状态和错误状态统一设计规范。

---

## 五、P2：进阶资料与生态

### 15. 红石与机器设计资料库

> **本批已落地（§15）**：
> - 数据层：迁移 `0018_redstone.sql` 新增两表——`redstone_components`（红石元件：`category` 类别 CHECK 约束 + 别名 / 适用版本 JSONB + 来源与核验时间）
>   与 `redstone_designs`（机器设计：`purpose` 用途 / `difficulty` 难度 / `review_status` 审核状态 CHECK 约束 + `materials` 材料 JSONB 数组 + 作者 / 授权 / 存档下载 + 来源与核验时间 + 审核留痕），并建检索与审核索引。
> - 元件资料：`redstone_components` 27 条，覆盖五类（信号源 6 / 传输 3 / 逻辑 2 / 执行 10 / 传感 6）；核心工作机制与信号说明用**文字**描述，
>   **不写死**未经核验的信号强度 / 延迟等数值。
> - 设计资料：`redstone_designs` 15 条，覆盖六用途（农场 3 / 分类机 2 / 运输 2 / 门与机关 3 / 逻辑电路 3 / 实用装置 2）；标记适用版本、难度、作者与材料成本。
> - 材料成本：`materials` JSONB 元素 `{item_id,name_zh,count_min,count_max}`，`count_max` 缺省等于 `count_min`；`item_id` 为空串表示未收录物品（此时 `name_zh` 必填），前端回退为纯文本，**不生成死链**；数量一律用**区间**表达，不编造精确配方。
> - 截图 / 视频 / 存档：支持 `screenshot_url` / `video_url` / `world_download_url` + `world_download_note`；**版权红线**——`world_download_url` 非空时 `license` 必填（DB CHECK 约束），未授权存档一律不给下载链接，**不伪造下载直链**。
> - 来源与许可：每条元件 / 设计带 `source_url`（Minecraft Wiki）、`data_version=wiki-2026-10`、`last_verified_at`、`license` / `license_note`。
> - 内容审核：`review_status ∈ {pending,published,rejected}`（默认 `pending`），**仅 `published` 对公开接口可见**；审核人取自登录会话（`middleware.SessionFrom(ctx).Username`），并写入审计日志。
> - 接口：公开 `POST /api/v1/redstone/components`、`/redstone/components/{slug}`、`/redstone/designs`、`/redstone/designs/{slug}`；
>   管理端 `POST /api/v1/admin/redstone/designs/list`、`/admin/redstone/designs/review`（`RequireAuth + RequireRole(admin) + CSRF`）。OpenAPI 同步。
> - 导入器：`cmd/seed --type redstone` 逐行校验（失败逐行汇总不中断）、幂等 upsert、写 `data_import_jobs`；单测覆盖正反例（含未授权存档带下载链接、材料数量区间非法、悬空 / 空 `item_id` 无中文名）。
> - 前端：新增内置工具「红石资料」（`/redstone` 元件 / 设计双 tab 检索 + `/redstone/components/:slug`、`/redstone/designs/:slug` 详情），
>   状态与 URL 双向同步（`q`/`category`/`purpose`/`difficulty`/`version`/`page`）；数据质量分类名与收藏白名单（`redstone_design`）同步，内置工具 16 → 17。
> - 后台审核页：`/admin/redstone`（admin 专属）按审核状态 / 用途筛选机器设计（默认只看待审核），逐条「通过 / 驳回」并留审核备注，复用反馈管理的 `ListShell` + `StatusBadge` 范式。
> - 验收记录：已执行 `0018_redstone` 迁移；`cmd/seed -type redstone` 导入 42 条（27 元件 + 15 设计），二次导入「新增 0」；四个公开接口实测 `200`（未收录 slug `404`），管理端未登录 `401`；
>   数据质量校验（材料数量区间非法 0、未授权存档带下载链接 0、空 `item_id` 无中文名 0、缺来源 0）；前端 `tests/redstone.spec.ts`（11 用例）与 `tests/admin-redstone.spec.ts`（4 用例）通过。

- [x] 建立红石元件资料及核心工作机制说明。
- [x] 按用途整理农场、分类机、运输系统等设计。
- [x] 为设计标记适用版本、难度、材料成本和作者。
- [x] 支持截图、视频和合法授权的存档下载链接。
- [x] 保留来源与许可信息，建立内容审核流程。

### 16. Mod 与整合包兼容性查询

> **本批已落地（§16）**：
> - 数据层：迁移 `0019_mods.sql` 新增五表——`mods`（Mod 父条目：`categories` / `loaders` / `mc_versions` JSONB + 来源 / 许可 / 核验时间 + `review_status` 审核留痕）、
>   `mod_versions`（同一 Mod 的不同发布版本：`release_type` CHECK + 下载入口与授权说明 + `verified` 验证状态）、
>   `mod_dependencies`（**已记录**的依赖 / 冲突：`dep_kind ∈ {required,optional,embedded,incompatible}` CHECK + 目标 Mod 引用或回退名 + `source_url` / `verified`）、
>   `modpacks`（整合包父条目：`mc_version` 目标版本 + `loader` + 作者 / 许可 / 审核留痕）、`modpack_mods`（包内 Mod 清单项：已收录引用或回退名 + 版本 / 备注），并建检索与审核索引。
> - Mod 资料：`mods` 17 条（每 Mod ≥2 个发布版本，共 `mod_versions` 34 条）；加载器覆盖 fabric / forge / neoforge / quilt，类别覆盖前置库 / 优化 / 界面 / 科技 / 存储 / 世界等。
> - 依赖与冲突：`mod_dependencies` 12 条，只呈现**已记录**关系，逐条标注 `source_url` 与 `verified`；**不做自动冲突分析**（文档明确为后续评估项）。
> - 整合包：`modpacks` 7 条（每包 ≥6 条清单项，共 `modpack_mods` 57 条）；清单项已收录则解析为可跳转引用，未收录回退纯文本名，**不生成死链**。
> - 版本化：`mod_versions` 按 `(mod_id, version)` upsert 并清理已删版本；`mod_dependencies` / `modpack_mods` 按父记录整体重建（净效果幂等）。
> - 来源与许可：真实 `source_url`（Modrinth / GitHub / 官方站点）、`data_version=curated-2026-10`、`last_verified_at=2026-10-18`；不确定许可的 Mod 留空 `license` 并用 `license_note` 说明「见官方仓库 LICENSE」。
> - **版权红线**：`download_url` 只允许官方 / 作者授权分发入口，**不镜像分发**；非空时所属 Mod 的 `license` / `license_note` 必须已说明（导入器强校验），无授权说明时前端回退 `download_note` 文字说明，**不伪造下载直链**。
> - 内容审核：`review_status ∈ {pending,published,rejected}`（默认 `pending`），**仅 `published` 对公开接口可见**；审核人取自登录会话并写入审计日志。
> - 接口：公开 `POST /api/v1/mods`、`/mods/{slug}`、`/modpacks`、`/modpacks/{slug}`；管理端 `POST /api/v1/admin/mods/list`、`/admin/mods/review`、`/admin/modpacks/list`、`/admin/modpacks/review`（`RequireAuth + RequireRole(admin) + CSRF`）。OpenAPI 同步。
> - 导入器：`cmd/seed --type mods` 两阶段导入（先落全部 Mod 再写依赖，避免依赖引用列表中靠后的 Mod 时悬空），逐行校验（失败逐行汇总不中断）、幂等 upsert、写 `data_import_jobs`；单测覆盖正反例（含悬空引用、缺授权说明的下载入口、非法加载器）。
> - 前端：新增内置工具「Mod 兼容性」（`/mods` Mod / 整合包双 tab 检索 + `/mods/:slug`、`/modpacks/:slug` 详情），状态与 URL 双向同步（`tab`/`q`/`loader`/`category`/`version`/`page`）；
>   Mod 详情展示发布版本、依赖分组（必需 / 可选 / 内嵌 / 不兼容）+ 验证徽章、反向依赖与来源许可；数据质量分类名与收藏白名单（`mod` / `modpack`）同步，内置工具 17 → 18。
> - 后台审核页：`/admin/mods`（admin 专属）Mod / 整合包双 tab 审核（按审核状态 / 加载器筛选，默认只看待审核），逐条「通过 / 驳回」并留审核备注。
> - 验收记录：已执行 `0019_mods` 迁移；`cmd/seed -type mods` 导入 24 条（17 Mod + 7 整合包），二次导入「新增 0」；四个公开接口实测 `200`（未收录 slug `404`），管理端未登录 `401`；
>   数据质量校验（未发布泄漏 0、悬空引用 0、缺授权说明的下载入口 0、非法加载器 0）；前端 `tests/mods.spec.ts`（15 用例）与 `tests/admin-mods.spec.ts`（5 用例）通过。

- [x] 建立 Mod 与整合包的版本、加载器及依赖关系模型。
- [x] 关联同一 Mod 的不同发布版本。
- [x] 展示已知前置依赖与明确记录的冲突。
- [x] 支持查询整合包内的 Mod 清单。
- [x] 为兼容性结论标注数据来源和验证状态。
- [x] 先实现基于可靠元数据的查询，再评估自动冲突分析。
- [x] 优先链接官方或作者授权的下载来源，不未经许可镜像分发文件。

### 17. 用户投稿与纠错

> **本批已落地（§17）**：
> - 数据层：迁移 `0017_feedback_submissions.sql` 在既有 `feedback` 基础上扩展——`feedback_type` 增加 `data_correction`（资料纠错）/ `resource_supplement`（资源补充信息）；
>   新增 `target_kind` / `target_ref`（关联到资源 / 物品 / 开发文档 / 生物 / 村民 / 药水 / 工具等具体条目）、`reviewer` / `reviewed_at` / `review_note`（最近一次审核）、
>   `license_version` / `license_accepted_at`（授权条款确认）、`dedup_hash`（反垃圾摘要，含时间窗索引）；并新增 `feedback_reviews` 表保存**完整审核变更历史**（状态流转 + 审核人 + 结论）。
> - 接口：`POST /api/v1/feedback`（公开投稿，扩展字段 + 授权条款 + 反垃圾）；`POST /api/v1/admin/feedback/update`（审核：状态 + 备注，审核人取自登录会话）；
>   `POST /api/v1/admin/feedback/reviews`（审核历史）。OpenAPI 同步。
> - 默认待审核：所有投稿 `status=new`，管理端逐条审核并留痕。
> - 反垃圾与恶意链接防护：沿用公开反馈限流（每 IP 30 分钟 5 次）；新增 24 小时时间窗内「类型 + 目标 + 标题 + 正文」摘要查重（重复提交返回 409）；
>   `related_url` 仅接受 `http`/`https`，拒绝脚本伪协议与 `localhost` / 私网 / 环回 / 未指定地址。
> - 授权条款：资料纠错与资源补充须携带当前授权条款版本（`2026-10-18`）并勾选确认，服务端强校验；授权版本与接受时间落库。
> - 前端：`/feedback` 投稿表单增加「关联资料类型 / 标识」与授权条款勾选；管理后台「反馈管理」增加审核备注输入、审核人信息展示与可展开的审核变更历史。
> - 测试：后端 `internal/service/content/content_test.go` 覆盖投稿类型 / 关联类型 / 授权条款 / 恶意链接 / 摘要稳定性 / 状态白名单；前端 `tests/feedback-submissions.spec.ts`（7 用例）。
> - 版权处理流程：投稿须为原创或已获授权，接受授权条款后本站可按开源许可整理、转载与再发布；侵权通知（`infringement`）单独分类，见 `/copyright`。

- [x] 允许用户提交资料纠错建议。
- [x] 允许用户提交资源补充信息。
- [x] 所有投稿默认进入待审核状态。
- [x] 记录审核人、审核结果和变更历史。
- [x] 增加重复提交、垃圾内容和恶意链接防护。
- [x] 明确投稿内容的授权条款与版权处理流程。

### 21. 我的世界开发栏目（NeoForge / Forge / Fabric / 版本映射）

> **编号说明**：§18–§20 为「六、横向工程任务」，本节为新增需求（开发栏目），顺序编号延续为 §21，
> 内容归属「进阶资料与生态」。**已落地并验收**：
> - 数据层：迁移 `0013_dev_docs.sql` 新增四表——`dev_docs`（开发文档知识库，正文 `body_md` + 结构化 `api_entries`）、
>   `mapping_sets`（映射数据集）、`mapping_classes` / `mapping_members`（类 → 成员两级映射，`obf / mid / named` 三列名字）；
>   `cmd/seed -type devdocs` 导入 40 条文档（NeoForge 11 / Forge 5 / Fabric 9 / 原版数据 8 / 跨平台 1 / 工具链 6，共 189 条关键接口）。
> - 映射导入器：新增 `cmd/import-mappings`（`internal/mappings` 解析包），支持 **ProGuard（Mojang 官方映射）/ TSRG·TSRG2（MCPConfig）/ Tiny v1·v2（Fabric Yarn）** 与 gzip 自动解压，
>   按 `set_key` **整体替换**导入，逐列保留来源命名空间语义（`mapping_sets.note` 说明各列对应命名空间）。**不预置任何映射数据**：真实混淆名随版本变化且无法核验，预置示例会误导。
> - 接口：`POST /api/v1/dev-docs`、`/dev-docs/facets`、`/dev-docs/{slug}`、`/mappings/sets`、`/mappings/search`、`/mappings/class/{id}`；OpenAPI 与后台数据质量检查（`dev_docs_*` / `mapping_sets_*` / `mapping_members_orphan`）同步补齐。
> - 前端：新增内置工具「开发文档」（`/dev` 检索列表 + `/dev/{slug}` 详情：要点正文、关键接口速查、官方文档与来源、相关条目）与「版本映射查询」（`/mappings`：数据集选择、类 / 方法 / 字段三列对照、成员明细展开、未导入时给出可复制的导入指引）；内置工具 12 → 14，sitemap / SEO / 导航 / 收藏白名单同步。
> - 验收记录：已执行 `0013_dev_docs` 迁移，`cmd/seed -type devdocs` 导入 40 条文档（`dev_docs` 表 40 行）；`/dev-docs`（40 条，facets 覆盖 6 个平台）、`/dev-docs/{slug}`、`/mappings/sets`、`/mappings/search` 均实测返回正常；前端 `tests/dev-docs.spec.ts` 通过。内置工具清单补齐 `dev-docs` / `mappings` 后共 16 项（含 §9-A/B/C 的 `potions` / `villagers` / `mobs`）。
> - 来源与版本口径：每条文档带 `official_url` + `source_url`（二者可为不同地址，例如 NeoForge 官方站在核对网络中不可达时，以官方仓库文档源文件作为可核验锚点并在说明中注明）+ `last_verified_at`；
>   版本适用范围沿用 `versioning` 声明式集合，查询侧用 `versioning.Satisfies` **精确判断**。
> - 全站搜索接入（2026-10-19 补做）：迁移 `0020_dev_docs_search_vector.sql` 为 `dev_docs` 增加 `search_vector` 生成列 + GIN 索引；`internal/search` 新增 `devdoc` 类型（命中中英文标题 / slug / 摘要 / 正文 / 标签，仅返回已发布条目，结果直达 `/dev/{slug}`）；前端搜索页类型筛选与全局搜索框标签新增「开发文档」；集成用例 `TestSearchDevDocs` 通过（覆盖标题命中、正文命中、草稿排除、全站并入、`type=item` 不混入）。
> - **未做**：映射数据需使用者自行导入，仓库内不含任何真实映射文件。

- [x] 建立开发知识库：NeoForge 官方文档要点与接口速查。
- [x] 建立开发知识库：Forge 官方文档要点与接口速查。
- [x] 建立开发知识库：Fabric 开发文档要点与接口速查。
- [x] 建立开发知识库：原版数据包 / 资源包 / NBT / 协议等不依赖加载器的内容。
- [x] 建立 Minecraft 版本映射与类 / 方法查询（`cmd/import-mappings` 导入 + 站内三列对照检索）。
- [x] 支持按名称、平台、主题与游戏版本检索开发文档。
- [x] 校验资料来源与版本适用范围（导入器强制 `source_url` 与 `mc_versions`；映射数据集强制记录来源与许可）。
- [x] 开发文档接入全站搜索（`internal/search` 新增 `devdoc` 类型：命中标题 / 摘要 / 正文 / 标签，仅返回已发布条目，结果直达 `/dev/{slug}`）。

---

### 22. MC 源码浏览器（mcsrc 整合）

> **背景**：`mcsrc-main` 是浏览器内的 MC 反编译源码浏览器（React 19 + Monaco + Vineflower WASM 双版本 + Dexie/IndexedDB 缓存）。
> 整合方式经确认采用**后端 Go 侧提供支持**：由服务端下载并缓存官方 jar、构建类索引，访客只做检索——
> 既避免每个访客各自下载数百 MB，也避免在仓库或服务端分发反编译产物（版权红线）。
>
> **已落地并验收（切片 1）**：
> - 缓存层 `internal/mcsrc/store.go`：按版本取官方下载地址 → 下载 → **校验官方 sha1** → 临时文件 + 原子改名落盘（`<version>-<side>.jar`）；
>   版本号白名单正则防路径穿越；单 jar 体积上限 `MCSRC_JAR_MAX_MB`（默认 512）；同一 `version+side` **单飞锁**（并发准备只下载一次）。
> - 索引层 `internal/mcsrc/index.go`：**只读 zip 中央目录**（不解析 class 字节码），过滤 `META-INF/` 与 `module-info.class`，产出全限定名 / 简单名 / 包名 / 内部类标记 / 字节数；
>   类数量上限 `MCSRC_INDEX_MAX_CLASSES`（默认 60000，超出截断并标记 `truncated`）。
> - 接口：`POST /api/v1/mcsrc/status`、`/mcsrc/prepared`、`/mcsrc/classes/list`（**公开只读**）；`POST /api/v1/mcsrc/prepare`（**`ops.mcsrc` 权限点 + CSRF + 重接口限流 + 并发上限 1**）。
> - 前端：内置工具「MC 源码浏览器」（`/mcsrc`，`slug=mcsrc`）：版本列表 + 客户端 / 服务端侧别切换 + 状态卡片 + 类检索表格 + 管理员「准备该版本」按钮（非管理员提示联系管理员）。
> - 验收记录：`go vet ./...` 通过；`internal/mcsrc` 单测（索引 / 检索 / 安全边界 / 单飞 / 截断 / 零值时间）+ `handler` 单测 + 集成测试 `TestMcsrcRoutesAuthGating`（匿名准备 401、editor 403、非法版本 422、公开只读 200）全部通过；
>   **真实上游用例** `TestLivePrepareRealJar`（`MCSRC_LIVE_TEST=1`）：1.8.9 客户端 jar（8,461,484 字节，sha1 `3870888a…`）sha1 校验通过、索引 2507 个类、检索到 `net.minecraft.server.MinecraftServer`、二次准备不重复下载；
>   运行中的服务实测 `/mcsrc/prepared` 200、非法版本 422、未建索引 409 `NEED_PREPARE`、匿名 `/mcsrc/prepare` 401。
> **已落地并验收（切片 2：类结构视图 + 继承关系，纯 Go、无需 JVM）**：
> - 解析层 `internal/mcsrc/classfile.go`：手写 JVM class 文件解析（JVMS §4）——常量池全 tag（含 Long / Double 双槽位）、修饰符 / 种类 / 父类 / 接口 / 字段 / 方法；属性表按长度跳过（结构视图不需要字节码正文）。
>   所有读取带边界检查并累积错误：**截断、伪造 tag、越界属性长度均返回错误且绝不 panic**（jar 内容按不可信输入处理）。
> - 渲染层 `internal/mcsrc/descriptor.go`：JVM 描述符 → Java 类型 / 方法签名（基础类型、数组、内部类名 `a/b/C$D` → `a.b.C$D`）；`void` 仅允许作返回类型（字段 / 参数中出现即报错）。
> - 服务层 `internal/mcsrc/structure.go`：`ClassStructure`（点分 / 斜杠类名等价、`in_jar_super` / `in_jar_interfaces` 只查 zip 中央目录，与 jar 体积无关）+ `Hierarchy`（祖先链 + 直接子类 + 直接 / 递归计数）；
>   关系索引按 **jar 摘要缓存于内存**（最多保留 4 个版本，摘要变化自动失效），构建用单飞锁避免重复全量扫描；损坏 class 条目跳过而非整体失败；子类列表超 500 截断但仍报准确总数。
> - 接口：`POST /api/v1/mcsrc/classes/detail`、`/mcsrc/classes/hierarchy`（**公开只读**，与检索同口径；未缓存 jar → 404、类不在 jar → 404 `CLASS_NOT_FOUND`、未建索引 → 409 `NEED_PREPARE`、缺参 / 非法类名 → 422）。
> - 前端：类检索表格点击类名或「查看结构」进入详情面板——修饰符 / 种类 / class 版本 / 父类 / 接口 / 继承关系（祖先链与子类均可点击继续下钻）+ 字段表 + 方法表（合成方法默认折叠，可勾选显示）。
> - 验收记录：`go vet ./...` 通过；`internal/mcsrc` 新增单测（合成 class 字节解析 / 描述符渲染 / 修饰符渲染 / 截断不 panic / 结构视图 / 继承关系 / 关系索引失效 / 子类截断）+ `handler` 单测 + 集成测试扩展全部通过；
>   **真实上游用例**（`MCSRC_LIVE_TEST=1`）实测：1.8.9 客户端 jar 的 `net.minecraft.server.MinecraftServer` 解析出 53 字段 / 129 方法 / class 版本 50（Java 6），继承关系实测 `aye extends axu extends avp extends java.lang.Object` 并**反向验证父类子类列表包含该子类**，jar 外类（`java.lang.Object`）返回 `CLASS_NOT_FOUND`。
> **已落地并验收（切片 3-D1：源码浏览 —— 浏览器内 Vineflower WASM 反编译，无需部署侧 JVM）**：
> - **路线修正**：原计划「依赖部署侧 JVM / Vineflower」作废。经逐文件核对 `mcsrc-main` 反编译链路，上游**从来不在服务端反编译**，而是在 Web Worker 内跑 **Vineflower 的 WASM/JS 移植版**（npm `@run-slicer/vf`，Apache-2.0/MIT）；因此直接复用同一引擎，**部署侧无需 JVM**。
>   （已排除的替代路线：wazero 跑上游 TeaVM 的 WasmGC 产物——wazero 明确不支持 WasmGC；纯 Go 自研反编译器——无生产级实现。）
> - 后端 `GET /api/v1/mcsrc/jar?version=&side=`（**公开只读**）：流式下发已缓存的官方 jar，`http.ServeContent` 支持 `Range` 与条件请求，`ETag` 取官方 sha1、`Cache-Control: public, max-age=86400`；未缓存 → 404（不触发下载）；缺 `version` / 非法版本号 → 422。`Store.OpenJar` 复用 `normalize` 做版本号白名单校验（防路径穿越）。
>   **这是 mcsrc 唯一的 GET 接口**：二进制大文件下载需要 `Range` / `ETag` 语义，无法用「POST + JSON body」表达。
> - 前端 `src/mcsrc/`：`vineflower.ts`（显式用 Vite `?url` 解析 `vf.wasm` 再交给 `load()`，避免依赖预打包下 `new URL(...)` 解析失败；WASM 失败回退包内纯 JS 运行时）、`decompile.worker.ts`（Comlink 暴露；`@katana-project/zip` 在 Worker 内解析 zip 并按 jar 缓存条目表；`source` / `resources` / `options` 调用形态与上游一致）、`decompiler.ts`（懒创建 Worker；jar 按 `(version, side)` 只下载一次，失败不缓存）、`monaco.ts` + `components/mcsrc/SourceViewer.vue`（Monaco 只读 Java 查看器，跟随站点明暗主题）、`names.ts`（点号 / 斜杠类名转换）。
> - 前端 `McsrcView.vue`：类列表与类详情面板均有「查看源码」→ 源码视图三态（反编译中 / 失败 / Monaco 源码）；切换版本 / 侧别 / 翻页自动关闭；Monaco 与 Worker 均**懒加载**（不打开源码面板不载入）。
> - 依赖新增：`@run-slicer/vf`（npm 别名 `vf-1.12.0`，Vineflower 1.12.0）、`@katana-project/zip`、`comlink`、`monaco-editor`；`vite.config.ts` 新增 `worker.format: 'es'`（Worker 内有动态 import 回退，需代码分割）。
> - 验收记录：`go vet ./...` 通过；`handler` 单测 `TestMcsrcJarDownload`（未准备 404 / 缺 version 422 / 非法版本 422 / 200 且字节与缓存 sha1 一致 + `Content-Type` + `ETag` / Range 206）、集成测试 `TestMcsrcRoutesAuthGating` 新增 `GET /mcsrc/jar` 未缓存 404 与缺 version 422；`go test ./...`（含集成）全绿；
>   **真实上游端到端**：临时程序实测 1.8.9 客户端 jar（8,461,484 字节 / sha1 `3870888a…`）——`GET /jar` 200 且 sha1 与缓存摘要一致、`Range: bytes=0-3` → 206（`504b0304`）、`If-None-Match` → 304、未缓存版本 → 404；
>   **真实反编译链路**（Node 24，与 Worker 相同的引擎与调用形状）从真实 1.8.9 jar（2507 类）反编译 `net.minecraft.server.MinecraftServer` 成功（耗时约 1s，产出含 `class MinecraftServer` 与 `tick` 方法的真实 Java 源码）；
>   前端 `vue-tsc` 通过、`vitest run` **54 文件 / 517 例**全绿（新增 `tests/mcsrc-source.spec.ts` 5 例 + `tests/mcsrc-decompiler.spec.ts` 5 例）、`vite build` 通过（`SourceViewer` 3.96 MB 与 `vf.wasm` 3.77 MB 均为懒加载分块）。
> **已落地并验收（切片 3-D2：token 着色 + 点击跳转 —— 跳转定义，字段与方法沿继承链定位声明类）**：
> - **上游语义对齐**：逐文件核对 `mcsrc-main` 的 `src/logic/{Tokens,FindDeclaration}.ts`、`src/ui/{Code,CodeUtils,CodeExtensions}.tsx`，确认上游的 token 模型（`class` / `field` / `method` / `parameter` / `local` + `declaration` 标记 + `className` / `name` / `descriptor`）、装饰方式（`inlineClassName` 分色 + 可跳转手型）与跳转机制（Monaco **定义提供器 + 编辑器打开器**，即 Ctrl/⌘ + 点击 / F12 / 右键「转到定义」）；字段与方法的声明定位是「先看引用点所属类，未命中再沿祖先链逐级向上」。
> - **Worker 收集 token**：`decompile.worker.ts` 向引擎传 `tokenCollector`（`start` / `visitClass` / `visitField` / `visitMethod` / `visitParameter` / `visitLocal` / `end`），按源码字符区间产出 token；额外为 import 行补 class token；结果按 `start` 排序后随源码返回，并**按 `(version, side, className)` 缓存反编译结果**（跳转来回不重复反编译）。
> - **类名对齐（关键修正）**：实测发现 Vineflower 的 `visitClass` 对内部类给出的是**内部名 + `$`**（如 `adp$a`），而**源码文本**里内部类写作 `adp.a`（Java 语法），jar 条目与后端索引用 `adp$a`。因此新增 `classLookupCandidates()`（原名 → 最外层类 → 逐级把包分隔符换成 `$`）在 Worker 内用 jar 条目集合探测，把 token 的 `className` 统一对齐到后端约定；`inJar` 由此可准确标注（仅 jar 内类可跳转）。
> - **跳转解析** `src/mcsrc/navigation.ts`：`resolveDeclaration(version, side, query)` —— 类 token 直接返回自身；字段 / 方法 token 先用 `classes/detail` 查引用点所属类是否声明该成员（方法按 `name + descriptor` 区分重载、字段按名），未命中再按 `classes/hierarchy` 的祖先链逐级向上，返回**声明类 + 定位目标**；类不在 jar 内（索引 404）视为「不声明」并继续向上，最终回退到引用点所属类（至少能打开源码）。成员集合按类缓存（跳转同一批类不重复请求索引）。
> - **前端接入**：`SourceViewer.vue` 新增 token 装饰（`inlineClassName` 分色 + 可跳转 token 手型光标与点状下划线）、Java 定义提供器（返回 `mcsrc-goto://` 自定义 URI，含 `#method:name:descriptor` 片段）、悬停提示（显示跳转目标与快捷键）、编辑器打开器（解析 URI → 抛出 `jump` 事件）、以及**成员声明定位**（`setSelection` + `revealRangeInCenter`）；`McsrcView.vue` 收到 `jump` 后解析声明类并打开源码（目标即当前类时只重新定位、不重复反编译），源码面板顶部给出跳转提示；`style.css` 新增明暗双主题 token 配色。
> - **验收记录**：`vue-tsc --noEmit` 通过；`vitest run` **56 文件 / 534 例**全绿（新增 `tests/mcsrc-tokens.spec.ts` 6 例 + `tests/mcsrc-navigation.spec.ts` 6 例，扩展 `mcsrc-decompiler.spec.ts` +2、`mcsrc-source.spec.ts` +3）；`vite build` 通过；
>   **真实上游验证**（Node 24 + 真实 1.8.9 客户端 jar + 真实 Vineflower 纯 JS 运行时，逐段复刻 Worker 逻辑）：反编译 `net.minecraft.server.MinecraftServer`（31,093 字符）产出 **1493 个 token**（引擎 1457 + import 36：class 309 / method 450 / field 311 / parameter 156 / local 267），抽样 8 类 token 的字符区间与源码文本一致、**越界 0 个**、`inJar` 标注与 jar 条目完全一致（1231 个可跳转）、类名**无残留斜杠**且 `inJar=true` 的类名均能在 jar 中命中、方法 / 字段描述符格式全部合法、内部类引用（`adp.a` → `adp$a`、`MinecraftServer$1`）已正确对齐 `$` 形式。
> - **未做（切片 3 余项）**：查找引用（需新增引用索引）、字节码指令视图、映射重命名展示、Javadoc 备注。

- [x] 切片 1：版本选择（复用 `POST /api/v1/versions`）+ jar 获取与缓存（官方 sha1 校验 + 单飞 + 体积上限）。
- [x] 切片 1：类索引与检索（zip 中央目录建索引 + 相关度排序 + 包过滤 + 分页）。
- [x] 切片 2：类结构视图（修饰符 / 种类 / 父类 / 接口 / 字段 / 方法，纯 Go 解析 class 字节码）+ 继承关系（祖先链 + 直接子类）。
- [x] 切片 3-D1：源码浏览（浏览器内 Vineflower WASM 反编译 + Monaco 展示）——`GET /mcsrc/jar` 下发缓存 jar，反编译在访客浏览器内完成，**无需部署侧 JVM**。
- [x] 切片 3-D2：token 着色 + 点击跳转（Ctrl/⌘ 点击 / F12 跳转定义；类跳类，字段与方法沿继承链定位声明类并选中声明处）。
- [ ] 切片 3 余项：查找引用、字节码指令视图、映射重命名展示、Javadoc 备注。

### 23. 后台账号与权限体系

> **已落地并验收**：
> - 权限底座（迁移 `0021_roles.sql`）：`roles` 表（`name` 主键、`permissions` JSONB、`is_system`）+ 内置 `admin` / `editor` / `user` 种子；
>   `admins.role` 由 CHECK 改为**外键引用 `roles.name`**，使自定义角色可立即分配；含反向 SQL。
> - 权限点（17 项 / 4 组：内容管理 `content.*`、内容审核 `review.redstone` / `review.mods`、账号与权限 `accounts.admins` / `accounts.users` / `roles.manage`、系统与运营 `ops.audit` / `ops.dataquality` / `ops.searchstats` / `ops.analytics` / `ops.linkhealth` / `ops.mcsrc`）；路由鉴权由硬编码 `RequireRole(admin)` 改为 `RequirePermission(roleSvc, <权限点>)`，内置角色行为与改造前等价。
> - 角色管理页（`/admin/roles`）：列表 + 新建 / 编辑弹窗 + 权限点分组勾选 + 内置 admin 锁定 + 删除确认；内置角色不可删除 / 改名，`admin` 削权返回 422，被引用角色删除返回 409。
> - 普通用户管理（`/admin/users`，只读）：用户名搜索 + 状态筛选 + 分页 + 个人数据概览（收藏 / 最近使用 / 常用种子计数）；后台仪表盘「用户总数」取该接口 `total`。
> - 登录 / `me` 返回 `permissions`，侧边栏按权限点过滤入口（后端 403 兜底）。
> - 验收记录：`go vet ./...` 通过；集成测试 `TestRoleManagement`（editor 403、非法权限点 422、删内置 422、admin 削权 422、自定义角色权限真实生效、被引用角色 409、删号后 204）通过；前端 `tests/admin-roles.spec.ts`（7 例）与 `tests/admin-users.spec.ts` 通过。
> - **统一账号体系（迁移 `0022_accounts.sql`）已落地并验收**：`admins` / `users` 合并为 `accounts`（`username` 全局唯一、`role` 外键引用 `roles.name` 且默认 `'user'`），数据搬迁 + **8 个外键重指** + `DROP TABLE admins` / `users`（含反向 SQL）；`domain.Account` 落地，`Admin` / `User` 改为类型别名；后台侧查询一律 `role <> 'user'`、用户侧一律 `role = 'user'`；双登录入口保留但**共用同一会话 Cookie**（`SESSION_COOKIE_NAME`，默认 `mc_session`；`USER_SESSION_COOKIE_NAME` 废弃），`auth.Session.AdminID` → `AccountID`。
> - 验收记录（0022）：开发库 `mc_tools` 实测版本 **22** / 表 **43** / `accounts` 1 行 / 8 外键指向 `accounts` / 旧表已删；集成测试 `TestUnifiedAccountBoundaries`（9 项边界）通过；运行中服务 curl 6 项（含普通用户登后台 401、用户会话访问后台 401 / 403）通过。

- [x] 角色与权限底座（`roles` 表 + 权限点 + `RequirePermission` 替换硬编码角色）。
- [x] 角色管理页（新建 / 编辑 / 权限点勾选 / 内置角色保护）。
- [x] 普通用户管理（列表 + 搜索 + 状态筛选 + 分页 + 个人数据概览）。
- [x] 统一账号体系：合并 `admins` / `users` 为 `accounts`（`role` 外键引用 `roles.name`），重指 8 个外键，迁移后删除旧表（迁移 `0022_accounts.sql`，含反向 SQL；已获确认后执行）。

---

## 六、横向工程任务

### 18. API、测试与文档

- [x] 新增接口同步更新 `backend/openapi/openapi.yaml`。（§17 投稿、§15 红石、§16 Mod 兼容性均已同步，`js-yaml` 校验通过）
- [x] 为新增数据库变更编写迁移脚本。（`0017_feedback_submissions` / `0018_redstone` / `0019_mods`，含回滚说明）
- [x] 为新增功能补充后端单元测试与必要的集成测试。（`content_test.go` 投稿校验；`redstone_test.go` / `mods_test.go` 导入器；`linkhealth` 集成测试实跑通过）
- [x] 为前端交互补充 Vitest 测试。（`feedback-submissions.spec.ts` 7 例、`redstone.spec.ts` 11 例、`mods.spec.ts` 15 例；`admin-roles.spec.ts` 7 例、`mcsrc.spec.ts` 9 例；`mcsrc-source.spec.ts` 8 例、`mcsrc-decompiler.spec.ts` 7 例、`mcsrc-tokens.spec.ts` 6 例、`mcsrc-navigation.spec.ts` 6 例、`app-footer.spec.ts` 2 例；当前合计 57 文件 / 536 用例）
- [x] 执行 `go vet ./...` 和 `go test ./...`。（全绿；并顺带修掉两处预存在的测试隔离缺陷：`cmd/api` 统计采集器缓冲跨用例累积、`linkhealth` 集成测试与 `cmd/api` 争用同一测试库 `public` schema——改为专用 schema；`go test ./...` 并行连续 2 次全绿）
- [x] 执行 `vue-tsc -b --noEmit`、前端测试和 `vite build`。（`vue-tsc` 无错误；`vitest run` 57 文件 / 536 用例全绿；`vite build` 成功）
- [x] 为关键算法准备固定输入与预期输出的回归用例。（种子地图 / 结构、区块文件解析、附魔铁砧花费、指令生成均已有固定用例）
- [x] 更新 `Function.md`，只将实际完成并验收的功能标记为已实现。
- [x] 更新部署、备份恢复和数据导入说明。（`README.md` 补齐 `potions`/`villagers`/`mobs`/`devdocs`/`redstone`/`mods` 导入命令与 `--type all` 依赖顺序；部署与备份恢复章节经实测核对）

### 19. 安全与可靠性

- [x] 对文件上传功能统一设置大小、超时、并发和解压限制。（`.mca` / `level.dat` 上传统一 64 MiB 上限 + 5s 解析时限 + `ConcurrencyLimit` + 重接口限流）
- [x] 对耗时任务设置上下文取消与明确的资源上限。（链接检查并发硬顶 16 / 默认 4 + 单请求超时；上传解析 deadline；指标 flush 超时）
- [x] 对外部链接请求持续执行 SSRF 防护与逐跳重定向校验。（`internal/linkcheck` 拒绝环回 / 私网 / 链路本地 / 未指定地址，并限制重定向跳数）
- [x] 为公开 API 设置合理的限流与请求体大小上限。（全局 `RateLimit` 中间件；73 处 `http.MaxBytesReader` 按接口设上限）
- [x] 确认后台新增接口继续执行身份认证、角色校验和 CSRF 校验。（§15/§16 审核路由与本批角色 / 用户 / 源码库路由均为 `RequireAuth + RequirePermission + CSRF`，匿名 401、无权限 403 已实测）
- [x] 确保审计日志不记录密码、Cookie、Authorization 或连接串。（审计仅落库调用方显式给出的 before/after 摘要 + 请求 ID / 来源 IP / 结果；请求头与请求体不落盘）
- [x] 为关键任务失败、数据库异常和缓存异常提供可观察性。（指标中间件 + `AccessLog`；审计写入失败降级告警；本批补齐 `response.HandleError` 的 500 错误服务端日志）
- [x] 定期验证数据库备份能够实际恢复。（实测 `scripts/backup.sh` 备份 → 独立库恢复，14 张核心表行数逐一比对一致）

### 20. 交付验收规则

- [x] 功能实现与需求文档一致。
- [x] 数据库迁移可在干净环境执行。（在全新库执行 `cmd/migrate`：22 个迁移全部应用，43 张表）
- [x] 新增接口已更新 OpenAPI。
- [x] 单元测试、集成测试及前端构建通过。
- [x] 权限、输入校验、限流和异常处理经过检查。
- [ ] 移动端与桌面端主要流程可用。
      - 现状：桌面端已走查；移动端以设备模拟视口（375×812）走查表单与结果展示通过，**真机走查仍待人工完成**。
- [x] 数据来源、版本范围和许可信息清楚。
- [x] `Function.md` 已根据实际验收结果更新。

---

## 七、推荐实施顺序

1. [x] 统一物品资料模型与数据导入机制。
2. [x] 建立版本化资料规则与数据质量校验。
3. [x] 建立物品、配方、指令、百科和资源之间的关联。
4. [x] 改进搜索无结果体验与资源元数据。
5. [x] 开发可视化指令生成器。
6. [x] 开发附魔与装备计算器。
7. [x] 扩展世界分析工具并补齐算法回归测试。
8. [x] 开发存档诊断工具。
9. [x] 完成酿造与养成资料（§9）：药水与酿造（§9-A）、村民交易（§9-B）、生物资料（§9-C）均已交付。
10. [x] 增加收藏、个人工作台和使用统计（收藏与个人工作台、使用统计与运营看板均已完成）。
11. [ ] 按实际用户需求扩展红石资料、Mod 兼容性与投稿体系。
12. [x] 新增「我的世界开发栏目」（§21）：开发文档知识库（NeoForge / Forge / Fabric / 原版数据）+ 版本映射与类 / 方法查询（映射数据由 `cmd/import-mappings` 按官方来源导入，不预置）。
13. [x] 补齐后台账号与权限体系（§23）：角色与权限底座 + 角色管理页 + 普通用户管理。
14. [ ] 整合 MC 源码浏览器（§22）：切片 1（版本选择 + jar 缓存 + 类索引 / 检索）、切片 2（类结构视图 + 继承关系，纯 Go 无 JVM）、切片 3-D1（浏览器内 Vineflower WASM 反编译 + Monaco 源码浏览）、切片 3-D2（token 着色 + 点击跳转）已交付；切片 3 余项（查找引用 / 字节码指令视图 / 映射重命名 / Javadoc 备注）待办。

> **执行原则：** 一次优先完成一个可验收的垂直功能；先保证数据准确、版本明确和安全边界，再扩展功能数量。没有完成测试和验收的事项，不要在 `Function.md` 中标记为“已实现”。
