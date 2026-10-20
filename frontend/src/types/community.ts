// ===== 社区（帖子 / 留言 / 点赞）=====
// 契约来源：backend/internal/http/handler/community.go
// 后端字段：author.user_no 为随机生成的对外用户 ID（如 U7K3M2QX9F）。

export type CommunityCategory = 'chat' | 'help' | 'showcase' | 'tutorial'

export const COMMUNITY_CATEGORIES: CommunityCategory[] = ['chat', 'help', 'showcase', 'tutorial']

export const COMMUNITY_CATEGORY_LABELS: Record<CommunityCategory, string> = {
  chat: '综合闲聊',
  help: '求助',
  showcase: '作品展示',
  tutorial: '教程 / 心得',
}

export const COMMUNITY_STATUS_LABELS: Record<string, string> = {
  published: '已发布',
  hidden: '已隐藏',
}

export interface CommunityAuthor {
  /** 对外随机用户 ID（不暴露内部 UUID） */
  user_no: string
  username: string
  nickname: string
}

export interface CommunityPost {
  id: string
  category: CommunityCategory
  title: string
  body: string
  status: 'published' | 'hidden'
  author: CommunityAuthor
  like_count: number
  comment_count: number
  liked: boolean
  created_at: string
  updated_at: string
}

export interface CommunityComment {
  id: string
  post_id: string
  /** 后台全站评论列表返回所属帖子标题；帖子详情内留言不返回。 */
  post_title?: string
  body: string
  status: 'published' | 'hidden'
  author: CommunityAuthor
  created_at: string
  updated_at: string
}

export interface CommunityPostListQuery {
  category?: CommunityCategory
  q?: string
  sort?: 'latest' | 'hot'
  page?: number
  page_size?: number
}

export interface CreateCommunityPostPayload {
  category: CommunityCategory
  title: string
  body: string
}

export interface CreateCommunityCommentPayload {
  post_id: string
  body: string
}

/** 展示名：昵称优先，回退用户名。 */
export function communityAuthorName(author: CommunityAuthor): string {
  return author.nickname || author.username || '匿名用户'
}
