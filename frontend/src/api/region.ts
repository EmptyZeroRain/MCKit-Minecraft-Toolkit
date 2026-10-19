// ===== 区块文件（.mca）检查 API =====
// 契约来源：backend/internal/http/handler/region.go
// 后端仅内存解析、不落盘；文件上限 64MiB。
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  LevelParseResult,
  RegionChunkDetailResult,
  RegionInspectResult,
} from '@/types/mc'

/** 上传 .mca，返回区块清单与顶层信息（/mc/region/inspect） */
export async function inspectRegion(file: File): Promise<RegionInspectResult> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await http.post<ApiResponse<RegionInspectResult>>('/mc/region/inspect', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 60000,
  })
  return data.data
}

/** 上传 level.dat，返回世界基础信息与只读告警（/mc/level/parse） */
export async function parseLevelDat(file: File): Promise<LevelParseResult> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await http.post<ApiResponse<LevelParseResult>>('/mc/level/parse', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 60000,
  })
  return data.data
}

/** 查看指定本地区块的顶层 NBT 摘要（/mc/region/chunk） */
export async function inspectRegionChunk(
  file: File,
  localX: number,
  localZ: number,
): Promise<RegionChunkDetailResult> {
  const form = new FormData()
  form.append('file', file)
  form.append('local_x', String(localX))
  form.append('local_z', String(localZ))
  const { data } = await http.post<ApiResponse<RegionChunkDetailResult>>(
    '/mc/region/chunk',
    form,
    {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 60000,
    },
  )
  return data.data
}
