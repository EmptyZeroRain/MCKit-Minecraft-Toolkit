// ===== jar 字节的持久缓存（IndexedDB）=====
//
// 背景：jar 下发接口按项目约定从 GET 改为 POST（JSON 入参 + 二进制响应），
// 浏览器不再对响应做 HTTP 缓存；而 jar 有几十 MB，刷新页面就重下代价太大。
// 这里按 (version, side) 把字节持久化到 IndexedDB，并用服务端回传的官方
// sha1 判断是否仍然有效（服务端重新准备后摘要变化 → 重新下载）。
//
// 环境不支持 IndexedDB（隐私模式、jsdom 单测）时全部降级为空操作：
// 读取返回 undefined（视为未命中，走正常下载），写入静默失败，不影响功能。

const DB_NAME = 'mckit-mcsrc'
const DB_VERSION = 1
const STORE = 'jars'

/** 单条 jar 缓存记录 */
export interface JarRecord {
  /** 缓存键：`<version>-<side>` */
  key: string
  /** 官方 sha1（服务端响应头回传；空串表示未知） */
  sha1: string
  blob: Blob
  /** 写入时间戳（毫秒） */
  savedAt: number
}

/** 打开数据库；不可用时返回 null。 */
function openDB(): Promise<IDBDatabase | null> {
  return new Promise((resolve) => {
    if (typeof indexedDB === 'undefined') {
      resolve(null)
      return
    }
    let req: IDBOpenDBRequest
    try {
      req = indexedDB.open(DB_NAME, DB_VERSION)
    } catch {
      resolve(null)
      return
    }
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(STORE)) {
        db.createObjectStore(STORE, { keyPath: 'key' })
      }
    }
    req.onsuccess = () => resolve(req.result)
    // 打不开（隐私模式 / 配额异常）时静默降级为无缓存。
    req.onerror = () => resolve(null)
    req.onblocked = () => resolve(null)
  })
}

/** 在事务中执行一次请求；任何异常都降级为 undefined。 */
async function withStore<T>(
  mode: IDBTransactionMode,
  run: (store: IDBObjectStore) => IDBRequest,
): Promise<T | undefined> {
  const db = await openDB()
  if (!db) return undefined
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(STORE, mode)
      const req = run(tx.objectStore(STORE))
      req.onsuccess = () => resolve(req.result as T)
      req.onerror = () => reject(req.error)
      tx.onabort = () => reject(tx.error)
    })
  } catch {
    return undefined
  } finally {
    db.close()
  }
}

/** 读取缓存的 jar；未命中或不可用返回 undefined。 */
export async function readJar(key: string): Promise<JarRecord | undefined> {
  const rec = await withStore<JarRecord | undefined>('readonly', (s) => s.get(key))
  if (!rec || !rec.blob) return undefined
  return rec
}

/** 写入 jar 缓存（写入失败不影响调用方，仅失去持久化收益）。 */
export async function writeJar(rec: JarRecord): Promise<void> {
  await withStore<IDBValidKey>('readwrite', (s) => s.put(rec))
}

/** 清空全部 jar 缓存（如版本被重新准备、缓存异常时调用）。 */
export async function clearJars(): Promise<void> {
  await withStore<undefined>('readwrite', (s) => s.clear())
}
