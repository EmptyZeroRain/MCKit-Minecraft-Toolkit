// ===== Minecraft 结构 / 种子工具 API（全 POST + JSON body）=====
// 契约来源：backend/internal/http/handler/mc.go
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  BiomeQuery,
  BiomeResult,
  CapabilitiesResult,
  FindSeedsQuery,
  FindSeedsResult,
  GenerateSeedsQuery,
  GenerateSeedsResult,
  ParseSeedQuery,
  ParseSeedResult,
  SlimeChunkQuery,
  SlimeChunkResult,
  StrongholdQuery,
  StrongholdResult,
  StructureQuery,
  StructureQueryResult,
  StructureTypesResult,
} from '@/types/mc'

/** 世界分析工具各能力的版本支持矩阵（/mc/capabilities） */
export async function listCapabilities(): Promise<CapabilitiesResult> {
  const { data } = await http.post<ApiResponse<CapabilitiesResult>>('/mc/capabilities', {})
  return data.data
}

/** 生物群系查询（/mc/biome，仅 1.18+ 支持） */
export async function queryBiome(params: BiomeQuery): Promise<BiomeResult> {
  const { data } = await http.post<ApiResponse<BiomeResult>>('/mc/biome', params)
  return data.data
}

/** 支持的结构类型清单（/mc/structures/types） */
export async function listStructureTypes(): Promise<StructureTypesResult> {
  const { data } = await http.post<ApiResponse<StructureTypesResult>>('/mc/structures/types', {})
  return data.data
}

/** 结构理论候选坐标（/mc/structures） */
export async function queryStructures(params: StructureQuery): Promise<StructureQueryResult> {
  const { data } = await http.post<ApiResponse<StructureQueryResult>>('/mc/structures', params)
  return data.data
}

/** 要塞估算（/mc/strongholds） */
export async function estimateStrongholds(params: StrongholdQuery): Promise<StrongholdResult> {
  const { data } = await http.post<ApiResponse<StrongholdResult>>('/mc/strongholds', params)
  return data.data
}

/** 史莱姆区块查询（/mc/slime-chunks） */
export async function querySlimeChunks(params: SlimeChunkQuery): Promise<SlimeChunkResult> {
  const { data } = await http.post<ApiResponse<SlimeChunkResult>>('/mc/slime-chunks', params)
  return data.data
}

/** 生成随机世界种子（/mc/seeds/generate） */
export async function generateSeeds(params: GenerateSeedsQuery = {}): Promise<GenerateSeedsResult> {
  const { data } = await http.post<ApiResponse<GenerateSeedsResult>>('/mc/seeds/generate', params)
  return data.data
}

/** 按「结构出现在目标坐标附近」搜索种子（/mc/seeds/find，服务端限时 5s） */
export async function findSeeds(params: FindSeedsQuery): Promise<FindSeedsResult> {
  const { data } = await http.post<ApiResponse<FindSeedsResult>>('/mc/seeds/find', params, {
    timeout: 20000,
  })
  return data.data
}

/** 解析种子文本为 Java 种子数值（/mc/seeds/parse） */
export async function parseSeed(params: ParseSeedQuery): Promise<ParseSeedResult> {
  const { data } = await http.post<ApiResponse<ParseSeedResult>>('/mc/seeds/parse', params)
  return data.data
}
