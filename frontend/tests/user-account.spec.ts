// 需求 §12：普通用户账号 / 个人工作台前端逻辑测试
// 说明：全部通过 mock '@/api/user' 隔离网络，只验证 store 行为与契约调用。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { UserFavoriteItem, UserRecentItem, UserSeed } from '@/types/user'

const api = {
  fetchCaptcha: vi.fn(),
  register: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  me: vi.fn(),
  changePassword: vi.fn(),
  deleteAccount: vi.fn(),
  listFavorites: vi.fn(),
  toggleFavorite: vi.fn(),
  removeFavorite: vi.fn(),
  listRecents: vi.fn(),
  recordRecent: vi.fn(),
  clearRecents: vi.fn(),
  listSeeds: vi.fn(),
  saveSeed: vi.fn(),
  deleteSeed: vi.fn(),
}

vi.mock('@/api/user', () => api)

// 必须在 vi.mock 之后导入，确保拿到的是 mock 版本
const { useUserAuthStore } = await import('@/stores/user-auth')
const { useUserWorkspaceStore } = await import('@/stores/user-workspace')
const { useRecentToolsStore } = await import('@/stores/recent-tools')

const FAV: UserFavoriteItem = {
  id: 'f1',
  kind: 'tool',
  object_id: 'seed-tools',
  label: '种子工具',
  url: '/seed-tools',
  created_at: '2026-01-01T00:00:00Z',
}

const RECENT: UserRecentItem = {
  id: 'r1',
  kind: 'tool',
  object_id: 'seed-tools',
  label: '种子工具',
  url: '/seed-tools',
  visited_at: '2026-01-01T00:00:00Z',
}

const SEED: UserSeed = {
  id: 's1',
  name: '示例',
  seed: '42',
  note: '',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.listFavorites.mockResolvedValue([])
  api.listRecents.mockResolvedValue([])
  api.listSeeds.mockResolvedValue([])
})

describe('user-auth store', () => {
  it('恢复会话成功时写入用户并预取收藏', async () => {
    api.me.mockResolvedValue({ id: 'u1', username: 'alice' })
    api.listFavorites.mockResolvedValue([FAV])

    const auth = useUserAuthStore()
    await auth.restoreSession()

    expect(auth.isLoggedIn).toBe(true)
    expect(auth.displayName).toBe('alice')
    expect(useUserWorkspaceStore().favorites).toHaveLength(1)
  })

  it('恢复会话失败时保持未登录且不抛错', async () => {
    api.me.mockRejectedValue(new Error('401'))

    const auth = useUserAuthStore()
    await auth.restoreSession()

    expect(auth.isLoggedIn).toBe(false)
    expect(auth.initialized).toBe(true)
  })

  it('restoreSession 幂等：重复调用只请求一次', async () => {
    api.me.mockResolvedValue({ id: 'u1', username: 'alice' })
    const auth = useUserAuthStore()
    await auth.restoreSession()
    await auth.restoreSession()
    expect(api.me).toHaveBeenCalledTimes(1)
  })

  it('登录后 isLoggedIn 为真，退出后清空登录态与个人数据', async () => {
    api.login.mockResolvedValue({ id: 'u1', username: 'alice' })
    const auth = useUserAuthStore()
    await auth.login('alice', 'password123')
    expect(auth.isLoggedIn).toBe(true)

    const workspace = useUserWorkspaceStore()
    workspace.favorites = [FAV]
    api.logout.mockResolvedValue(undefined)
    await auth.logout()

    expect(auth.isLoggedIn).toBe(false)
    expect(workspace.favorites).toHaveLength(0)
  })

  it('改密失败时不应清空本地登录态', async () => {
    api.login.mockResolvedValue({ id: 'u1', username: 'alice' })
    const auth = useUserAuthStore()
    await auth.login('alice', 'password123')

    api.changePassword.mockRejectedValue(new Error('原密码错误'))
    await expect(auth.changePassword('bad', 'newpassword123')).rejects.toThrow()
    expect(auth.isLoggedIn).toBe(true)

    api.changePassword.mockResolvedValue(undefined)
    await auth.changePassword('password123', 'newpassword123')
    expect(auth.isLoggedIn).toBe(false)
  })

  it('登录以 identifier 传参，注册透传完整 payload', async () => {
    api.login.mockResolvedValue({ id: 'u1', username: 'alice', nickname: '爱丽丝' })
    const auth = useUserAuthStore()
    await auth.login('alice@example.com', 'password123')
    expect(api.login).toHaveBeenCalledWith({
      identifier: 'alice@example.com',
      password: 'password123',
    })
    expect(auth.displayName).toBe('爱丽丝')

    api.register.mockResolvedValue({ id: 'u2', username: 'bob', nickname: '鲍勃' })
    const payload = {
      username: 'bob',
      nickname: '鲍勃',
      email: 'bob@example.com',
      password: 'password123',
      captcha_id: 'c1',
      captcha_answer: 'AB12',
    }
    await auth.register(payload)
    expect(api.register).toHaveBeenCalledWith(payload)
  })

  it('displayName 无昵称时回退到用户名', async () => {
    api.me.mockResolvedValue({ id: 'u1', username: 'alice' })
    const auth = useUserAuthStore()
    await auth.restoreSession()
    expect(auth.displayName).toBe('alice')
  })
})

describe('user-workspace store', () => {
  it('loadAll 并行拉取收藏 / 最近使用 / 常用种子', async () => {
    api.listFavorites.mockResolvedValue([FAV])
    api.listRecents.mockResolvedValue([RECENT])
    api.listSeeds.mockResolvedValue([SEED])

    const ws = useUserWorkspaceStore()
    await ws.loadAll()

    expect(ws.favorites).toEqual([FAV])
    expect(ws.recents).toEqual([RECENT])
    expect(ws.seeds).toEqual([SEED])
    expect(ws.loaded).toBe(true)
  })

  it('toggle 后刷新收藏，isFavorite 反映最新状态', async () => {
    const ws = useUserWorkspaceStore()
    expect(ws.isFavorite('tool', 'seed-tools')).toBe(false)

    api.toggleFavorite.mockResolvedValue(true)
    api.listFavorites.mockResolvedValue([FAV])
    const favorited = await ws.toggle('tool', 'seed-tools', '种子工具', '/seed-tools')

    expect(favorited).toBe(true)
    expect(api.toggleFavorite).toHaveBeenCalledWith({
      kind: 'tool',
      object_id: 'seed-tools',
      label: '种子工具',
      url: '/seed-tools',
    })
    expect(ws.isFavorite('tool', 'seed-tools')).toBe(true)
  })

  it('removeFavorite 同步剔除本地项', async () => {
    api.listFavorites.mockResolvedValue([FAV])
    const ws = useUserWorkspaceStore()
    await ws.loadFavorites()

    api.removeFavorite.mockResolvedValue(undefined)
    await ws.removeFavorite('tool', 'seed-tools')

    expect(ws.favorites).toHaveLength(0)
  })

  it('recordRecent 失败时静默（不抛出）', async () => {
    api.recordRecent.mockRejectedValue(new Error('网络异常'))
    const ws = useUserWorkspaceStore()
    await expect(ws.recordRecent('tool', 'seed-tools', '种子工具', '/seed-tools')).resolves.toBeUndefined()
  })

  it('clearRecents 清空本地最近使用', async () => {
    api.listRecents.mockResolvedValue([RECENT])
    const ws = useUserWorkspaceStore()
    await ws.loadAll()
    expect(ws.recents).toHaveLength(1)

    api.clearRecents.mockResolvedValue(undefined)
    await ws.clearRecents()
    expect(ws.recents).toHaveLength(0)
  })

  it('saveSeed 成功后重新拉取列表', async () => {
    const ws = useUserWorkspaceStore()
    api.saveSeed.mockResolvedValue(SEED)
    api.listSeeds.mockResolvedValue([SEED])

    await ws.saveSeed({ name: '示例', seed: '42' })
    expect(api.saveSeed).toHaveBeenCalledWith({ name: '示例', seed: '42' })
    expect(ws.seeds).toEqual([SEED])
  })

  it('deleteSeed 同步剔除本地项', async () => {
    api.listSeeds.mockResolvedValue([SEED])
    const ws = useUserWorkspaceStore()
    await ws.loadAll()

    api.deleteSeed.mockResolvedValue(undefined)
    await ws.deleteSeed('s1')
    expect(ws.seeds).toHaveLength(0)
  })
})

describe('最近使用同步到服务端', () => {
  it('已登录时 record 会调用服务端记录接口', async () => {
    api.login.mockResolvedValue({ id: 'u1', username: 'alice' })
    api.recordRecent.mockResolvedValue(undefined)
    await useUserAuthStore().login('alice', 'password123')

    useRecentToolsStore().record('seed-tools', '种子工具', '种子')

    // recordRecent 是 fire-and-forget，等待微任务队列
    await Promise.resolve()
    expect(api.recordRecent).toHaveBeenCalledTimes(1)
    const arg = api.recordRecent.mock.calls[0][0]
    expect(arg.kind).toBe('tool')
    expect(arg.object_id).toBe('seed-tools')
    expect(arg.label).toBe('种子工具')
    expect(arg.url).toMatch(/^\//)
  })

  it('未登录时 record 不调用服务端', async () => {
    useRecentToolsStore().record('seed-tools', '种子工具', '种子')
    await Promise.resolve()
    expect(api.recordRecent).not.toHaveBeenCalled()
  })
})
