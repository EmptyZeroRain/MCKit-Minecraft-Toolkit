// ===== 社区 API（用户端 / 公开读，全 POST）=====
// 读接口匿名可用；写接口由 userHttp 自动附加用户 CSRF token。
import userHttp from './user-http'
import type { ApiResponse, ListResult } from '@/types/api'
import type {
  CommunityComment,
  CommunityPost,
  CommunityPostListQuery,
  CreateCommunityCommentPayload,
  CreateCommunityPostPayload,
} from '@/types/community'

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

/** 帖子列表（公开，仅 published） */
export async function listCommunityPosts(
  params: CommunityPostListQuery = {},
): Promise<ListResult<CommunityPost>> {
  const { data } = await userHttp.post<ApiResponse<CommunityPost[]>>('/community/posts', params)
  return toListResult(data)
}

/** 帖子详情（公开） */
export async function fetchCommunityPost(id: string): Promise<CommunityPost> {
  const { data } = await userHttp.post<ApiResponse<CommunityPost>>('/community/posts/detail', { id })
  return data.data
}

/** 帖子留言列表（公开，按时间正序） */
export async function listCommunityComments(params: {
  post_id: string
  page?: number
  page_size?: number
}): Promise<ListResult<CommunityComment>> {
  const { data } = await userHttp.post<ApiResponse<CommunityComment[]>>(
    '/community/comments',
    params,
  )
  return toListResult(data)
}

/** 发帖（登录 + CSRF） */
export async function createCommunityPost(
  payload: CreateCommunityPostPayload,
): Promise<CommunityPost> {
  const { data } = await userHttp.post<ApiResponse<CommunityPost>>(
    '/community/posts/create',
    payload,
  )
  return data.data
}

/** 删除自己的帖子（登录 + CSRF） */
export async function deleteCommunityPost(id: string): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/community/posts/delete', { id })
}

/** 点赞 / 取消点赞，返回操作后的状态与点赞数 */
export async function toggleCommunityLike(
  postId: string,
): Promise<{ liked: boolean; like_count: number }> {
  const { data } = await userHttp.post<ApiResponse<{ liked: boolean; like_count: number }>>(
    '/community/posts/like',
    { post_id: postId },
  )
  return data.data
}

/** 发表留言（登录 + CSRF） */
export async function createCommunityComment(
  payload: CreateCommunityCommentPayload,
): Promise<CommunityComment> {
  const { data } = await userHttp.post<ApiResponse<CommunityComment>>(
    '/community/comments/create',
    payload,
  )
  return data.data
}

/** 删除留言（留言作者或帖子作者） */
export async function deleteCommunityComment(id: string): Promise<void> {
  await userHttp.post<ApiResponse<null>>('/community/comments/delete', { id })
}
