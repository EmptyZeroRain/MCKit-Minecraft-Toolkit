// ===== 普通用户账号与个人数据 API（需求 §12，全 POST）=====
// 契约来源：backend/internal/http/handler/user.go
import userHttp, { setUserCsrfToken } from './user-http'
import type { ApiResponse } from '@/types/api'
import type {
  LoginPayload,
  RecordRecentPayload,
  RegisterPayload,
  SaveSeedPayload,
  ToggleFavoritePayload,
  UserAccount,
  UserFavoriteItem,
  UserRecentItem,
  UserSeed,
} from '@/types/user'

/** 登录 / me 响应额外携带 CSRF token */
type UserAuthResponse = UserAccount & { csrf_token?: string }

/** 注册（不自动登录，注册后需登录） */
export async function register(payload: RegisterPayload): Promise<UserAccount> {
  const { data } = await userHttp.post<ApiResponse<UserAccount>>('/user/register', payload)
  return data.data
}

/** 登录（后端种下用户会话 Cookie，并返回 CSRF token） */
export async function login(payload: LoginPayload): Promise<UserAccount> {
  const { data } = await userHttp.post<ApiResponse<UserAuthResponse>>('/user/login', payload)
  if (data.data.csrf_token) setUserCsrfToken(data.data.csrf_token)
  return data.data
}

/** 退出登录 */
export async function logout(): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/logout', {})
  setUserCsrfToken('')
}

/** 当前用户信息（用于恢复会话） */
export async function me(): Promise<UserAccount> {
  const { data } = await userHttp.post<ApiResponse<UserAuthResponse>>('/user/me', {})
  if (data.data.csrf_token) setUserCsrfToken(data.data.csrf_token)
  return data.data
}

/** 修改密码（成功后全部会话失效） */
export async function changePassword(oldPassword: string, newPassword: string): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/change-password', {
    old_password: oldPassword,
    new_password: newPassword,
  })
  setUserCsrfToken('')
}

/** 删除账号及其全部个人数据（需密码二次确认） */
export async function deleteAccount(password: string): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/delete', { password })
  setUserCsrfToken('')
}

/** 收藏列表 */
export async function listFavorites(): Promise<UserFavoriteItem[]> {
  const { data } = await userHttp.post<ApiResponse<{ items: UserFavoriteItem[] }>>(
    '/user/favorites/list',
    {},
  )
  return data.data.items ?? []
}

/** 收藏 / 取消收藏，返回操作后的状态 */
export async function toggleFavorite(payload: ToggleFavoritePayload): Promise<boolean> {
  const { data } = await userHttp.post<ApiResponse<{ favorited: boolean }>>(
    '/user/favorites/toggle',
    payload,
  )
  return data.data.favorited
}

/** 取消收藏 */
export async function removeFavorite(kind: string, objectId: string): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/favorites/remove', {
    kind,
    object_id: objectId,
  })
}

/** 最近使用列表 */
export async function listRecents(): Promise<UserRecentItem[]> {
  const { data } = await userHttp.post<ApiResponse<{ items: UserRecentItem[] }>>(
    '/user/recents/list',
    {},
  )
  return data.data.items ?? []
}

/** 记录一次最近使用 */
export async function recordRecent(payload: RecordRecentPayload): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/recents/record', payload)
}

/** 清空最近使用 */
export async function clearRecents(): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/recents/clear', {})
}

/** 常用种子列表 */
export async function listSeeds(): Promise<UserSeed[]> {
  const { data } = await userHttp.post<ApiResponse<{ items: UserSeed[] }>>('/user/seeds/list', {})
  return data.data.items ?? []
}

/** 新建或更新常用种子 */
export async function saveSeed(payload: SaveSeedPayload): Promise<UserSeed> {
  const { data } = await userHttp.post<ApiResponse<UserSeed>>('/user/seeds/save', payload)
  return data.data
}

/** 删除常用种子 */
export async function deleteSeed(id: string): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/user/seeds/delete', { id })
}
