// ===== 工具管理 API（需求 8.4，全 POST + 动作子路径）=====
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type { Tool, ToolListQuery, ToolPayload } from '@/types/tool'

export async function listTools(params: ToolListQuery = {}) {
  const { data } = await http.post<ApiResponse<Tool[]>>('/admin/tools/list', params)
  return toPaged({ data })
}

export async function getTool(id: string): Promise<Tool | null> {
  const { data } = await http.post<ApiResponse<Tool>>('/admin/tools/detail', { id })
  return data.data
}

export async function createTool(payload: ToolPayload): Promise<Tool> {
  const { data } = await http.post<ApiResponse<Tool>>('/admin/tools/create', payload)
  return data.data
}

export async function updateTool(id: string, payload: Partial<ToolPayload>): Promise<Tool> {
  const { data } = await http.post<ApiResponse<Tool>>('/admin/tools/update', { id, ...payload })
  return data.data
}

export async function deleteTool(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/tools/delete', { id })
}