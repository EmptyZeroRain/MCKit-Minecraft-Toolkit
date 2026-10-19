// ===== 种子查询功能类型定义 =====

/** 支持查询的游戏版本区间 */
export const SEED_VERSIONS = [
  '1.18+',
  '1.16-1.17',
  '1.13-1.15',
  '1.8-1.12',
] as const
export type SeedVersion = (typeof SEED_VERSIONS)[number]

/** 结构类型（与后端 seedStructureMap 对齐） */
export const SEED_STRUCTURE_TYPES = [
  'stronghold',
  'village',
  'ancient_city',
  'mansion',
  'monument',
  'outpost',
  'ruined_portal',
  'end_city',
  'desert_pyramid',
  'jungle_pyramid',
  'igloo',
  'swamp_hut',
  'ocean_ruin',
  'shipwreck',
  'trail_ruins',
  'trial_chambers',
  'buried_treasure',
  'spawn',
  'slime_chunk',
] as const
export type SeedStructureType = (typeof SEED_STRUCTURE_TYPES)[number]

/** 结构位置（方块坐标，后端引擎返回） */
export interface StructurePoint {
  type: SeedStructureType
  /** 方块坐标 */
  x: number
  z: number
  /** 生成该结构所需信息（如 FORGE 中需要若干结构同时存在） */
  label?: string
}

/** 单区块采样：区块坐标 + 生态群系 ID（后端引擎返回，供地图渲染） */
export interface BiomeChunkSample {
  /** 区块坐标 */
  cx: number
  cz: number
  /** 生态群系 ID，如 plains / forest / ocean */
  biome: string
}

/** 种子分析请求参数（全 POST + body） */
export interface SeedAnalyzeQuery {
  /** 用户输入的种子（数字或字符串） */
  seed: string
  version?: SeedVersion
  /** 地图半径（区块数，默认 8，即 16x16 区块） */
  radius?: number
  /** 是否返回生物群系采样数据 */
  include_biomes?: boolean
  /** 是否返回结构位置 */
  include_structures?: boolean
}

/** 种子分析综合结果 */
export interface SeedAnalysis {
  /** 解析出的世界种子 */
  world_seed: string
  /** 区块地图 */
  radius: number
  /** 生物群系采样（顺序排列：cz 从 centerZ-radius 到 centerZ+radius 等） */
  biomes?: BiomeChunkSample[]
  /** 结构位置（含史莱姆区块） */
  structures?: StructurePoint[]
  /** 后端附带的说明（如「结构坐标为理论候选/估算值」） */
  note?: string
}

/** 前端本地引擎可精确计算的结果 */
export interface LocalSeedResult {
  world_seed: string
  /** 本地精确史莱姆区块 */
  slime_chunks: StructurePoint[]
  /** 是否依赖后端引擎（false 表示 hero 全部本地，true 表示结构数据需后端） */
  backend_pending: boolean
}