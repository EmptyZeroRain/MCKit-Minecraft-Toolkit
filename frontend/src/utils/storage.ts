// ===== 安全的 localStorage 封装 =====
// 所有本地持久化必须捕获 JSON 解析异常；解析失败时安全重置，
// 保证损坏数据不会导致页面崩溃（需求 5.9 / 13.3）。

const PREFIX = 'mckit:'

export interface StoredPayload<T> {
  version: number
  data: T
}

/**
 * 读取带 schema 版本的数据。版本不匹配或解析失败时返回 null。
 * 调用方（store）负责在版本过期时重置。
 */
export function loadStored<T>(key: string, expectedVersion: number): T | null {
  try {
    const raw = window.localStorage.getItem(PREFIX + key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as StoredPayload<T>
    if (!parsed || typeof parsed !== 'object' || parsed.version !== expectedVersion) {
      return null
    }
    return parsed.data
  } catch {
    // JSON 解析失败或数据形状异常：安全返回 null，不抛出
    return null
  }
}

/** 写入带 schema 版本的数据；localStorage 满或不可用时不抛出 */
export function saveStored<T>(key: string, data: T, version: number): boolean {
  try {
    const payload: StoredPayload<T> = { version, data }
    window.localStorage.setItem(PREFIX + key, JSON.stringify(payload))
    return true
  } catch {
    return false
  }
}

/** 移除某个 key */
export function removeStored(key: string): void {
  try {
    window.localStorage.removeItem(PREFIX + key)
  } catch {
    // 忽略清理失败
  }
}

/** 读取纯字符串（如 UI 偏好），解析异常安全返回默认值 */
export function loadRaw(key: string, fallback = ''): string {
  try {
    return window.localStorage.getItem(PREFIX + key) ?? fallback
  } catch {
    return fallback
  }
}

/** 写入纯字符串，失败静默 */
export function saveRaw(key: string, value: string): void {
  try {
    window.localStorage.setItem(PREFIX + key, value)
  } catch {
    // 忽略
  }
}

/** 是否可用（隐私合规说明用） */
export function storageAvailable(): boolean {
  try {
    const k = '__mckit_test__'
    window.localStorage.setItem(k, '1')
    window.localStorage.removeItem(k)
    return true
  } catch {
    return false
  }
}