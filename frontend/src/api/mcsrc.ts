// ===== MC 源码库（mcsrc）API =====
// 契约与项目一致：全 POST + JSON body。
//   只读接口公开（status / prepared / classes/list / classes/detail / classes/hierarchy）；
//   prepare 触发服务端下载 jar 并建索引，仅具备 ops.mcsrc 权限的管理员可调用。
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  McsrcClassBytecode,
  McsrcClassMapping,
  McsrcClassPage,
  McsrcClassStructure,
  McsrcHierarchy,
  McsrcReferenceQuery,
  McsrcReferences,
  McsrcSide,
  McsrcStatus,
} from '@/types/mcsrc'

/** 查询某版本（侧别）的准备状态 */
export async function getStatus(version: string, side: McsrcSide = 'client'): Promise<McsrcStatus> {
  const { data } = await http.post<ApiResponse<McsrcStatus>>('/mcsrc/status', { version, side })
  return data.data
}

/** 列出全部已缓存的版本 */
export async function listPrepared(): Promise<McsrcStatus[]> {
  const { data } = await http.post<ApiResponse<McsrcStatus[]>>('/mcsrc/prepared', {})
  return data.data ?? []
}

/**
 * 触发准备：服务端下载 jar（校验 sha1）并建立类索引（幂等）。
 * 任何访客都可调用（前端选中版本即自动触发）；匿名访客受服务端
 * `MCSRC_AUTO_PREPARE_MAX` 额度限制，超出返回 429 `AUTO_PREPARE_LIMIT`。
 * 首次准备需下载数十 MB 并建索引，故超时放宽到 5 分钟。
 */
export async function prepare(version: string, side: McsrcSide = 'client'): Promise<McsrcStatus> {
  const { data } = await http.post<ApiResponse<McsrcStatus>>(
    '/mcsrc/prepare',
    { version, side },
    { timeout: 300_000 },
  )
  return data.data
}

export interface ClassQuery {
  version: string
  side?: McsrcSide
  /** 关键词（大小写不敏感，匹配简单名与全限定名） */
  q?: string
  /** 包名前缀过滤 */
  pkg?: string
  include_inner?: boolean
  page?: number
  page_size?: number
}

/** 在已建索引的版本中检索类 */
export async function listClasses(params: ClassQuery): Promise<McsrcClassPage> {
  const { data } = await http.post<ApiResponse<McsrcClassPage>>('/mcsrc/classes/list', params)
  return data.data
}

/** 查询单个类的结构视图（修饰符 / 父类 / 接口 / 字段 / 方法） */
export async function getClassDetail(
  version: string,
  className: string,
  side: McsrcSide = 'client',
): Promise<McsrcClassStructure> {
  const { data } = await http.post<ApiResponse<McsrcClassStructure>>('/mcsrc/classes/detail', {
    version,
    side,
    class: className,
  })
  return data.data
}

/** 查询单个类的继承关系（祖先链 + 直接子类） */
export async function getClassHierarchy(
  version: string,
  className: string,
  side: McsrcSide = 'client',
): Promise<McsrcHierarchy> {
  const { data } = await http.post<ApiResponse<McsrcHierarchy>>('/mcsrc/classes/hierarchy', {
    version,
    side,
    class: className,
  })
  return data.data
}

/** 查询单个类的字节码反汇编（ASM Textifier 风格文本） */
export async function getClassBytecode(
  version: string,
  className: string,
  side: McsrcSide = 'client',
): Promise<McsrcClassBytecode> {
  const { data } = await http.post<ApiResponse<McsrcClassBytecode>>('/mcsrc/classes/bytecode', {
    version,
    side,
    class: className,
  })
  return data.data
}

/**
 * 查找对某个类 / 方法 / 字段的引用。
 * 首次调用需服务端扫描整个 jar 建反向引用索引（较慢），之后为毫秒级，故超时放宽。
 */
export async function getClassReferences(params: McsrcReferenceQuery): Promise<McsrcReferences> {
  const { data } = await http.post<ApiResponse<McsrcReferences>>(
    '/mcsrc/classes/references',
    { side: 'client', kind: 'class', ...params },
    { timeout: 120_000 },
  )
  return data.data
}

/**
 * 查询类映射重命名（按版本 + jar 内类名反查已导入的映射）。
 * 未导入映射或没有覆盖该类时返回 `available: false`（正常状态，非错误）。
 */
export async function getClassMapping(
  version: string,
  className: string,
  side: McsrcSide = 'client',
): Promise<McsrcClassMapping> {
  const { data } = await http.post<ApiResponse<McsrcClassMapping>>('/mcsrc/classes/mapping', {
    version,
    side,
    class: className,
  })
  return data.data
}

/** jar 摘要响应头：服务端回传官方 sha1，供前端做持久缓存键 */
const JAR_SHA1_HEADER = 'x-mcsrc-jar-sha1'

/** 下载结果：jar 字节 + 服务端回传的官方 sha1 */
export interface McsrcJarBlob {
  blob: Blob
  /** 官方 sha1（响应头缺失时为空串） */
  sha1: string
}

/**
 * 下发已缓存的 jar（公开只读）。
 *
 * 与项目约定一致走 POST + JSON body，因此浏览器不会对响应做 HTTP 缓存；
 * 几十 MB 的 jar 改由前端按响应头回传的 sha1 持久化到 IndexedDB
 * （见 `@/mcsrc/jarStore`），跨刷新复用。
 */
export async function fetchJarBlob(
  version: string,
  side: McsrcSide = 'client',
): Promise<McsrcJarBlob> {
  const res = await http.post<Blob>(
    '/mcsrc/jar',
    { version, side },
    {
      responseType: 'blob',
      // jar 可达数十 MB，默认 15s 超时不够。
      timeout: 300_000,
    },
  )
  const sha1 = String(res.headers?.[JAR_SHA1_HEADER] ?? '')
  return { blob: res.data, sha1 }
}
