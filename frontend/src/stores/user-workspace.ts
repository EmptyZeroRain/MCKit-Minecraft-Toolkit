// ===== 个人工作台数据（收藏 / 最近使用 / 常用种子，需求 §12）=====
// 仅登录用户使用；数据由后端按当前会话用户隔离存储。
import { defineStore } from 'pinia'
import type {
  FavoriteKind,
  SaveSeedPayload,
  UserFavoriteItem,
  UserRecentItem,
  UserSeed,
} from '@/types/user'
import * as userApi from '@/api/user'

export const useUserWorkspaceStore = defineStore('user-workspace', {
  state: () => ({
    favorites: [] as UserFavoriteItem[],
    recents: [] as UserRecentItem[],
    seeds: [] as UserSeed[],
    loaded: false,
    loading: false,
  }),
  getters: {
    isFavorite: (state) => (kind: FavoriteKind, objectId: string) =>
      state.favorites.some((f) => f.kind === kind && f.object_id === objectId),
    favoriteCount: (state) => state.favorites.length,
  },
  actions: {
    async loadFavorites(): Promise<void> {
      this.favorites = await userApi.listFavorites()
    },
    /** 一次拉取全部个人数据（工作台页使用） */
    async loadAll(): Promise<void> {
      this.loading = true
      try {
        const [favorites, recents, seeds] = await Promise.all([
          userApi.listFavorites(),
          userApi.listRecents(),
          userApi.listSeeds(),
        ])
        this.favorites = favorites
        this.recents = recents
        this.seeds = seeds
        this.loaded = true
      } finally {
        this.loading = false
      }
    },
    /** 切换收藏状态，返回操作后是否已收藏 */
    async toggle(
      kind: FavoriteKind,
      objectId: string,
      label: string,
      url: string,
    ): Promise<boolean> {
      const favorited = await userApi.toggleFavorite({
        kind,
        object_id: objectId,
        label,
        url,
      })
      await this.loadFavorites()
      return favorited
    },
    async removeFavorite(kind: FavoriteKind, objectId: string): Promise<void> {
      await userApi.removeFavorite(kind, objectId)
      this.favorites = this.favorites.filter(
        (f) => !(f.kind === kind && f.object_id === objectId),
      )
    },
    /** 记录最近使用（失败静默：不阻塞浏览） */
    async recordRecent(
      kind: FavoriteKind,
      objectId: string,
      label: string,
      url: string,
    ): Promise<void> {
      try {
        await userApi.recordRecent({ kind, object_id: objectId, label, url })
      } catch {
        // 静默失败：最近使用属于辅助数据
      }
    },
    async clearRecents(): Promise<void> {
      await userApi.clearRecents()
      this.recents = []
    },
    async saveSeed(payload: SaveSeedPayload): Promise<void> {
      await userApi.saveSeed(payload)
      this.seeds = await userApi.listSeeds()
    },
    async deleteSeed(id: string): Promise<void> {
      await userApi.deleteSeed(id)
      this.seeds = this.seeds.filter((s) => s.id !== id)
    },
    reset() {
      this.favorites = []
      this.recents = []
      this.seeds = []
      this.loaded = false
    },
  },
})
