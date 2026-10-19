// ===== MC 源码库（mcsrc）API =====
// 契约与项目一致：全 POST + JSON body。
//   只读接口公开（status / prepared / classes/list / classes/detail / classes/hierarchy）；
//   prepare 触发服务端下载 jar 并建索引，仅具备 ops.mcsrc 权限的管理员可调用。
import http from './http'
import type { ApiResponse } from '@/types/api'
import type {
  McsrcClassPage,
  McsrcClassStructure,
  McsrcHierarchy,
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

/** 准备版本：服务端下载 jar（校验 sha1）并建立类索引（幂等） */
export async function prepare(version: string, side: McsrcSide = 'client'): Promise<McsrcStatus> {
  const { data } = await http.post<ApiResponse<McsrcStatus>>('/mcsrc/prepare', { version, side })
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

/**
 * 下载已缓存的 jar（公开只读）。
 * 这是本模块唯一的 GET 接口：二进制大文件下载需要 Range / ETag 语义；
 * 前端在浏览器内用 Vineflower（WASM）反编译，因此需要 jar 字节。
 */
export async function fetchJarBlob(version: string, side: McsrcSide = 'client'): Promise<Blob> {
  const { data } = await http.get<Blob>('/mcsrc/jar', {
    params: { version, side },
    responseType: 'blob',
    // jar 可达数十 MB，默认 15s 超时不够。
    timeout: 120_000,
  })
  return data
}
