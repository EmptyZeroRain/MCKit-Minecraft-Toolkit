// ===== 社区管理 API（后台，需 content.community 权限）=====
import http from './http'
import type { ApiResponse, ListResult } from '@/types/api'
import type { CommunityCategory, CommunityComment, CommunityPost } from '@/types/community'

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

export interface AdminCommunityListQuery {
  status?: 'published' | 'hidden' | ''
  category?: CommunityCategory | ''
  q?: string
  page?: number
  page_size?: number
}

/** 后台帖子列表（含隐藏内容） */
export async function adminListCommunityPosts(
  params: AdminCommunityListQuery = {},
): Promise<ListResult<CommunityPost>> {
  const { data } = await http.post<ApiResponse<CommunityPost[]>>(
    '/admin/community/posts/list',
    params,
  )
  return toListResult(data)
}

/** 隐藏 / 恢复帖子 */
export async function adminSetCommunityPostStatus(
  id: string,
  status: 'published' | 'hidden',
): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/community/posts/status', { id, status })
}

/** 删除帖子（级联留言与点赞） */
export async function adminDeleteCommunityPost(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/community/posts/delete', { id })
}

/** 查看某帖全部留言（含隐藏） */
export async function adminListCommunityComments(params: {
  post_id: string
  page?: number
  page_size?: number
}): Promise<ListResult<CommunityComment>> {
  const { data } = await http.post<ApiResponse<CommunityComment[]>>(
    '/admin/community/comments/list',
    params,
  )
  return toListResult(data)
}

export interface AdminCommentListQuery {
  status?: 'published' | 'hidden' | ''
  q?: string
  page?: number
  page_size?: number
}

/** 全站用户评论列表（跨帖子，含隐藏与所属帖子标题） */
export async function adminListAllCommunityComments(
  params: AdminCommentListQuery = {},
): Promise<ListResult<CommunityComment>> {
  const { data } = await http.post<ApiResponse<CommunityComment[]>>(
    '/admin/community/comments/all',
    params,
  )
  return toListResult(data)
}

/** 隐藏 / 恢复留言 */
export async function adminSetCommunityCommentStatus(
  id: string,
  status: 'published' | 'hidden',
): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/community/comments/status', { id, status })
}

/** 删除留言 */
export async function adminDeleteCommunityComment(id: string): Promise<void> {
  await http.post<ApiResponse<null>>('/admin/community/comments/delete', { id })
}
