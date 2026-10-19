# MCkit 用户成长系统开发规格说明

> 文档类型：可执行开发规格（Implementation Specification）  
> 项目：MCkit Minecraft Java 工具与社区平台  
> 技术栈：Go + Chi + GORM + PostgreSQL 16；Vue 3 + TypeScript + Vite  
> 状态：待评审后实施  
> 目标：统一实现经验、等级、成就、每日任务、帖子互动奖励与成长审计  
> 原则：服务端判定、业务事件驱动、经验账本可审计、重复事件不可重复领奖

---

## 1. 开发目标与范围

### 1.1 建设目标

将以下能力建设为一个统一的用户成长系统：

1. **经验系统（XP）**：记录有效行为带来的经验增减。
2. **等级系统**：依据当前有效经验计算等级、称号和升级进度。
3. **成就系统**：依据长期累计行为解锁成就，已解锁记录默认永久保留。
4. **每日任务系统**：每日登录、每日点赞、每日留言等任务，按站点时区每日重置。
5. **成长记录**：记录经验流水、任务完成、成就解锁、人工调整和冲正。
6. **后台管理**：配置规则、任务、成就、等级阈值，查询流水与异常，执行受审计保护的人工操作。
7. **社区互动接入**：将帖子点赞、有效留言等真实业务行为接入成长事件中心。

### 1.2 首期范围

必须实现：

- 每日登录任务；
- 每日点赞任务；
- 每日留言任务；
- 有效使用工具；
- 收藏资源；
- 提交资源及审核通过；
- 帖子点赞、帖子留言；
- 个人资料完善、邮箱验证（项目已有邮箱验证能力时接入）；
- 经验、等级、成就、每日任务进度；
- 后台规则管理、经验流水、操作审计；
- 幂等、防刷、并发与事务测试。

首期不实现：

- 经验商城、虚拟货币；
- 连续签到加成；
- 复杂称号装备或等级特权；
- 排行榜；
- 用户间经验转移；
- 可执行脚本或任意 SQL 的成就条件；
- 需要实时推送基础设施的复杂通知中心。

### 1.3 不可违反的原则

- 前端不得直接决定经验数值、等级、任务完成状态或成就解锁状态。
- 普通用户 API 不提供直接发放经验的能力。
- 点赞、留言等任务只能由成功提交且通过服务端校验的业务事件推进。
- 重复请求、并发请求、取消后重做、删除后重发不得绕过奖励唯一约束。
- 原始经验流水不得物理删除；错误奖励通过冲正流水处理。
- 任务每天重置，长期成就不按天重置。
- 等级首期只用于展示和成长激励，不作为后台权限或内容审核权限的依据。

---

## 2. 统一概念与数据定义

### 2.1 经验字段

- `current_xp`：当前有效经验，作为等级计算依据。
- `lifetime_earned_xp`：历史累计正向发放经验，不因冲正而减少。
- `lifetime_reversed_xp`：历史累计冲正经验的绝对值。
- `level`：由 `current_xp` 与等级阈值计算得到，不接受客户端传入。
- `xp_to_next_level`：当前经验距离下一级阈值的差值；最高等级时为 `0`。

有效经验计算必须与账本一致。建议账本流水 `delta_xp` 使用有符号整数：奖励为正，冲正为负。`current_xp` 应等于所有有效流水的 `delta_xp` 之和；累计正向发放和累计冲正分别统计。

### 2.2 成长事件

成长事件是由业务模块产生、可被成长系统消费的已确认事实，例如：

- `user.registered`
- `user.profile_completed`
- `user.email_verified`
- `tool.used_validly`
- `resource.favorited`
- `resource.submission_created`
- `resource.approved`
- `resource.broken_report_confirmed`
- `feedback.correction_accepted`
- `post.liked`
- `post.unliked`
- `post.comment_created`
- `post.comment_deleted`
- `wiki.contribution_approved`

事件至少包含：

- `event_id`：全局唯一事件 ID；
- `event_type`：事件类型；
- `actor_user_id`：行为发起者；
- `subject_id`：目标对象 ID，如帖子或资源；
- `occurred_at`：业务事件时间；
- `payload`：经过服务端验证的必要业务信息；
- `source_module`：产生事件的业务模块。

成长模块不得仅凭前端传来的 `event_type`、`user_id` 或 `payload` 发奖。事件必须由内部业务服务在成功事务中产生，或通过事务型 Outbox 投递。

### 2.3 事件、经验和任务的关系

一次真实业务行为可以同时：

- 推进一个或多个每日任务；
- 推进一个或多个长期成就；
- 按经验规则发放经验。

但每个奖励必须有独立且可确定的唯一键。例如，同一次点赞可以推进“每日点赞”任务和“累计点赞”成就，但不能因为两个处理器重复消费事件而重复发放同一笔经验。

---

## 3. 初始经验规则

以下数值作为默认初始值，必须存入数据库规则表，禁止散落在前端或 Go 业务代码中。正式上线前可在后台调整。

| 规则编码 | 行为 | 初始经验 | 发放条件 | 限制 |
|---|---|---:|---|---|
| `profile_completed` | 完善个人资料 | +20 XP | 首次达到服务端定义的完整度标准 | 每账号一次 |
| `email_verified` | 验证邮箱 | +10 XP | 邮箱验证成功 | 每账号一次 |
| `resource_favorited` | 收藏资源 | +2 XP | 收藏关系首次成功创建 | 每用户-资源组合一次 |
| `tool_daily_valid_use` | 有效使用工具 | +3 XP | 工具成功执行并返回有效结果 | 每账号每天一次；任务进度可按不同工具统计 |
| `resource_submission_valid` | 首次有效投稿 | +10 XP | 投稿通过基础校验并进入正式审核流程 | 每个投稿业务 ID 一次；可设置每日上限 |
| `resource_approved` | 投稿审核通过 | +50 XP | 投稿首次进入已通过状态 | 每个投稿业务 ID 一次 |
| `broken_resource_confirmed` | 确认失效资源 | +10 XP | 管理员确认报告有效 | 每个报告 ID 一次 |
| `correction_accepted` | 有效纠错 | +5 XP | 纠错经审核接受 | 每个纠错 ID 一次 |
| `post_like_daily` | 有效点赞 | +1 XP | 对允许点赞的帖子成功建立点赞关系 | 每账号每天最多计 5 次有效点赞行为；每目标帖子只计一次 |
| `post_comment_daily` | 有效留言 | +2 XP | 留言通过服务端校验且满足有效留言标准 | 每账号每天最多计 3 条；同一帖子每天最多计 1 条 |
| `daily_login` | 每日登录任务奖励 | +5 XP | 当日首次成功建立有效登录会话 | 每账号每天一次 |
| `daily_like_task_bonus` | 完成每日点赞任务 | +3 XP | 当日达到任务目标 | 每账号每天一次 |
| `daily_comment_task_bonus` | 完成每日留言任务 | +5 XP | 当日达到任务目标 | 每账号每天一次 |
| `daily_tool_task_bonus` | 完成每日工具任务 | +3 XP | 当日达到任务目标 | 每账号每天一次 |

### 3.1 帖子点赞规则

- 只奖励从“未点赞”成功变为“已点赞”的状态变更。
- 重复点击已点赞按钮不产生新事件、不发经验。
- 取消点赞不返还已经获得的点赞经验，但取消后的再次点赞不再获得同一帖子对应的点赞行为经验。
- 每日点赞任务按当天**不同目标帖子**的有效点赞数统计。
- 用户不能通过给自己的帖子点赞获得成长奖励。
- 被删除、隐藏、判定为垃圾内容或违反社区规则的帖子不计入有效点赞任务。
- 同一账号对同一帖子在历史上只能获得一次该帖点赞行为经验；每日任务可否再次计数以“当天首次有效点赞关系创建”为准，但不得通过取消/重赞反复计数。

### 3.2 留言规则

“留言”包括帖子评论或回复；必须以项目实际社区数据模型为准，统一定义一种有效评论事件。

有效留言的最低标准：

- 已登录用户提交；
- 服务端创建成功；
- 去除首尾空白后内容非空；
- 不属于重复提交；
- 未被反垃圾规则直接拒绝；
- 不是用户对自己内容的灌水式重复留言（建议不奖励自我回复，或只允许其推进不奖励型统计）；
- 未被管理员判定为垃圾评论。

防刷规则：

- 同一用户同一帖子每天最多 1 条留言计入每日任务；
- 每账号每天最多 3 条留言产生经验；
- 相同内容或高度重复内容在短时间内不计奖励；
- 删除留言不返还已获得经验，但若判定为垃圾留言，可通过冲正流水撤销对应经验；
- 不能通过删除后重新发布相同留言重复领奖。

### 3.3 经验规则的后台字段

每条规则至少包含：

- `code`：唯一规则编码；
- `name`：后台展示名称；
- `event_type`：匹配的成长事件；
- `xp_amount`：经验值；
- `daily_limit`：每日奖励次数上限，可为空；
- `per_subject_limit`：每个目标对象的次数上限；
- `enabled`：是否启用；
- `effective_from`、`effective_until`：生效时间范围；
- `version`：规则版本；
- `created_by`、`updated_by`、时间戳。

规则修改只影响生效时间之后的新事件。除非执行经过授权的历史重算任务，否则不得追溯性改变历史奖励。

---

## 4. 每日任务系统

### 4.1 任务设计

每日任务是每日重置的目标；任务奖励与完成状态由服务端计算。首期任务如下：

| 任务编码 | 名称 | 目标 | 奖励 | 进度来源 |
|---|---|---|---:|---|
| `daily_login` | 每日登录 | 当天成功登录 1 次 | +5 XP | 登录成功事件 |
| `daily_like_3` | 点赞 3 次 | 当天给 3 个不同帖子有效点赞 | +3 XP | 有效点赞关系创建事件 |
| `daily_comment_1` | 留下足迹 | 当天发表 1 条有效留言 | +5 XP | 有效留言创建事件 |
| `daily_tool_1` | 工具试用 | 当天有效使用 1 次工具 | +3 XP | 有效工具使用事件 |
| `daily_favorite_1` | 收藏资源 | 当天首次收藏 1 个资源 | +2 XP | 有效收藏事件 |

任务目标与奖励必须由后台配置。若“收藏资源”本身已有 +2 XP 行为奖励，任务完成奖励应明确是额外的任务奖金；后台应显示这两种奖励可能叠加，避免运营误解。

### 4.2 每日边界与时区

- 统一使用配置项 `growth.timezone` 定义站点业务时区，默认 `Asia/Shanghai`。
- 服务端按该时区计算 `task_date`，不能直接用浏览器本地日期。
- 数据库存储时间使用 UTC；每日任务记录保存明确的业务日期。
- 修改站点时区必须谨慎：只影响修改生效之后的新任务日期，不自动合并或拆分历史记录。
- 每日任务不需要定时任务来“清空”；使用 `(user_id, task_code, task_date)` 形成每日独立记录即可。

### 4.3 任务状态

任务状态由服务端根据进度计算：

- `in_progress`：进行中；
- `completed`：已达到目标；
- `claimed`：已发放奖励。

首期建议采用**自动发奖**，达到目标后在同一事务中记录完成并发放奖励，不要求用户手动领取，降低操作复杂度和遗漏。数据结构仍可保留 `claimed_at`，但自动发奖时由系统填写。

### 4.4 防止每日任务重复领奖

数据库必须建立唯一约束：

`UNIQUE (user_id, task_code, task_date)`

任务达到目标时，在事务中锁定或安全更新任务进度，检查是否已完成奖励；只有首次从未完成变为完成时发放一次奖励。并发事件不得造成重复奖励。

### 4.5 登录任务

- 以认证服务确认登录成功、会话创建成功作为依据。
- 刷新页面、调用用户信息接口、续期会话不算新登录。
- 同一账号同一业务日期只推进一次。
- 如果项目支持 OAuth 或多种登录方式，所有成功登录方式统一发出 `user.login_succeeded` 事件。

---

## 5. 等级系统

首期采用 10 级，等级阈值存储在数据库：

| 等级 | 累计有效经验门槛 | 称号 |
|---:|---:|---|
| Lv.1 | 0 XP | 初来乍到 |
| Lv.2 | 100 XP | 方块学徒 |
| Lv.3 | 300 XP | 探索者 |
| Lv.4 | 600 XP | 熟练玩家 |
| Lv.5 | 1,000 XP | 资深玩家 |
| Lv.6 | 1,600 XP | 资源猎人 |
| Lv.7 | 2,400 XP | 红石专家 |
| Lv.8 | 3,400 XP | 资深作者 |
| Lv.9 | 4,600 XP | 工匠大师 |
| Lv.10 | 6,000 XP | MCkit 传奇 |

计算规则：

1. `current_xp` 小于 100 时为 Lv.1。
2. 取所有满足 `threshold_xp <= current_xp` 的最高等级。
3. 升级进度使用当前等级门槛与下一等级门槛计算。
4. 达到 Lv.10 后不再升级，进度显示为满级。
5. 经验冲正导致 `current_xp` 下降时重新计算等级；是否降级不由前端决定。
6. 等级称号仅用于展示，不自动授予管理权限、审核权或资源发布特权。

---

## 6. 成就系统

### 6.1 首期成就清单

| 成就编码 | 名称 | 分类 | 解锁条件 |
|---|---|---|---|
| `account_registered` | 初次见面 | 探索 | 完成注册 |
| `profile_complete` | 新手上路 | 探索 | 完善个人资料 |
| `first_tool_use` | 初次探索 | 探索 | 首次有效使用工具 |
| `tool_explorer_10` | 工具达人 | 探索 | 使用 10 种不同工具 |
| `favorite_first` | 收藏新手 | 收藏 | 收藏 1 个不同资源 |
| `favorite_20` | 收藏家 | 收藏 | 收藏 20 个不同资源 |
| `favorite_50` | 资源考古学家 | 收藏 | 收藏 50 个不同资源 |
| `first_resource_submission` | 初次投稿 | 贡献 | 首次有效提交资源 |
| `resource_approved_1` | 资源贡献者 | 贡献 | 1 份资源审核通过 |
| `resource_approved_5` | 资源收藏家 | 贡献 | 5 份资源审核通过 |
| `resource_approved_10` | 优质贡献者 | 贡献 | 10 份资源审核通过 |
| `first_post_like` | 点赞同行 | 社区 | 首次有效点赞他人帖子 |
| `comment_10` | 交流新手 | 社区 | 累计 10 条有效留言 |
| `comment_100` | 社区交流者 | 社区 | 累计 100 条有效留言 |
| `daily_tasks_7` | 坚持探索 | 成长 | 累计完成 7 个不同日期的每日任务（至少 1 项/日） |
| `level_3` | 稳步成长 | 成长 | 达到 Lv.3 |
| `level_5` | 经验丰富 | 成长 | 达到 Lv.5 |
| `level_10` | 社区达人 | 成长 | 达到 Lv.10 |
| `correction_1` | 纠错先锋 | 特殊 | 1 次纠错审核通过 |
| `wiki_contribution_1` | 百科贡献者 | 特殊 | 1 次百科贡献审核通过 |

### 6.2 成就规则

- 成就解锁记录通过 `(user_id, achievement_id)` 唯一约束防止重复。
- 已解锁成就默认永久保留，不因取消收藏、资源下架或后续编辑自动删除。
- 对违规行为撤销成就必须有明确的管理操作、原因和审计记录；首期可以不提供撤销功能。
- 成就奖励可选，首期默认全部为 `0 XP`，避免每日任务、行为经验和成就奖励多层叠加导致经验膨胀。
- 等级成就由等级计算结果触发，不维护另一份独立等级状态。
- 隐藏成就未解锁时只返回占位信息，不返回名称、描述、条件和奖励细节。
- 成就定义修改必须增加版本号。已解锁记录保留解锁时的名称、描述、版本快照或定义版本引用。
- 新成就启用后，可由管理员触发受控的历史数据补算；补算必须幂等并生成审计记录。

---

## 7. 防刷与内容质量规则

### 7.1 通用防刷

1. 所有奖励以服务端确认的业务事件为准。
2. 对每条经验规则生成确定性 `idempotency_key`。
3. 经验账本对 `idempotency_key` 建立唯一约束。
4. 任务记录对用户、任务、业务日期建立唯一约束。
5. 成就记录对用户和成就建立唯一约束。
6. 关键业务操作与事件写入使用同一数据库事务；若跨模块或跨事务，使用 Outbox/Inbox 模式。
7. 对登录、点赞、评论、投稿等接口执行现有速率限制；成长奖励限额不能替代接口限流。
8. 不以 IP 作为唯一身份判据；IP 仅用于风险信号和限流，避免公共网络误伤。
9. 后台人工调整必须填写原因，记录操作者、目标用户、调整前后数值和关联工单/说明。
10. 不允许用户提交 `xp_amount`、`task_completed=true`、`level` 或 `achievement_id` 来直接获取奖励。

### 7.2 点赞防刷

- 点赞关系必须有数据库唯一约束，例如 `UNIQUE (user_id, post_id)`。
- 只有插入点赞关系成功时发出 `post.liked` 事件。
- 对已存在的点赞关系重复请求返回幂等成功，但不产生新奖励事件。
- 取消点赞只改变点赞关系状态或删除关系，不删除已发放流水。
- 同一用户对同一帖子点赞奖励只发一次，建议唯一键：`like:{user_id}:{post_id}:xp_v1`。
- 用户给自己的帖子点赞不奖励，也不推进奖励型任务。
- 管理员隐藏或删除帖子后，是否冲正点赞经验按明确的内容处罚规则执行，不做无条件自动冲正。

### 7.3 留言防刷

- 留言接口应有速率限制、重复内容检测和内容长度校验。
- 去除首尾空白后为空的留言不得创建。
- 通过标准化内容哈希识别短时间内重复内容；重复留言可按社区规则拒绝或允许发布但不计成长奖励。
- 不要仅用字符数作为唯一质量标准；极短但有意义的回复可按业务规则处理。
- 任务奖励以有效留言事件计数；被标记为垃圾留言后，可冲正相关经验并记录原因。
- 删除留言不得重置该业务事件的幂等记录。

### 7.4 投稿与纠错防刷

- 以投稿、报告或纠错业务 ID 作为唯一奖励关联对象。
- 同一内容重复投稿、重复报告、同一纠错重复审核不得重复奖励。
- 审核状态变更必须校验合法状态流转，例如 `pending -> approved`；重复调用 `approved` 不产生第二次奖励。
- 投稿作者不能审核自己的投稿。
- 管理员账号不能通过自我审核、反复修改状态或重复执行审核接口发放奖励。

---

## 8. 数据库设计（PostgreSQL 16）

> 具体字段命名需遵循仓库现有命名规范。所有表应有主键、必要索引、创建/更新时间；外键策略应与现有数据删除策略一致。用户、帖子、评论、资源表沿用现有业务表，不重复创建。

### 8.1 `user_growth`

用途：用户当前成长投影。

字段建议：

- `user_id`：PK/FK，唯一用户；
- `current_xp BIGINT NOT NULL DEFAULT 0`；
- `lifetime_earned_xp BIGINT NOT NULL DEFAULT 0`；
- `lifetime_reversed_xp BIGINT NOT NULL DEFAULT 0`；
- `level INT NOT NULL DEFAULT 1`；
- `updated_at TIMESTAMPTZ NOT NULL`；
- `version BIGINT NOT NULL DEFAULT 0`：可用于乐观锁。

约束：经验值不得小于 0，等级不得小于 1。若项目需要允许负向有效经验，应先定义是否允许 `current_xp` 低于 0；首期建议不允许。

### 8.2 `experience_ledger`

用途：不可物理删除的经验流水。

字段建议：

- `id UUID PRIMARY KEY`；
- `user_id`；
- `delta_xp BIGINT NOT NULL`；
- `balance_after BIGINT NOT NULL`；
- `source_type VARCHAR(64) NOT NULL`；
- `source_id VARCHAR(128)`；
- `event_id UUID`；
- `idempotency_key VARCHAR(255) NOT NULL UNIQUE`；
- `rule_code VARCHAR(100)`；
- `rule_version INT`；
- `reason TEXT`；
- `operator_user_id`：人工操作时填写；
- `created_at TIMESTAMPTZ NOT NULL`。

索引：`(user_id, created_at DESC)`、`(source_type, source_id)`、`event_id`。流水应保存规则版本和奖励发生时的原因，以便历史审计。

### 8.3 `growth_events`

用途：事件接收和幂等处理。

字段建议：

- `event_id UUID PRIMARY KEY`；
- `event_type VARCHAR(100) NOT NULL`；
- `actor_user_id`；
- `subject_type VARCHAR(64)`；
- `subject_id VARCHAR(128)`；
- `source_module VARCHAR(100) NOT NULL`；
- `occurred_at TIMESTAMPTZ NOT NULL`；
- `processed_at TIMESTAMPTZ`；
- `status VARCHAR(20) NOT NULL`：`pending`、`processed`、`failed`；
- `payload JSONB NOT NULL DEFAULT '{}'`；
- `error_message TEXT`；
- `created_at TIMESTAMPTZ NOT NULL`。

如果使用事务型 Outbox，应在业务模块的业务事务内写入 Outbox，再由 worker 投递；消费端使用 Inbox/`growth_events` 去重。不要依赖“先提交业务，再尽力调用成长 API”的非事务方式。

### 8.4 `growth_rules`

字段建议：

- `id UUID PRIMARY KEY`；
- `code VARCHAR(100) NOT NULL`；
- `version INT NOT NULL`；
- `name VARCHAR(150) NOT NULL`；
- `event_type VARCHAR(100) NOT NULL`；
- `xp_amount INT NOT NULL`；
- `daily_limit INT`；
- `per_subject_limit INT`；
- `enabled BOOLEAN NOT NULL DEFAULT TRUE`；
- `effective_from TIMESTAMPTZ`；
- `effective_until TIMESTAMPTZ`；
- `created_by`、`updated_by`；
- `created_at`、`updated_at`。

唯一约束：`UNIQUE (code, version)`。已生效版本不得原地修改历史含义；后台编辑应创建新版本或保留变更快照。

### 8.5 `growth_levels`

字段建议：

- `level INT PRIMARY KEY`；
- `threshold_xp BIGINT NOT NULL UNIQUE`；
- `title VARCHAR(100) NOT NULL`；
- `enabled BOOLEAN NOT NULL DEFAULT TRUE`；
- `created_at`、`updated_at`。

校验阈值非负，启用等级的阈值必须严格递增，且必须存在 Lv.1、0 XP 的基础记录。

### 8.6 `daily_task_definitions`

字段建议：

- `id UUID PRIMARY KEY`；
- `code VARCHAR(100) NOT NULL UNIQUE`；
- `name VARCHAR(150) NOT NULL`；
- `description TEXT NOT NULL`；
- `event_type VARCHAR(100) NOT NULL`；
- `target_count INT NOT NULL`；
- `reward_xp INT NOT NULL DEFAULT 0`；
- `enabled BOOLEAN NOT NULL DEFAULT TRUE`；
- `sort_order INT NOT NULL DEFAULT 0`；
- `version INT NOT NULL DEFAULT 1`；
- `created_at`、`updated_at`。

### 8.7 `user_daily_tasks`

字段建议：

- `id UUID PRIMARY KEY`；
- `user_id`；
- `task_code VARCHAR(100) NOT NULL`；
- `task_date DATE NOT NULL`；
- `progress_count INT NOT NULL DEFAULT 0`；
- `target_count_snapshot INT NOT NULL`；
- `reward_xp_snapshot INT NOT NULL`；
- `status VARCHAR(20) NOT NULL`；
- `completed_at TIMESTAMPTZ`；
- `claimed_at TIMESTAMPTZ`；
- `definition_version INT NOT NULL`；
- `created_at`、`updated_at`。

唯一约束：`UNIQUE (user_id, task_code, task_date)`。进度不应由客户端写入。`target_count_snapshot`、`reward_xp_snapshot` 用于确保当天任务定义调整后，已开始任务的目标和奖励保持一致。

### 8.8 `achievement_definitions`

字段建议：

- `id UUID PRIMARY KEY`；
- `code VARCHAR(100) NOT NULL UNIQUE`；
- `name VARCHAR(150) NOT NULL`；
- `description TEXT NOT NULL`；
- `category VARCHAR(50) NOT NULL`；
- `rarity VARCHAR(30) NOT NULL DEFAULT 'common'`；
- `icon_url TEXT`；
- `condition_type VARCHAR(100) NOT NULL`；
- `target_value BIGINT NOT NULL DEFAULT 1`；
- `reward_xp INT NOT NULL DEFAULT 0`；
- `is_hidden BOOLEAN NOT NULL DEFAULT FALSE`；
- `enabled BOOLEAN NOT NULL DEFAULT TRUE`；
- `version INT NOT NULL DEFAULT 1`；
- `sort_order INT NOT NULL DEFAULT 0`；
- `created_at`、`updated_at`。

`condition_type` 必须是后端允许列表中的类型，不允许存储任意代码执行条件。

### 8.9 `user_achievements`

字段建议：

- `id UUID PRIMARY KEY`；
- `user_id`；
- `achievement_id`；
- `achievement_code_snapshot VARCHAR(100) NOT NULL`；
- `name_snapshot VARCHAR(150) NOT NULL`；
- `description_snapshot TEXT NOT NULL`；
- `definition_version INT NOT NULL`；
- `unlocked_at TIMESTAMPTZ NOT NULL`；
- `source_event_id UUID`；
- `is_public BOOLEAN NOT NULL DEFAULT TRUE`。

唯一约束：`UNIQUE (user_id, achievement_id)`。

### 8.10 `achievement_progress`

字段建议：

- `id UUID PRIMARY KEY`；
- `user_id`；
- `achievement_id`；
- `progress_count BIGINT NOT NULL DEFAULT 0`；
- `target_count BIGINT NOT NULL`；
- `updated_at TIMESTAMPTZ NOT NULL`。

唯一约束：`UNIQUE (user_id, achievement_id)`。对于可以从业务表可靠聚合的成就，缓存可以重建；缓存不得作为唯一审计依据。

### 8.11 `growth_audit_logs`

字段建议：

- `id UUID PRIMARY KEY`；
- `operator_user_id`；
- `action VARCHAR(100) NOT NULL`；
- `target_user_id`；
- `entity_type VARCHAR(100) NOT NULL`；
- `entity_id VARCHAR(128)`；
- `reason TEXT NOT NULL`；
- `before_data JSONB`；
- `after_data JSONB`；
- `request_id VARCHAR(128)`；
- `created_at TIMESTAMPTZ NOT NULL`。

对人工调整经验、授予特殊成就、规则修改、历史补算、冲正等操作写审计记录。

---

## 9. API 设计

遵循项目现有 API 约定：业务查询和操作使用 POST JSON；成功响应 `{ "data": ..., "meta": ... }`，失败响应 `{ "error": { "code": "...", "message": "...", "details": ... }, "request_id": "..." }`。具体路径前缀应与仓库现有路由保持一致。

### 9.1 用户端 API

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/growth/overview` | 当前经验、等级、升级进度、成长统计 |
| POST | `/api/growth/ledger/list` | 本人经验流水，分页 |
| POST | `/api/growth/daily-tasks/list` | 当日任务、目标、进度、状态与奖励 |
| POST | `/api/growth/daily-tasks/history` | 查询历史每日任务完成情况 |
| POST | `/api/achievements/list` | 成就目录、分类、公开条件 |
| POST | `/api/achievements/mine` | 本人已解锁成就 |
| POST | `/api/achievements/progress` | 本人可见成就进度 |
| POST | `/api/public/users/profile` | 公开用户等级及资料 |
| POST | `/api/public/users/achievements` | 用户公开展示的成就 |

请求约束：

- 用户 ID 从认证上下文获取，不信任请求体里的用户 ID。
- 分页参数遵循项目统一规范：`page` 从 1 开始，默认 `page_size=20`，普通接口最大 50。
- 成长接口只读；不得存在普通用户可调用的“加经验”“完成任务”“解锁成就”接口。

### 9.2 业务事件接入

不建议将内部成长事件 API 暴露给互联网用户。由 Go 服务内部接口或事件总线调用，例如：

- `GrowthService.HandleEvent(ctx, event)`
- `GrowthService.AwardXP(ctx, command)`：仅内部使用，必须要求合法事件和幂等键
- `AchievementService.Evaluate(ctx, event)`
- `DailyTaskService.ApplyEvent(ctx, event)`

如果必须通过 HTTP 跨服务调用，使用内部网络、服务身份鉴权、签名和重放保护；不能仅依赖路径不公开。

### 9.3 管理端 API

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/api/admin/growth/overview` | 成长统计 |
| POST | `/api/admin/growth/rules/list` | 经验规则列表 |
| POST | `/api/admin/growth/rules/create` | 新建规则版本 |
| POST | `/api/admin/growth/rules/update` | 修改规则并记录版本 |
| POST | `/api/admin/growth/levels/list` | 等级配置 |
| POST | `/api/admin/growth/levels/update` | 调整等级阈值 |
| POST | `/api/admin/growth/ledger/list` | 全局经验流水查询 |
| POST | `/api/admin/growth/adjust` | 人工增减经验 |
| POST | `/api/admin/growth/reverse` | 冲正指定流水 |
| POST | `/api/admin/daily-tasks/list` | 每日任务定义列表 |
| POST | `/api/admin/daily-tasks/create` | 创建任务 |
| POST | `/api/admin/daily-tasks/update` | 修改任务并版本化 |
| POST | `/api/admin/daily-tasks/toggle` | 启停任务 |
| POST | `/api/admin/achievements/list` | 成就列表 |
| POST | `/api/admin/achievements/create` | 创建成就 |
| POST | `/api/admin/achievements/update` | 更新成就定义 |
| POST | `/api/admin/achievements/toggle` | 启停成就 |
| POST | `/api/admin/achievements/recalculate` | 授权执行历史补算 |
| POST | `/api/admin/growth/audit/list` | 查询成长审计日志 |

人工调整请求必须包含：

- `user_id`；
- `delta_xp`；
- `reason`，非空且设置合理长度上限；
- `idempotency_key` 或由服务端生成的操作请求唯一 ID。

人工调整不得直接覆盖 `current_xp`，必须写入账本并更新投影。冲正请求必须关联原流水 ID，不能冲正不存在的流水，也不能对同一流水重复冲正超过原奖励金额。

### 9.4 统一响应示例

成长概览：

```json
{
  "data": {
    "current_xp": 180,
    "lifetime_earned_xp": 180,
    "level": 2,
    "level_title": "方块学徒",
    "next_level_xp": 300,
    "xp_to_next_level": 120,
    "level_progress_percent": 45,
    "unlocked_achievements": 4,
    "today_tasks_completed": 2,
    "today_tasks_total": 5
  },
  "meta": {}
}
```

每日任务列表：

```json
{
  "data": [
    {
      "code": "daily_like_3",
      "name": "点赞 3 次",
      "progress": 2,
      "target": 3,
      "reward_xp": 3,
      "status": "in_progress",
      "completed_at": null
    }
  ],
  "meta": {
    "task_date": "2026-10-19",
    "timezone": "Asia/Shanghai"
  }
}
```

---

## 10. 后台页面要求

在现有 `/admin` 左侧菜单增加一级菜单“用户成长”。

### 10.1 成长概览

展示：

- 当前活跃成长用户数；
- 今日发放经验、近 7 日发放经验；
- 各等级用户分布；
- 每日任务参与人数、完成率；
- 成就解锁数和解锁率；
- 点赞与留言奖励统计；
- 冲正、人工调整和异常事件统计。

统计指标必须注明统计口径和时间范围，不允许将事件数量直接等同于奖励笔数。

### 10.2 每日任务管理

支持：

- 创建、编辑、启用、停用；
- 配置名称、描述、事件类型、目标次数、奖励经验、排序；
- 预览规则变更影响；
- 查看任务完成率和发奖总量；
- 修改任务后保留已生成的当日任务快照；
- 不允许通过编辑任务将已有完成记录重复发奖。

### 10.3 成就管理

支持：

- 分类、名称、描述、图标、稀有度、隐藏状态；
- 条件类型与目标值；
- 启停、版本、排序、奖励经验；
- 预览条件说明；
- 历史补算；
- 查看解锁数量与解锁记录。

图标只允许受控的 HTTPS URL 或项目本地静态资源路径；不得允许任意脚本、SVG 脚本注入或危险 URL。

### 10.4 经验规则管理

支持：

- 经验数值、每日上限、单对象上限；
- 规则启停与版本；
- 生效时间；
- 变更前后对比；
- 变更原因；
- 预览每日按典型行为可能获得的经验。

### 10.5 经验流水与用户详情

支持按以下字段过滤：

- 用户 ID / 用户名；
- 经验来源；
- 关联业务 ID；
- 时间范围；
- 正向奖励 / 冲正 / 人工调整；
- 操作者；
- 幂等键。

所有人工调整、冲正、特殊成就授予和历史补算均需二次确认，并填写原因。高风险操作只能由 `admin` 执行。

---

## 11. 前台页面要求

### 11.1 成长中心

展示：

- 用户头像、昵称、等级和称号；
- 当前有效经验；
- 下一级所需经验和进度条；
- 今日任务列表与进度；
- 今日已获得经验；
- 最近经验流水；
- 最近解锁成就；
- 成就总数与完成比例。

### 11.2 每日任务区域

每个任务显示：

- 任务名称与简短说明；
- 当前进度 / 目标；
- 奖励经验；
- 状态：进行中、已完成；
- 完成时间（已完成时）。

不提供“领取经验”按钮，因为首期采用自动发奖。前端显示完成状态之前必须以服务端返回结果为准。

### 11.3 社区交互

帖子卡片应正确显示点赞状态与留言数量。点赞或留言成功后刷新对应业务状态；成长数据可在下一次查询或业务成功响应后更新，但不能以动画播放成功替代服务端成功。

不建议每次点赞都弹出“获得经验”提示。可以只在经验确实入账时轻量提示，避免社区交互被奖励提示打断。

### 11.4 公开个人主页

可公开展示：

- 等级与称号；
- 用户选择公开的成就；
- 已发布资源与社区贡献摘要。

默认不公开：

- 完整经验流水；
- 登录时间；
- 风险标记；
- 管理员备注；
- 被拒绝的投稿详情。

---

## 12. Go 服务端实现要求

建议模块结构如下，具体目录遵循现有仓库布局：

```text
backend/internal/growth/
  model.go
  repository.go
  service.go
  event.go
  event_handler.go
  xp_ledger.go
  level_service.go
  daily_task_service.go
  achievement_service.go
  rule_service.go
  audit_service.go
  handler.go
  dto.go
  errors.go
```

### 12.1 事务边界

处理一个有效业务事件时，应在事务中完成：

1. 检查并登记事件幂等键；
2. 锁定或安全更新对应用户成长记录；
3. 按当前有效规则计算经验；
4. 插入经验流水；
5. 更新 `user_growth` 投影；
6. 更新每日任务进度；
7. 检查并记录成就解锁；
8. 为任务或成就奖励生成独立幂等键并写入经验流水；
9. 更新最终等级；
10. 写入必要的审计记录。

任一步骤失败时，应整体回滚，不能出现“任务已完成但奖励未记账”或“经验已增加但流水缺失”。

如果事件跨服务或无法在同一数据库事务中完成，采用事务型 Outbox + 幂等消费者。事件消费者必须允许至少一次投递，并保证重复消费结果一致。

### 12.2 并发控制

- 对 `user_growth` 更新使用行锁或可靠的乐观锁。
- 每日任务进度更新必须原子化，避免并发事件丢失计数。
- 唯一约束是最后一道防线，不能只依赖应用层先查再插入。
- 遇到唯一约束冲突时，应按幂等成功处理，不重复发奖。
- 事务重试必须有限次数，并只重试可安全重放的操作。

### 12.3 经验发放伪代码

```text
HandleGrowthEvent(event):
  validateInternalEvent(event)

  begin transaction

  if event_id already processed:
    return idempotent success

  register event

  rules = loadActiveRules(event.type, event.occurred_at)

  for each matching rule:
    key = buildDeterministicRewardKey(event, rule)
    if ledger already contains key:
      continue
    if daily/per-subject limit reached:
      continue
    insert XP ledger entry
    update user growth projection

  update daily task progress atomically
  for each newly completed task:
    insert task reward ledger entry using unique task reward key

  update achievement progress
  for each newly unlocked achievement:
    insert user achievement using unique constraint
    if achievement reward > 0:
      insert achievement reward ledger entry using unique key

  recalculate level from current_xp
  mark event processed

  commit transaction
```
### 12.4 等级与统计重建

- `user_growth` 是可重建的投影，经验账本是经验变更的审计依据。
- 必须提供内部维护命令或受控管理任务，对比账本合计与投影值。
- 修复任务先生成差异报告，再执行修复；执行结果写审计记录。
- 不允许在普通 API 请求中全量扫描用户流水重算。
- 历史补算必须有任务 ID、操作者、开始/结束时间、处理数量、失败数量和幂等机制。

---

## 13. 数据迁移与上线策略

1. 新增迁移文件，不修改已经发布的历史迁移。
2. 建表、索引、唯一约束均纳入数据库迁移。
3. 初始化默认等级、经验规则、每日任务与成就定义，初始化操作必须可重复执行。
4. 先部署数据库与只读 API，再部署事件处理。
5. 事件接入按模块逐步启用：登录 → 点赞/留言 → 工具 → 收藏 → 投稿审核 → 纠错/百科。
6. 初期可通过配置关闭具体经验规则，但不能关闭幂等、流水和审计。
7. 对接已有历史数据时，默认不自动补发历史经验；如需补算，必须单独审批并运行幂等补算任务。
8. 上线前对规则奖励做模拟，确认普通用户合理使用一周的经验量不会让等级曲线失去意义。
9. 上线后监控奖励总量、单用户奖励分布、每日任务完成率和异常冲正量。

---

## 14. 自动化测试与验收标准

### 14.1 经验账本

- [ ] 同一业务事件重复处理 10 次，只产生一笔相同规则奖励。
- [ ] 两个并发请求处理同一事件，不重复发奖。
- [ ] 事务中途失败，流水、用户经验、任务进度、成就记录全部回滚。
- [ ] 人工调整必须有原因并记录审计。
- [ ] 冲正不删除原始流水，且同一奖励不能被重复冲正。
- [ ] `user_growth.current_xp` 与账本有效流水总和一致。

### 14.2 每日登录

- [ ] 首次成功登录完成每日登录任务并自动发奖。
- [ ] 刷新页面、查询用户资料、续期会话不重复奖励。
- [ ] 同一天切换登录方式不重复奖励。
- [ ] 跨越站点业务时区的日期边界后，任务进入新日期记录。

### 14.3 点赞

- [ ] 首次有效点赞可推进每日点赞任务。
- [ ] 重复点赞请求不重复推进进度或发奖。
- [ ] 取消后重新点赞不能重复获得同一帖子点赞经验。
- [ ] 自己给自己的帖子点赞不产生奖励。
- [ ] 无权点赞、已删除帖子、失败请求不产生成长事件。
- [ ] 并发点赞请求只产生一个有效点赞关系和一次奖励。

### 14.4 留言

- [ ] 空留言、被拒绝的留言不计任务。
- [ ] 同一帖子每天多条留言不突破该任务的单帖计数限制。
- [ ] 重复内容不能无限获得经验。
- [ ] 删除后重发相同内容不能绕过幂等与每日上限。
- [ ] 垃圾留言被确认后按策略冲正，原始流水保留。

### 14.5 成就与任务

- [ ] 每日任务达到目标时只奖励一次。
- [ ] 当日任务定义更新后，已生成任务快照保持一致。
- [ ] 成就条件达到时只解锁一次。
- [ ] 已解锁成就不因取消收藏或资源下架自动消失。
- [ ] 隐藏成就未解锁时不返回敏感条件。
- [ ] 历史补算重复执行不会重复发奖或解锁。

### 14.6 权限与安全

- [ ] 普通用户无法调用管理端成长 API。
- [ ] `editor` 无权执行仅限 `admin` 的经验调整、冲正和历史补算。
- [ ] 客户端伪造用户 ID、经验值、任务状态均无效。
- [ ] 所有列表 API 均有分页上限。
- [ ] 规则编辑、人工调整、冲正、历史补算均写审计记录。

### 14.7 前端

- [ ] 成长中心正确展示经验、等级、进度和任务状态。
- [ ] 点赞/留言失败时不显示成功奖励。
- [ ] 移动端布局正常。
- [ ] 隐藏成就不会泄露条件。
- [ ] 用户公开设置不会改变实际成长状态。

---

## 15. 开发任务清单（TODO）

### P0：基础架构与规则

- [ ] 阅读现有用户、帖子、评论、点赞、收藏、投稿、审核和认证模型，确认真实表名与事务边界。
- [ ] 确认站点业务时区配置及日期边界策略。
- [ ] 编写数据库迁移与默认数据初始化。
- [ ] 实现经验账本和幂等键。
- [ ] 实现成长事件接收、事务处理与事件审计。
- [ ] 实现 `user_growth` 投影和等级计算。
- [ ] 实现账本与投影一致性检查。

### P1：每日任务与社区行为

- [ ] 接入成功登录事件。
- [ ] 接入点赞成功/取消点赞事件。
- [ ] 接入有效留言创建/删除事件。
- [ ] 实现每日登录任务。
- [ ] 实现每日点赞任务。
- [ ] 实现每日留言任务。
- [ ] 实现每日工具使用任务。
- [ ] 实现每日收藏任务。
- [ ] 实现每日任务进度、自动发奖与历史查询。
- [ ] 为点赞、留言补充重复行为检测和速率限制。

### P2：长期成就与其他业务

- [ ] 实现成就定义、进度、解锁记录。
- [ ] 实现探索、收藏、贡献、社区、成长、特殊分类。
- [ ] 接入工具、收藏、资源投稿、审核通过、纠错与百科事件。
- [ ] 实现隐藏成就与公开展示设置。
- [ ] 实现受控历史补算。

### P3：后台和前台

- [ ] 开发成长概览。
- [ ] 开发每日任务管理。
- [ ] 开发经验规则与等级配置。
- [ ] 开发成就管理。
- [ ] 开发经验流水查询。
- [ ] 开发用户成长详情。
- [ ] 开发成长中心与每日任务 UI。
- [ ] 将等级与公开成就接入个人主页。
- [ ] 完成移动端适配。

### P4：安全、测试与上线

- [ ] 完成重复事件与并发测试。
- [ ] 完成点赞取消重做、留言删除重发等防刷测试。
- [ ] 完成权限测试和审计测试。
- [ ] 完成历史数据差异检查工具。
- [ ] 完成奖励模拟与经验曲线评估。
- [ ] 完成部署、回滚与故障排查文档。
- [ ] 通过全部验收项后启用生产奖励规则。

---

## 16. 实施前必须核对的仓库事项

此文档定义目标行为，但开发前必须检查仓库实际代码，禁止猜测现有表名、路由或模型：

1. 现有用户 ID 类型、认证上下文与角色权限中间件。
2. 现有帖子、评论、点赞、收藏关系的模型、唯一约束与删除策略。
3. 点赞和评论接口是否已有幂等、速率限制及事务。
4. 资源投稿状态流转和审核权限。
5. 现有 API 响应封装、错误码、分页规范。
6. GORM 迁移策略、UUID 生成方式、数据库事务约定。
7. 后台菜单与权限配置方式。
8. 邮箱验证是否已实现；未实现时不应虚构事件来源。
9. 现有测试框架、集成测试数据库与迁移测试方法。

如果现有代码与本规格不一致，应先记录差异并更新实施计划；不得为了套用本文档而破坏已有 API 契约或数据库数据。

---

## 17. 最终完成定义（Definition of Done）

满足以下所有条件才视为完成：

- 数据库迁移可在空库和现有库安全执行；
- 默认规则初始化可重复执行；
- 经验只通过服务端已验证的事件发放；
- 每日登录、点赞、留言任务端到端可用；
- 重复请求、并发事件和取消/删除后重做不会重复领奖；
- 经验账本、成长投影、任务和成就在事务失败时保持一致；
- 后台规则、流水和人工操作有权限控制与审计；
- 前台成长中心、每日任务、等级和成就正常展示；
- 自动化测试覆盖本文件第 14 节；
- 所有 P0、P1 任务和必要的 P2/P3 任务完成；
- 文档中的仓库实际差异已记录，部署与回滚步骤已验证。

**开发优先级结论：先完成 P0 的账本、事件幂等和事务，再接入每日登录、点赞、留言任务。不要先做前端展示或直接在点赞/评论 handler 中散落经验加减逻辑。**
'''
- 每日任务和经验奖励统一管理：每日登录、每日点赞、每日留言通过任务进度判定，不能再额外绕过经验账本发奖。
- 有效行为才计数：重复点赞、取消后重新点赞、删除后重发留言，不能无限刷经验。
- 成就和任务分开：每日任务可以每天重置；成就记录长期成长，不随每日任务重置。
- 所有奖励服务端发放：前端只能查询任务、上报允许的业务行为，不能直接提交经验值或伪造任务完成状态。