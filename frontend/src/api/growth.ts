// ===== 用户成长系统 API（全 POST + 动作子路径）=====
// 契约来源：backend/internal/http/handler/growth.go
//
// 实例分离：用户端成长数据使用 userHttp（用户会话 + 用户 CSRF），
// 后台管理接口使用 http（管理员会话 + 管理员 CSRF）。
// 经验 / 等级 / 任务 / 成就一律由服务端判定，前端只提交业务动作与展示结果。
import http from './http'
import userHttp from './user-http'
import type { ApiResponse, ListResult, PageQuery } from '@/types/api'
import type {
  AdjustLevelInput,
  AdjustXPInput,
  AdminGrowthOverview,
  AdminGrowthUserDetail,
  AdminGrowthUserSummary,
  AdminUserAchievement,
  LevelAdjustResult,
  GrowthAchievement,
  GrowthAchievementDefinition,
  GrowthAchievementInput,
  GrowthAuditFilter,
  GrowthAuditLog,
  GrowthDailyTaskRecord,
  GrowthDailyTasks,
  GrowthLedgerFilter,
  GrowthLedgerItem,
  GrowthLevel,
  GrowthLevelCreateInput,
  GrowthOverview,
  GrowthPublicProfile,
  GrowthRule,
  GrowthRuleInput,
  GrowthTaskDefinition,
  GrowthTaskInput,
  GrowthUserAchievement,
} from '@/types/growth'

/** 将后端分页响应折叠为统一 ListResult */
function toListResult<T>(data: ApiResponse<T[]>): ListResult<T> {
  const meta = data.meta
  return {
    items: data.data ?? [],
    total: meta?.total ?? 0,
    page: meta?.page ?? 1,
    pageSize: meta?.page_size ?? 0,
    totalPages: meta?.total_pages ?? 0,
  }
}

// ---- 用户端（userHttp）----

/** 本人成长概览（经验 / 等级 / 今日任务 / 成就统计） */
export async function fetchGrowthOverview(): Promise<GrowthOverview> {
  const { data } = await userHttp.post<ApiResponse<GrowthOverview>>('/growth/overview', {})
  return data.data
}

/** 本人经验流水（分页，倒序） */
export async function fetchGrowthLedger(params: PageQuery = {}): Promise<ListResult<GrowthLedgerItem>> {
  const { data } = await userHttp.post<ApiResponse<GrowthLedgerItem[]>>('/growth/ledger', params)
  return toListResult(data)
}

/** 今日每日任务（含业务日期与时区） */
export async function fetchGrowthDailyTasks(): Promise<GrowthDailyTasks> {
  const { data } = await userHttp.post<ApiResponse<GrowthDailyTasks>>('/growth/daily-tasks', {})
  return data.data
}

/** 历史每日任务记录（分页） */
export async function fetchGrowthDailyTaskHistory(
  params: PageQuery = {},
): Promise<ListResult<GrowthDailyTaskRecord>> {
  const { data } = await userHttp.post<ApiResponse<GrowthDailyTaskRecord[]>>(
    '/growth/daily-tasks/history',
    params,
  )
  return toListResult(data)
}

/** 成就目录（含本人解锁状态与进度） */
export async function fetchGrowthAchievements(): Promise<GrowthAchievement[]> {
  const { data } = await userHttp.post<ApiResponse<{ items: GrowthAchievement[] }>>(
    '/growth/achievements',
    {},
  )
  return data.data.items ?? []
}

/** 公开个人主页成长摘要 */
export async function fetchPublicGrowthProfile(id: string): Promise<GrowthPublicProfile> {
  const { data } = await userHttp.post<ApiResponse<GrowthPublicProfile>>('/public/users/detail', {
    id,
  })
  return data.data
}

/** 公开个人主页成就列表 */
export async function fetchPublicAchievements(id: string): Promise<GrowthUserAchievement[]> {
  const { data } = await userHttp.post<ApiResponse<{ items: GrowthUserAchievement[] }>>(
    '/public/users/achievements',
    { id },
  )
  return data.data.items ?? []
}

// ---- 后台管理端（http）----

/** 后台成长概览（含等级分布与统计口径） */
export async function fetchAdminGrowthOverview(): Promise<AdminGrowthOverview> {
  const { data } = await http.post<ApiResponse<AdminGrowthOverview>>('/admin/growth/overview', {})
  return data.data
}

/** 后台成长用户列表（按用户名 / 昵称 / user_no 搜索） */
export async function fetchAdminGrowthUsers(
  params: { q?: string; page?: number; page_size?: number } = {},
): Promise<ListResult<AdminGrowthUserSummary>> {
  const { data } = await http.post<ApiResponse<AdminGrowthUserSummary[]>>(
    '/admin/growth/users/list',
    params,
  )
  return toListResult(data)
}

/** 后台单用户成长详情（等级 / 成就 / 经验 / 礼物 / 审计） */
export async function fetchAdminGrowthUserDetail(
  userId: string,
): Promise<AdminGrowthUserDetail> {
  const { data } = await http.post<ApiResponse<AdminGrowthUserDetail>>(
    '/admin/growth/users/detail',
    { user_id: userId },
  )
  return data.data
}

/** 全局经验流水（按用户 / 来源 / 规则过滤） */
export async function fetchAdminGrowthLedger(
  filter: GrowthLedgerFilter = {},
): Promise<ListResult<GrowthLedgerItem>> {
  const { data } = await http.post<ApiResponse<GrowthLedgerItem[]>>(
    '/admin/growth/ledger/list',
    filter,
  )
  return toListResult(data)
}

/** 人工调整经验（正负均可，需原因；高风险操作仅 admin） */
export async function adjustUserXP(payload: AdjustXPInput): Promise<GrowthLedgerItem> {
  const { data } = await http.post<ApiResponse<GrowthLedgerItem>>('/admin/growth/adjust', payload)
  return data.data
}

/** 冲正一笔经验流水（生成等额负向流水，原始流水保留） */
export async function reverseGrowthLedger(id: string, reason: string): Promise<GrowthLedgerItem> {
  const { data } = await http.post<ApiResponse<GrowthLedgerItem>>('/admin/growth/reverse', {
    id,
    reason,
  })
  return data.data
}

/** 成长审计日志（分页） */
export async function fetchGrowthAudit(
  filter: GrowthAuditFilter = {},
): Promise<ListResult<GrowthAuditLog>> {
  const { data } = await http.post<ApiResponse<GrowthAuditLog[]>>(
    '/admin/growth/audit/list',
    filter,
  )
  return toListResult(data)
}

/** 经验规则列表（版本降序） */
export async function fetchGrowthRules(): Promise<GrowthRule[]> {
  const { data } = await http.post<ApiResponse<{ items: GrowthRule[] }>>(
    '/admin/growth/rules/list',
    {},
  )
  return data.data.items ?? []
}

/** 新建经验规则（版本 1） */
export async function createGrowthRule(input: GrowthRuleInput): Promise<GrowthRule> {
  const { data } = await http.post<ApiResponse<GrowthRule>>('/admin/growth/rules/create', input)
  return data.data
}

/** 更新经验规则（生成新版本，历史版本保留） */
export async function updateGrowthRule(id: string, input: GrowthRuleInput): Promise<GrowthRule> {
  const { data } = await http.post<ApiResponse<GrowthRule>>('/admin/growth/rules/update', {
    id,
    ...input,
  })
  return data.data
}

/** 启用 / 停用某编码的全部规则版本 */
export async function toggleGrowthRule(id: string, enabled: boolean): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/growth/rules/toggle', { id, enabled })
}

/** 删除规则全部版本（已被账本引用时后端拒绝） */
export async function deleteGrowthRule(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/growth/rules/delete', { id })
}

/** 等级阈值列表 */
export async function fetchGrowthLevels(): Promise<GrowthLevel[]> {
  const { data } = await http.post<ApiResponse<{ items: GrowthLevel[] }>>(
    '/admin/growth/levels/list',
    {},
  )
  return data.data.items ?? []
}

/** 新增等级（可指定等级号并跳号；不传 level 时追加到当前最高级之后） */
export async function createGrowthLevel(input: GrowthLevelCreateInput): Promise<GrowthLevel> {
  const { data } = await http.post<ApiResponse<GrowthLevel>>('/admin/growth/levels/create', input)
  return data.data
}

/** 更新单个等级阈值 / 标题 / 启用状态 */
export async function updateGrowthLevel(level: GrowthLevel): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/growth/levels/update', level)
}

/** 删除任意非 Lv.1 等级（后端会重排后续等级并重算用户展示等级） */
export async function deleteGrowthLevel(level: number): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/growth/levels/delete', { level })
}

/** 依据当前阈值表重算所有用户等级，返回被修正的用户数 */
export async function recalculateGrowthLevels(): Promise<number> {
  const { data } = await http.post<ApiResponse<{ fixed: number }>>(
    '/admin/growth/levels/recalculate',
    {},
  )
  return data.data.fixed
}

/** 每日任务定义列表（含停用项） */
export async function fetchDailyTaskDefs(): Promise<GrowthTaskDefinition[]> {
  const { data } = await http.post<ApiResponse<{ items: GrowthTaskDefinition[] }>>(
    '/admin/daily-tasks/list',
    {},
  )
  return data.data.items ?? []
}

/** 新建每日任务定义 */
export async function createDailyTaskDef(input: GrowthTaskInput): Promise<GrowthTaskDefinition> {
  const { data } = await http.post<ApiResponse<GrowthTaskDefinition>>(
    '/admin/daily-tasks/create',
    input,
  )
  return data.data
}

/** 更新每日任务定义（原地更新并递增版本，已生成进度保留快照） */
export async function updateDailyTaskDef(
  id: string,
  input: GrowthTaskInput,
): Promise<GrowthTaskDefinition> {
  const { data } = await http.post<ApiResponse<GrowthTaskDefinition>>('/admin/daily-tasks/update', {
    id,
    ...input,
  })
  return data.data
}

/** 启用 / 停用每日任务定义 */
export async function toggleDailyTaskDef(id: string, enabled: boolean): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/daily-tasks/toggle', { id, enabled })
}

/** 删除每日任务定义（已产生用户进度时后端拒绝） */
export async function deleteDailyTaskDef(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/daily-tasks/delete', { id })
}

/** 成就定义列表（含停用项） */
export async function fetchAchievementDefs(): Promise<GrowthAchievementDefinition[]> {
  const { data } = await http.post<ApiResponse<{ items: GrowthAchievementDefinition[] }>>(
    '/admin/achievements/list',
    {},
  )
  return data.data.items ?? []
}

/** 新建成就定义 */
export async function createAchievementDef(
  input: GrowthAchievementInput,
): Promise<GrowthAchievementDefinition> {
  const { data } = await http.post<ApiResponse<GrowthAchievementDefinition>>(
    '/admin/achievements/create',
    input,
  )
  return data.data
}

/** 更新成就定义（原地更新并递增版本） */
export async function updateAchievementDef(
  id: string,
  input: GrowthAchievementInput,
): Promise<GrowthAchievementDefinition> {
  const { data } = await http.post<ApiResponse<GrowthAchievementDefinition>>(
    '/admin/achievements/update',
    { id, ...input },
  )
  return data.data
}

/** 启用 / 停用成就定义（停用不影响已解锁记录） */
export async function toggleAchievementDef(id: string, enabled: boolean): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/achievements/toggle', { id, enabled })
}

/** 历史补算成就：user_id 为空时对全部有成长记录的用户执行，返回新解锁数 */
export async function recalculateAchievements(userId = ''): Promise<number> {
  const { data } = await http.post<ApiResponse<{ unlocked: number }>>(
    '/admin/achievements/recalculate',
    { user_id: userId },
  )
  return data.data.unlocked
}

/** 单用户等级调整（只调整等级偏移，不修改等级定义本身） */
export async function adjustUserLevel(payload: AdjustLevelInput): Promise<LevelAdjustResult> {
  const { data } = await http.post<ApiResponse<LevelAdjustResult>>(
    '/admin/growth/levels/adjust',
    payload,
  )
  return data.data
}

/** 某用户全部成就的持有 / 撤销状态 */
export async function fetchAdminUserAchievements(
  targetUserId: string,
): Promise<AdminUserAchievement[]> {
  const { data } = await http.post<ApiResponse<{ items: AdminUserAchievement[] }>>(
    '/admin/achievements/user/list',
    { target_user_id: targetUserId },
  )
  return data.data.items ?? []
}

/** 手动授予成就（会按定义补发一次性奖励经验） */
export async function grantUserAchievement(
  targetUserId: string,
  achievementId: string,
  reason: string,
): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/achievements/grant', {
    target_user_id: targetUserId,
    achievement_id: achievementId,
    reason,
  })
}

/** 撤销成就（软删除，不回收已发经验；自动补算不会重新解锁） */
export async function revokeUserAchievement(
  targetUserId: string,
  achievementId: string,
  reason: string,
): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/achievements/revoke', {
    target_user_id: targetUserId,
    achievement_id: achievementId,
    reason,
  })
}
