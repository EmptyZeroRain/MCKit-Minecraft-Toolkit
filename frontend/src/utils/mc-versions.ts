// ===== MC 版本清单（TODO §6-C：版本选项覆盖全部 MC 版本）=====
//
// 数据来源：后端 `POST /versions`（官方 Mojang 清单，服务端带缓存）。
// 拉取失败时回退到离线兜底清单，保证版本筛选始终可用（不因上游抖动而不可用）。
import { listVersions } from '@/api/versions'
import { ALL_VERSIONS, ALL_VERSIONS_LABEL } from '@/types/version'

export { ALL_VERSIONS, ALL_VERSIONS_LABEL }

/** 离线兜底：覆盖各主版本，仅在官方清单拉取失败时使用。 */
export const FALLBACK_RELEASE_VERSIONS: string[] = [
  '1.21',
  '1.20',
  '1.19',
  '1.18',
  '1.17',
  '1.16',
  '1.15',
  '1.14',
  '1.13',
  '1.12',
  '1.11',
  '1.10',
  '1.9',
  '1.8',
  '1.7',
  '1.6',
  '1.5',
  '1.4',
  '1.3',
  '1.2',
  '1.1',
  '1.0',
]

let cache: string[] | null = null
let inflight: Promise<string[]> | null = null

/**
 * 拉取官方**正式版**清单（最新在前）。
 *
 * 内存缓存 + 并发合并：同一会话内多次调用只请求一次；
 * 拉取失败或返回为空时回退兜底清单，调用方无需处理异常。
 */
export async function loadReleaseVersions(): Promise<string[]> {
  if (cache) return cache
  if (inflight) return inflight
  inflight = (async () => {
    try {
      const res = await listVersions({ type: 'release' })
      const ids = (res?.versions ?? []).map((v) => v.id).filter(Boolean)
      cache = ids.length > 0 ? ids : [...FALLBACK_RELEASE_VERSIONS]
    } catch {
      cache = [...FALLBACK_RELEASE_VERSIONS]
    } finally {
      inflight = null
    }
    return cache
  })()
  return inflight
}

/** 仅供测试：重置内存缓存与进行中的请求。 */
export function resetVersionCache(): void {
  cache = null
  inflight = null
}

/** 从完整清单提取主版本（`X.Y`），保持传入顺序（去重）。 */
export function majorVersions(versions: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const v of versions) {
    const m = /^(\d+\.\d+)/.exec(v)
    if (!m || seen.has(m[1])) continue
    seen.add(m[1])
    out.push(m[1])
  }
  return out
}

export interface VersionOption {
  label: string
  value: string
}

/**
 * 构造版本筛选项。
 *
 * `includeAll` 为 true 时在最前加入「全部 MC 版本」，用于筛出**声明了
 * 通配（all）**的内容；不选则不过滤（由调用方用空值表示）。
 */
export function buildVersionOptions(
  versions: string[],
  opts: { includeAll?: boolean } = {},
): VersionOption[] {
  const out: VersionOption[] = versions.map((v) => ({ label: v, value: v }))
  if (opts.includeAll) {
    out.unshift({ label: ALL_VERSIONS_LABEL, value: ALL_VERSIONS })
  }
  return out
}
