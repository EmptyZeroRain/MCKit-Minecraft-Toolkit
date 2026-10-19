// ===== 角色与权限 API（admin 专属，全 POST + 动作子路径）=====
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  CreateRolePayload,
  PermissionGroup,
  RoleRow,
  UpdateRolePayload,
} from '@/types/admin'

/** 角色列表（内置在前；角色总量小，不分页） */
export async function listRoles(): Promise<RoleRow[]> {
  const { data } = await http.post<ApiResponse<RoleRow[]>>('/admin/roles/list', {})
  return data.data ?? []
}

/** 权限点目录（分组 + 中文名） */
export async function listPermissions(): Promise<PermissionGroup[]> {
  const { data } = await http.post<ApiResponse<PermissionGroup[]>>('/admin/roles/permissions', {})
  return data.data ?? []
}

/** 新建自定义角色 */
export async function createRole(payload: CreateRolePayload): Promise<RoleRow> {
  const { data } = await http.post<ApiResponse<RoleRow>>('/admin/roles/create', payload)
  return data.data
}

/** 更新角色展示信息与权限 */
export async function updateRole(payload: UpdateRolePayload): Promise<RoleRow> {
  const { data } = await http.post<ApiResponse<RoleRow>>('/admin/roles/update', payload)
  return data.data
}

/** 删除自定义角色（内置角色与仍被账号引用的角色会被拒绝） */
export async function deleteRole(name: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/roles/delete', { name })
}

/** 角色权限变更的影响预览（只读） */
export interface RoleImpact {
  role: string
  label: string
  is_system: boolean
  affected_accounts: number
  current_permissions: string[]
  next_permissions: string[]
  added: string[]
  removed: string[]
  notes: string[]
}

/** 计算把角色权限调整为 permissions 后的影响 */
export async function getRoleImpact(name: string, permissions: string[]): Promise<RoleImpact> {
  const { data } = await http.post<ApiResponse<RoleImpact>>('/admin/roles/impact', {
    name,
    permissions,
  })
  return data.data
}
