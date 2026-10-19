// ===== 内容历史版本 API（gj.md §15 / §16.2，全 POST + 动作子路径）=====
// 权限由后端按内容类型校验：resources→content.resources、tools→content.tools、wiki→content.wiki。
import http, { toPaged } from './http'
import type { ApiResponse } from '@/types/api'
import type {
  ContentRevision,
  RevisionContentType,
  RevisionDiff,
  RevisionListItem,
} from '@/types/revision'

/** 内容类型 → 路由段（与后端 /admin/revisions/{type}/* 对应） */
const SEGMENT: Record<RevisionContentType, string> = {
  resource: 'resources',
  tool: 'tools',
  wiki: 'wiki',
}

/** 历史版本列表（新→旧） */
export async function listRevisions(
  contentType: RevisionContentType,
  contentId: string,
  page = 1,
  pageSize = 20,
) {
  const { data } = await http.post<ApiResponse<RevisionListItem[]>>(
    `/admin/revisions/${SEGMENT[contentType]}/list`,
    { content_id: contentId, page, page_size: pageSize },
  )
  return toPaged({ data })
}

/** 某版本相对上一版的字段级差异（含该版本完整快照） */
export async function getRevisionDiff(
  contentType: RevisionContentType,
  contentId: string,
  revisionNo: number,
): Promise<RevisionDiff> {
  const { data } = await http.post<ApiResponse<RevisionDiff>>(
    `/admin/revisions/${SEGMENT[contentType]}/diff`,
    { content_id: contentId, revision_no: revisionNo },
  )
  return data.data
}

/** 回滚到指定历史版本（追加一条 rollback 版本，返回新版本） */
export async function rollbackRevision(
  contentType: RevisionContentType,
  contentId: string,
  revisionNo: number,
  note = '',
): Promise<ContentRevision> {
  const { data } = await http.post<ApiResponse<ContentRevision>>(
    `/admin/revisions/${SEGMENT[contentType]}/rollback`,
    { content_id: contentId, revision_no: revisionNo, note },
  )
  return data.data
}
