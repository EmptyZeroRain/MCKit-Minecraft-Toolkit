// ===== Minecraft 结构 / 种子 / 区块工具类型 =====
// 对齐后端：/api/v1/mc/* 与 /api/v1/seed/analyze（见 backend/internal/http/handler）。

/** 方块坐标 */
export interface BlockPos {
  x: number
  z: number
}

/**
 * 种子数值。
 * ⚠️ 后端目前以 JSON number 返回 int64 种子，超过 2^53 时 JS 会静默丢精度。
 * 前端同时接受字符串，后端改为 string 返回后无需再改前端。
 */
export type SeedValue = number | string

/** 后端 mcstructure 支持的结构类型（与 internal/mcstructure/structure.go 保持一致） */
export const MC_STRUCTURE_TYPES = [
  'ancient_city',
  'buried_treasure',
  'desert_pyramid',
  'end_city',
  'igloo',
  'jungle_pyramid',
  'mineshaft',
  'nether_fortress',
  'bastion_remnant',
  'nether_fossil',
  'ocean_ruin',
  'pillager_outpost',
  'ruined_portal',
  'shipwreck',
  'swamp_hut',
  'trail_ruins',
  'trial_chambers',
  'village',
  'woodland_mansion',
  'ocean_monument',
] as const
export type McStructureType = (typeof MC_STRUCTURE_TYPES)[number]

export const MC_STRUCTURE_LABELS: Record<McStructureType, string> = {
  ancient_city: '远古城市',
  buried_treasure: '埋藏的宝藏',
  desert_pyramid: '沙漠神殿',
  end_city: '末地城',
  igloo: '雪屋',
  jungle_pyramid: '丛林神庙',
  mineshaft: '废弃矿井',
  nether_fortress: '下界要塞',
  bastion_remnant: '堡垒遗迹',
  nether_fossil: '下界化石',
  ocean_ruin: '海底废墟',
  pillager_outpost: '掠夺者前哨站',
  ruined_portal: '废弃传送门',
  shipwreck: '沉船',
  swamp_hut: '女巫小屋',
  trail_ruins: '古迹废墟',
  trial_chambers: '试炼密室',
  village: '村庄',
  woodland_mansion: '林地府邸',
  ocean_monument: '海底神殿',
}

// ===== 请求参数 =====

/** 结构坐标查询（/mc/structures） */
export interface StructureQuery {
  seed: string
  type: McStructureType | string
  version?: string
  region_x?: number
  region_z?: number
  count_x?: number
  count_z?: number
}

/** 群系查询（/mc/biome，仅 1.18+ 支持） */
export interface BiomeQuery {
  seed: string
  x: number
  z: number
  version?: string
}

/** 要塞估算（/mc/strongholds） */
export interface StrongholdQuery {
  seed: string
  count?: number
}

/** 史莱姆区块查询（/mc/slime-chunks） */
export interface SlimeChunkQuery {
  seed: string
  min_chunk_x: number
  min_chunk_z: number
  max_chunk_x: number
  max_chunk_z: number
}

/** 种子生成（/mc/seeds/generate） */
export interface GenerateSeedsQuery {
  count?: number
}

/** 种子搜索（/mc/seeds/find） */
export interface FindSeedsQuery {
  type: McStructureType | string
  center_x: number
  center_z: number
  radius?: number
  limit?: number
  max_attempts?: number
}

/** 种子解析（/mc/seeds/parse） */
export interface ParseSeedQuery {
  seed: string
}

// ===== 响应 =====

/** 结构候选点（含群系可行性标注，1.18+ 可用） */
export interface StructurePosition extends BlockPos {
  biome?: string
  /** true=群系可行（含邻近偏移），false=邻近无合适群系 */
  feasible?: boolean
  shift_x?: number
  shift_z?: number
}

export interface StructureQueryResult {
  seed: SeedValue
  type: string
  version?: string
  dimension?: string
  /** 生成尝试频率上限（如 0.01 表示 1%） */
  probability?: number
  /** 该结构是否在所选版本生成 */
  available?: boolean
  /** 分布算法是否与当前实现一致（1.12 及更早为 false） */
  placement_supported?: boolean
  /** available && placement_supported */
  supported?: boolean
  timed_out?: boolean
  positions: StructurePosition[]
  note?: string
}

/** 结构参数描述（/mc/structures/types） */
export interface StructureInfo {
  type: string
  spacing: number
  separation: number
  salt: number
  uniform: boolean
  dimension: string
  probability: number
  has_biome_rule: boolean
  since_version: string
}

/** 单个功能在某一版本区间的支持情况 */
export interface FeatureSupport {
  version: string
  supported: boolean
  note?: string
}

/** 单个能力的版本支持矩阵（/mc/capabilities） */
export interface FeatureCapability {
  id: string
  name: string
  supported: FeatureSupport[]
  note: string
}

export interface CapabilitiesResult {
  versions: string[]
  features: FeatureCapability[]
}

/** 群系查询结果（/mc/biome） */
export interface BiomeResult {
  seed: SeedValue
  x: number
  z: number
  version: string
  supported: boolean
  biome?: string
  note?: string
}

export interface StrongholdResult {
  seed: SeedValue
  count: number
  positions: BlockPos[]
  note?: string
}

export interface SlimeChunkItem {
  chunk_x: number
  chunk_z: number
  x: number
  z: number
}

export interface SlimeChunkResult {
  seed: SeedValue
  count: number
  chunks: SlimeChunkItem[]
  timed_out?: boolean
  note?: string
}

export interface GenerateSeedsResult {
  seeds: number[]
}

export interface FoundSeed {
  seed: SeedValue
  x: number
  z: number
  distance: number
}

export interface FindSeedsResult {
  seeds: FoundSeed[]
  note?: string
}

export interface ParseSeedResult {
  seed: SeedValue
}

export interface StructureTypesResult {
  types: string[]
  /** 结构参数描述（版本、维度、频率、群系约束），后端新版本提供 */
  structures?: StructureInfo[]
  versions?: string[]
}

// ===== 区块文件（.mca）=====

/** 压缩类型：1=gzip, 2=zlib, 3=无压缩, 0=未知 */
export const REGION_COMPRESSION_LABELS: Record<number, string> = {
  0: '未知',
  1: 'gzip',
  2: 'zlib',
  3: '无压缩',
}

export function compressionLabel(code: number): string {
  return REGION_COMPRESSION_LABELS[code] ?? `未知(${code})`
}

export interface RegionChunkEntry {
  local_x: number
  local_z: number
  global_chunk_x: number
  global_chunk_z: number
  /** unix 秒 */
  timestamp: number
  compression: number
  data_length: number
  data_version?: number
  status?: string
}

/** 区域文件基础统计（/mc/region/inspect 的 stats） */
export interface RegionStats {
  file_size: number
  /** 非空位置项数量 */
  total_chunks: number
  /** 空位置项数量 */
  empty_slots: number
  /** 位置表声明的扇区总数 */
  sectors_declared: number
  /** 文件可容纳的扇区数 */
  sector_capacity: number
  /** 末尾不足一个扇区的残余字节 */
  trailing_bytes: number
  /** 压缩类型名称 -> 区块数量 */
  compression: Record<string, number>
  /** 有效时间戳最小值（unix 秒），无则为 0 */
  oldest_timestamp: number
  /** 有效时间戳最大值 */
  newest_timestamp: number
}

/** 单条诊断结论（只读检查，不代表已修复） */
export interface RegionIssue {
  level: 'error' | 'warning'
  code: string
  message: string
  /** 区块本地区块坐标；文件级问题不含该字段 */
  local_x?: number
  local_z?: number
}

/** 区域文件只读诊断报告 */
export interface RegionDiagnosis {
  region_x: number
  region_z: number
  errors: RegionIssue[]
  warnings: RegionIssue[]
  stats: RegionStats
}

export interface RegionInspectResult {
  region_x: number
  region_z: number
  chunk_count: number
  chunks: RegionChunkEntry[]
  /** 区域文件统计（后端新增） */
  stats?: RegionStats
  /** 只读诊断报告（后端新增） */
  diagnosis?: RegionDiagnosis
  /** 逐区块 NBT 解析是否因超时提前结束 */
  timed_out?: boolean
}

export interface RegionChunkDetailResult {
  region_x: number
  region_z: number
  local_x: number
  local_z: number
  global_chunk_x: number
  global_chunk_z: number
  compression: number
  fields: Record<string, unknown>
  top_level_keys: string[]
}

// ===== 世界存档（level.dat）=====

/** 存档版本信息（Data.Version） */
export interface WorldVersionInfo {
  id: number
  name?: string
  snapshot?: boolean
}

/** level.dat 基础信息（数值字段缺失时不出现） */
export interface WorldInfo {
  level_name?: string
  data_version?: number
  version?: WorldVersionInfo
  /** 最近游玩时间（unix 毫秒） */
  last_played_ms?: number
  /** ISO8601 文本形式 */
  last_played?: string
  /** int64 种子以字符串返回，避免 JS 丢精度 */
  random_seed?: string
  spawn_x?: number
  spawn_y?: number
  spawn_z?: number
  time?: number
  day_time?: number
  game_type?: number
  difficulty?: number
  hardcore?: boolean
  allow_commands?: boolean
  generator_name?: string
  game_rules?: Record<string, string>
}

/** level.dat 只读诊断告警（非确定错误） */
export interface LevelWarning {
  code: string
  message: string
}

export interface LevelParseResult {
  file_name: string
  world: WorldInfo
  warnings: LevelWarning[]
  top_level_keys: string[]
  note?: string
}
