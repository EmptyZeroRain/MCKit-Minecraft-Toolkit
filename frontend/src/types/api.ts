// ===== 通用 API 类型（对齐后端统一响应结构）=====

/** 分页元信息 */
export interface PageMeta {
  total: number
  page: number
  page_size: number
  total_pages: number
  /** 无结果搜索时可尝试的关键词（仅搜索接口返回，TODO §5）。 */
  suggestions?: string[]
}

/** 统一成功响应 */
export interface ApiResponse<T> {
  data: T
  meta?: PageMeta
}

/** 单字段错误详情 */
export interface ErrorDetail {
  field?: string
  message: string
}

/** 统一错误响应 */
export interface ApiErrorBody {
  error: {
    code: string
    message: string
    details?: ErrorDetail[]
  }
  request_id: string
}

/** 分页请求通用参数 */
export interface PageQuery {
  page?: number
  page_size?: number
}

/** 可解析为 "合格" 或折叠的 API 错误 */
export class ApiError extends Error {
  code: string
  details?: ErrorDetail[]
  requestId?: string
  status?: number

  constructor(
    message: string,
    code = 'UNKNOWN_ERROR',
    opts?: { details?: ErrorDetail[]; requestId?: string; status?: number },
  ) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.details = opts?.details
    this.requestId = opts?.requestId
    this.status = opts?.status
  }
}

/** 列表接口统一封装结果 */
export interface ListResult<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}