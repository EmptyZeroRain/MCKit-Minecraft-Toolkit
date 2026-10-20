// ===== 用户成长系统类型 =====
// 契约来源：backend/internal/http/handler/growth.go 与 internal/service/growth。
// 所有经验 / 等级 / 任务 / 成就状态均由服务端判定，前端仅负责展示。

/** 成长概览（用户端，jingyan.md §11.1） */
export interface GrowthOverview {
  current_xp: number
  lifetime_earned_xp: number
  lifetime_reversed_xp: number
  level: number
  level_title: string
  /** 下一级所需经验阈值；满级时为 null */
  next_level_xp: number | null
  xp_to_next_level: number
  level_progress_percent: number
  is_max_level: boolean
  unlocked_achievements: number
  achievement_total: number
  today_tasks_completed: number
  today_tasks_total: number
  today_xp: number
}

/** 经验流水条目（只增不删，冲正以负向流水表达） */
export interface GrowthLedgerItem {
  id: string
  user_id: string
  delta_xp: number
  balance_after: number
  /** rule / daily_task / achievement / manual / reversal */
  source_type: string
  source_id: string
  rule_code: string
  reason: string
  created_at: string
}

/** 每日任务视图（定义 + 当天进度合并） */
export interface GrowthDailyTask {
  code: string
  name: string
  description: string
  progress: number
  target: number
  reward_xp: number
  /** in_progress / completed / claimed */
  status: string
  completed_at: string
}

/** 每日任务列表响应（含业务日期与时区） */
export interface GrowthDailyTasks {
  date: string
  timezone: string
  tasks: GrowthDailyTask[]
}

/** 成就目录项（含本人解锁状态与进度；隐藏成就未解锁时仅占位） */
export interface GrowthAchievement {
  code: string
  name: string
  description: string
  category: string
  rarity: string
  icon_url: string
  is_hidden: boolean
  unlocked: boolean
  unlocked_at?: string
  progress: number
  target: number
  reward_xp: number
}

/** 用户每日任务历史记录（含完成时快照，不随定义变更改写） */
export interface GrowthDailyTaskRecord {
  id: string
  user_id: string
  task_code: string
  task_date: string
  progress_count: number
  target_count: number
  reward_xp: number
  status: string
  completed_at: string | null
  claimed_at: string | null
  definition_version: number
  created_at: string
  updated_at: string
}

/** 用户已解锁成就快照 */
export interface GrowthUserAchievement {
  id: string
  user_id: string
  achievement_id: string
  code: string
  name: string
  description: string
  definition_version: number
  unlocked_at: string
  source_event_id: string | null
  is_public: boolean
}

/** 公开个人主页成长摘要（不含经验流水与登录信息） */
export interface GrowthPublicProfile {
  user_id: string
  user_no: string
  username: string
  nickname: string
  avatar_url: string
  level: number
  level_title: string
  current_xp: number
  unlocked_achievements: number
}

// ===== 后台管理端 =====

/** 单等级用户分布桶 */
export interface GrowthLevelBucket {
  level: number
  count: number
}

/** 后台成长概览（口径见后端 repo.Stats 注释） */
export interface AdminGrowthOverview {
  timezone: string
  date: string
  /** 有成长记录的用户数 */
  active_users: number
  /** 今日正向发放经验合计 */
  xp_issued_today: number
  /** 近 7 日正向发放经验合计 */
  xp_issued_7days: number
  /** 今日奖励笔数（正向流水） */
  ledger_today: number
  /** 累计冲正经验（绝对值） */
  reversed_total: number
  /** 累计人工调整笔数 */
  manual_adjust_count: number
  /** 累计成就解锁数 */
  unlocked_total: number
  /** 启用成就总数 */
  achievement_count: number
  /** 今日完成任务数 */
  tasks_completed_today: number
  /** 今日任务参与人数 */
  task_participants_today: number
  level_distribution: GrowthLevelBucket[]
  total_rules: number
  total_tasks: number
}

/** 经验规则（版本化：修改创建新版本，不原地改写历史含义） */
export interface GrowthRule {
  id: string
  code: string
  version: number
  name: string
  event_type: string
  xp_amount: number
  daily_limit: number | null
  per_subject_limit: number | null
  enabled: boolean
  effective_from: string | null
  effective_until: string | null
  created_by: string | null
  updated_by: string | null
  created_at: string
  updated_at: string
}

/** 经验规则创建 / 更新入参 */
export interface GrowthRuleInput {
  code?: string
  name: string
  event_type: string
  xp_amount: number
  daily_limit: number | null
  per_subject_limit: number | null
  enabled: boolean
  effective_from?: string | null
  effective_until?: string | null
}

/** 等级阈值 */
export interface GrowthLevel {
  level: number
  threshold_xp: number
  title: string
  enabled: boolean
  created_at?: string
  updated_at?: string
}

/** 新增等级入参（可指定等级号并跳号；省略 level 时由服务端追加到最高级之后） */
export interface GrowthLevelCreateInput {
  level?: number
  threshold_xp: number
  title: string
  enabled: boolean
}

/** 每日任务定义 */
export interface GrowthTaskDefinition {
  id: string
  code: string
  name: string
  description: string
  event_type: string
  target_count: number
  reward_xp: number
  enabled: boolean
  sort_order: number
  version: number
  created_at: string
  updated_at: string
}

/** 每日任务定义创建 / 更新入参 */
export interface GrowthTaskInput {
  code?: string
  name: string
  description: string
  event_type: string
  target_count: number
  reward_xp: number
  enabled: boolean
  sort_order: number
}

/** 成就定义 */
export interface GrowthAchievementDefinition {
  id: string
  code: string
  name: string
  description: string
  category: string
  rarity: string
  icon_url: string
  /** event_count / distinct_subject_count / level_reached / daily_task_days */
  condition_type: string
  condition_event_type: string
  condition_subject_type: string
  target_value: number
  reward_xp: number
  is_hidden: boolean
  enabled: boolean
  version: number
  sort_order: number
  created_at: string
  updated_at: string
}

/** 成就定义创建 / 更新入参 */
export interface GrowthAchievementInput {
  code?: string
  name: string
  description: string
  category: string
  rarity: string
  icon_url: string
  condition_type: string
  condition_event_type: string
  condition_subject_type: string
  target_value: number
  reward_xp: number
  is_hidden: boolean
  enabled: boolean
  sort_order: number
}

/** 成长审计日志 */
export interface GrowthAuditLog {
  id: string
  operator_user_id: string | null
  action: string
  target_user_id: string | null
  entity_type: string
  entity_id: string
  reason: string
  before_data?: unknown
  after_data?: unknown
  request_id: string
  created_at: string
}

/** 全局经验流水筛选（后台） */
export interface GrowthLedgerFilter {
  user_id?: string
  rule_code?: string
  source_type?: string
  page?: number
  page_size?: number
}

/** 成长审计筛选（后台） */
export interface GrowthAuditFilter {
  operator_id?: string
  target_user_id?: string
  action?: string
  page?: number
  page_size?: number
}

/** 人工调整入参 */
export interface AdjustXPInput {
  target_user_id: string
  delta_xp: number
  reason: string
  /** 客户端幂等键；相同键重复提交不会重复调整 */
  request_key?: string
}

// ===== 枚举与展示标签 =====
// 与后端 domain 常量保持一致；后台配置只允许引用这些合法值。

/** 合法成长事件类型（domain.GrowthEventTypes） */
export const GROWTH_EVENT_TYPES = [
  'user.registered',
  'user.profile_completed',
  'user.email_verified',
  'user.login_succeeded',
  'tool.used_validly',
  'resource.favorited',
  'resource.submission_created',
  'resource.approved',
  'resource.broken_report_confirmed',
  'feedback.correction_accepted',
  'post.liked',
  'post.unliked',
  'post.comment_created',
  'post.comment_deleted',
  'wiki.contribution_approved',
] as const

/** 事件类型中文标签 */
export const GROWTH_EVENT_LABELS: Record<string, string> = {
  'user.registered': '用户注册',
  'user.profile_completed': '完善个人资料',
  'user.email_verified': '验证邮箱',
  'user.login_succeeded': '成功登录',
  'tool.used_validly': '有效使用工具',
  'resource.favorited': '收藏资源',
  'resource.submission_created': '提交资源投稿',
  'resource.approved': '资源审核通过',
  'resource.broken_report_confirmed': '确认失效资源',
  'feedback.correction_accepted': '有效纠错',
  'post.liked': '有效点赞',
  'post.unliked': '取消点赞',
  'post.comment_created': '发表留言',
  'post.comment_deleted': '删除留言',
  'wiki.contribution_approved': '百科贡献通过',
}

/** 成就条件类型（domain.AchievementConditionTypes） */
export const ACHIEVEMENT_CONDITION_TYPES = [
  'event_count',
  'distinct_subject_count',
  'level_reached',
  'daily_task_days',
] as const

/** 成就条件类型中文标签 */
export const ACHIEVEMENT_CONDITION_LABELS: Record<string, string> = {
  event_count: '事件累计次数',
  distinct_subject_count: '去重目标数',
  level_reached: '达到等级',
  daily_task_days: '完成任务的累计天数',
}

/** 经验流水来源类型中文标签 */
export const LEDGER_SOURCE_LABELS: Record<string, string> = {
  rule: '业务奖励',
  daily_task: '每日任务',
  achievement: '成就奖励',
  manual: '人工调整',
  reversal: '冲正',
}

/** 每日任务状态中文标签 */
export const DAILY_TASK_STATUS_LABELS: Record<string, string> = {
  in_progress: '进行中',
  completed: '已完成',
  claimed: '已完成',
}

/** 成就稀有度中文标签 */
export const RARITY_LABELS: Record<string, string> = {
  common: '普通',
  rare: '稀有',
  epic: '史诗',
  legendary: '传说',
}

/** 单用户等级调整入参（delta_level 可负；不修改等级定义本身） */
export interface AdjustLevelInput {
  target_user_id: string
  delta_level: number
  reason: string
  request_key?: string
}

/** 单用户等级调整结果 */
export interface LevelAdjustResult {
  user_id: string
  base_level: number
  level_offset: number
  level: number
  level_title: string
  delta_level: number
}

/** 后台查看某用户成就的持有 / 撤销状态 */
export interface AdminUserAchievement {
  achievement_id: string
  code: string
  name: string
  category: string
  rarity: string
  enabled: boolean
  reward_xp: number
  unlocked: boolean
  revoked: boolean
  grant_source: string
  unlocked_at: string | null
  revoked_at: string | null
  revoke_reason: string
}

/** 后台成长用户摘要 */
export interface AdminGrowthUserSummary {
  user_id: string
  user_no: string
  username: string
  nickname: string
  role: string
  is_active: boolean
  level: number
  level_title: string
  current_xp: number
  lifetime_earned_xp: number
  lifetime_reversed_xp: number
  achievement_count: number
  gift_balance: number
  gift_general_balance: number
  gift_checkin_balance: number
  created_at: string
  updated_at: string | null
}

/** 后台用户详情中的已解锁成就 */
export interface AdminGrowthAchievement {
  id: string
  code: string
  name: string
  description: string
  category: string
  rarity: string
  reward_xp: number
  unlocked_at: string
  grant_source: string
  is_public: boolean
  definition_id: string
}

/** 后台用户详情中的礼物币流水 */
export interface AdminGiftLedgerItem {
  id: string
  amount: number
  coin_type: 'general' | 'checkin'
  balance_after: number
  event_type: string
  source_type: string
  source_id: string
  reason: string
  created_at: string
}

/** 后台用户详情中的成长审计日志 */
export interface AdminGrowthAuditItem {
  id: string
  action: string
  entity_type: string
  entity_id: string
  reason: string
  operator_user_id: string
  created_at: string
}

/** 后台单用户成长详情（树状展示数据源） */
export interface AdminGrowthUserDetail {
  user: AdminGrowthUserSummary
  base_level: number
  level_offset: number
  next_level_xp: number | null
  xp_to_next_level: number
  level_progress_percent: number
  is_max_level: boolean
  achievement_total: number
  experience_total: number
  gift_total: number
  audit_total: number
  experience: GrowthLedgerItem[]
  achievements: AdminGrowthAchievement[]
  gifts: AdminGiftLedgerItem[]
  audits: AdminGrowthAuditItem[]
}
