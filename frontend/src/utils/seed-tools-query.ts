// ===== 种子与结构工具：URL 状态编解码（TODO §14）=====
//
// 该页是「多工具聚合页」，每个子表单的输入都属于可分享的查询条件，
// 因此在此集中做「查询参数 ↔ 表单状态」双向映射：
//   - 视图代码保持精简；
//   - 编解码为纯函数，可独立单测（含默认值省略与非法值回退）。
// 参数命名按子工具加前缀（`stSeed` / `bioX` / ...），可读性优先于长度。
import type { LocationQuery } from 'vue-router'
import { MC_STRUCTURE_TYPES } from '@/types/mc'
import { SEED_VERSIONS, type SeedVersion } from '@/types/seed'
import { queryNumber, queryString } from './share'

/** 子表单状态类型（与后端接口字段同名，便于直接展开传入 API） */
export interface StructureFormState {
  seed: string
  type: string
  region_x: number
  region_z: number
  count_x: number
  count_z: number
}

export interface BiomeFormState {
  seed: string
  x: number
  z: number
}

export interface StrongholdFormState {
  seed: string
  count: number
}

export interface SlimeFormState {
  seed: string
  min_chunk_x: number
  min_chunk_z: number
  max_chunk_x: number
  max_chunk_z: number
}

export interface FindSeedsFormState {
  type: string
  center_x: number
  center_z: number
  radius: number
  limit: number
}

/** 完整页面状态 */
export interface SeedToolsQueryState {
  version: SeedVersion
  parseInput: string
  genCount: number
  structures: StructureFormState
  biome: BiomeFormState
  stronghold: StrongholdFormState
  slime: SlimeFormState
  find: FindSeedsFormState
}

/**
 * 各子表单默认值：同时作为「URL 省略默认值」的依据，
 * 保证「默认状态 ⇔ 无查询参数」这一双向一致。
 */
export const SEED_TOOLS_DEFAULTS: SeedToolsQueryState = {
  version: SEED_VERSIONS[0],
  parseInput: '',
  genCount: 3,
  structures: {
    seed: '',
    type: MC_STRUCTURE_TYPES[0],
    region_x: 0,
    region_z: 0,
    count_x: 1,
    count_z: 1,
  },
  biome: { seed: '', x: 0, z: 0 },
  stronghold: { seed: '', count: 3 },
  slime: { seed: '', min_chunk_x: -4, min_chunk_z: -4, max_chunk_x: 4, max_chunk_z: 4 },
  find: { type: 'village', center_x: 0, center_z: 0, radius: 256, limit: 5 },
}

/** 版本参数校验：非法或缺失回退默认版本（避免分享链接带坏值导致接口 422）。 */
function restoreVersion(value: string): SeedVersion {
  return (SEED_VERSIONS as readonly string[]).includes(value)
    ? (value as SeedVersion)
    : SEED_TOOLS_DEFAULTS.version
}

/**
 * 结构类型校验：只接受 `snake_case` slug。
 * 不硬编码结构清单，因为后端可能扩展类型；此处只拦截明显非法的分享参数。
 */
function restoreStructureType(value: string, fallback: string): string {
  return /^[a-z][a-z0-9_]*$/.test(value) ? value : fallback
}

/** 从路由 query 还原页面状态（缺失或非法一律回退默认值）。 */
export function readSeedToolsQuery(query: LocationQuery): SeedToolsQueryState {
  const d = SEED_TOOLS_DEFAULTS
  return {
    version: restoreVersion(queryString(query.v)),
    parseInput: queryString(query.parse),
    genCount: queryNumber(query.gen, d.genCount),
    structures: {
      seed: queryString(query.stSeed),
      type: restoreStructureType(queryString(query.stType), d.structures.type),
      region_x: queryNumber(query.stRx, d.structures.region_x),
      region_z: queryNumber(query.stRz, d.structures.region_z),
      count_x: queryNumber(query.stCx, d.structures.count_x),
      count_z: queryNumber(query.stCz, d.structures.count_z),
    },
    biome: {
      seed: queryString(query.bioSeed),
      x: queryNumber(query.bioX, d.biome.x),
      z: queryNumber(query.bioZ, d.biome.z),
    },
    stronghold: {
      seed: queryString(query.shSeed),
      count: queryNumber(query.shCount, d.stronghold.count),
    },
    slime: {
      seed: queryString(query.slSeed),
      min_chunk_x: queryNumber(query.slMinX, d.slime.min_chunk_x),
      min_chunk_z: queryNumber(query.slMinZ, d.slime.min_chunk_z),
      max_chunk_x: queryNumber(query.slMaxX, d.slime.max_chunk_x),
      max_chunk_z: queryNumber(query.slMaxZ, d.slime.max_chunk_z),
    },
    find: {
      type: restoreStructureType(queryString(query.fdType), d.find.type),
      center_x: queryNumber(query.fdCx, d.find.center_x),
      center_z: queryNumber(query.fdCz, d.find.center_z),
      radius: queryNumber(query.fdRadius, d.find.radius),
      limit: queryNumber(query.fdLimit, d.find.limit),
    },
  }
}

/** 将页面状态编码为查询参数（省略默认值与空值，保持链接简洁可读）。 */
export function buildSeedToolsQuery(state: SeedToolsQueryState): Record<string, string> {
  const d = SEED_TOOLS_DEFAULTS
  const out: Record<string, string> = {}
  const put = (key: string, value: string | number, fallback: string | number) => {
    if (value === fallback) return
    const str = String(value)
    if (str === '') return
    out[key] = str
  }

  put('v', state.version, d.version)
  put('parse', state.parseInput, d.parseInput)
  put('gen', state.genCount, d.genCount)

  put('stSeed', state.structures.seed, d.structures.seed)
  put('stType', state.structures.type, d.structures.type)
  put('stRx', state.structures.region_x, d.structures.region_x)
  put('stRz', state.structures.region_z, d.structures.region_z)
  put('stCx', state.structures.count_x, d.structures.count_x)
  put('stCz', state.structures.count_z, d.structures.count_z)

  put('bioSeed', state.biome.seed, d.biome.seed)
  put('bioX', state.biome.x, d.biome.x)
  put('bioZ', state.biome.z, d.biome.z)

  put('shSeed', state.stronghold.seed, d.stronghold.seed)
  put('shCount', state.stronghold.count, d.stronghold.count)

  put('slSeed', state.slime.seed, d.slime.seed)
  put('slMinX', state.slime.min_chunk_x, d.slime.min_chunk_x)
  put('slMinZ', state.slime.min_chunk_z, d.slime.min_chunk_z)
  put('slMaxX', state.slime.max_chunk_x, d.slime.max_chunk_x)
  put('slMaxZ', state.slime.max_chunk_z, d.slime.max_chunk_z)

  put('fdType', state.find.type, d.find.type)
  put('fdCx', state.find.center_x, d.find.center_x)
  put('fdCz', state.find.center_z, d.find.center_z)
  put('fdRadius', state.find.radius, d.find.radius)
  put('fdLimit', state.find.limit, d.find.limit)

  return out
}
