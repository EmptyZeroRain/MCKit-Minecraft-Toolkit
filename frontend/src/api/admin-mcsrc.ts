// ===== MC 源码缓存运维 API（需 ops.mcsrc 权限，全 POST + 动作子路径）=====
//
// jar 与索引都是「可重建的派生数据」：清理只影响下次准备需要重新下载 / 建索引，
// 不会丢业务数据；但属运维动作，后端会写审计日志。
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  McsrcCacheEntry,
  McsrcCacheListResult,
  McsrcCachePurgeAllResult,
  McsrcCachePurgeResult,
  McsrcCacheRebuildResult,
} from '@/types/admin'

/** 列出全部已缓存版本、各自磁盘占用与汇总。 */
export async function listMcsrcCache(): Promise<McsrcCacheListResult> {
  const { data } = await http.post<ApiResponse<McsrcCacheListResult>>('/admin/mcsrc/cache/list', {})
  return data.data
}

/** 清理单个版本的影响预览（只读）：预计释放空间、待删文件、是否正在准备。 */
export interface McsrcCacheImpact {
  version: string
  side: string
  cached: boolean
  jar_bytes: number
  index_bytes: number
  meta_bytes: number
  total_bytes: number
  preparing: boolean
  rebuildable: boolean
  files: string[]
}

/** 获取清理单个版本缓存的影响预览（不执行删除）。 */
export async function previewMcsrcPurge(
  version: string,
  side: 'client' | 'server' = 'client',
): Promise<McsrcCacheImpact> {
  const { data } = await http.post<ApiResponse<McsrcCacheImpact>>(
    '/admin/mcsrc/cache/preview',
    { version, side },
  )
  return data.data
}

/** 清理单个版本的源码缓存（jar / 索引 / 元数据）。 */
export async function purgeMcsrcCache(
  version: string,
  side: 'client' | 'server' = 'client',
): Promise<McsrcCachePurgeResult> {
  const { data } = await http.post<ApiResponse<McsrcCachePurgeResult>>('/admin/mcsrc/cache/purge', {
    version,
    side,
  })
  return data.data
}

/**
 * 重建单个版本的缓存（清理 → 重新下载 → 重建索引）。
 *
 * 重建耗时数十秒，后端作为后台任务执行：本接口只负责入队并返回 job_id，
 * 进度与失败原因在「后台任务」页查看，失败可一键重试。
 */
export async function rebuildMcsrcCache(
  version: string,
  side: 'client' | 'server' = 'client',
): Promise<McsrcCacheRebuildResult> {
  const { data } = await http.post<ApiResponse<McsrcCacheRebuildResult>>(
    '/admin/mcsrc/cache/rebuild',
    { version, side },
  )
  return data.data
}

/** 清空全部源码缓存（破坏性操作，需二次确认）。 */
export async function purgeAllMcsrcCache(): Promise<McsrcCachePurgeAllResult> {
  const { data } = await http.post<ApiResponse<McsrcCachePurgeAllResult>>(
    '/admin/mcsrc/cache/purge-all',
    {},
  )
  return data.data
}

export type { McsrcCacheEntry }
