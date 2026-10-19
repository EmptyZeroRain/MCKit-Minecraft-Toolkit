<script setup lang="ts">
// 个人工作台（需求 §12）：收藏 / 最近使用 / 常用种子 / 用户信息
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSkeleton from '@/components/common/LoadingSkeleton.vue'
import { ApiError } from '@/types/api'
import {
  FAVORITE_KIND_LABELS,
  MAX_NICKNAME_LENGTH,
  MIN_USER_PASSWORD_LENGTH,
  type FavoriteKind,
  type UserFavoriteItem,
} from '@/types/user'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'
import { ALLOWED_AVATAR_TYPES, fileToAvatarDataUrl } from '@/utils/image'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '我的工作台',
  description: '管理收藏、最近使用与常用种子。',
  canonicalPath: '/workspace',
})

const router = useRouter()
const userAuth = useUserAuthStore()
const workspace = useUserWorkspaceStore()

const error = ref('')
const notice = ref('')
const busy = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const seedForm = reactive({ name: '', seed: '', note: '' })
const pwdForm = reactive({ oldPassword: '', newPassword: '', confirm: '' })
const deletePassword = ref('')

// 用户信息（昵称 / 头像）：头像仅允许上传图片。
const profileForm = reactive({ nickname: '', avatar_url: '' })
const profileError = ref('')
const savingProfile = ref(false)

const avatarBroken = ref(false)
const avatarPreview = computed(() => (avatarBroken.value ? '' : profileForm.avatar_url))
const avatarFallback = computed(() => userAuth.displayName.charAt(0).toUpperCase() || '?')

watch(
  () => profileForm.avatar_url,
  () => {
    avatarBroken.value = false
  },
)

const KIND_ORDER: FavoriteKind[] = [
  'tool',
  'item',
  'potion',
  'villager',
  'mob',
  'redstone_design',
  'mod',
  'modpack',
  'resource',
  'wiki',
]

/** 收藏按类型分组，空组不展示 */
const favoriteGroups = computed(() =>
  KIND_ORDER.map((kind) => ({
    kind,
    label: FAVORITE_KIND_LABELS[kind],
    items: workspace.favorites.filter((f) => f.kind === kind),
  })).filter((g) => g.items.length > 0),
)

function errText(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

function isInternal(url: string): boolean {
  return url.startsWith('/')
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('zh-CN')
}

onMounted(async () => {
  if (!userAuth.initialized) await userAuth.restoreSession()
  // 用当前账号资料回填用户信息表单。
  profileForm.nickname = userAuth.user?.nickname ?? ''
  profileForm.avatar_url = userAuth.user?.avatar_url ?? ''
  try {
    await workspace.loadAll()
  } catch (e) {
    error.value = errText(e, '个人数据加载失败')
  }
})

async function onAvatarChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  profileError.value = ''
  try {
    profileForm.avatar_url = await fileToAvatarDataUrl(file)
  } catch (err) {
    profileError.value = err instanceof Error ? err.message : '图片处理失败'
  }
}

function removeAvatar() {
  profileForm.avatar_url = ''
  profileError.value = ''
}

async function saveProfile() {
  error.value = ''
  notice.value = ''
  profileError.value = ''
  if ([...profileForm.nickname.trim()].length > MAX_NICKNAME_LENGTH) {
    profileError.value = `昵称长度不能超过 ${MAX_NICKNAME_LENGTH} 个字符`
    return
  }
  if (!profileForm.nickname.trim()) {
    profileError.value = '昵称不能为空'
    return
  }
  savingProfile.value = true
  try {
    await userAuth.updateProfile(profileForm.nickname.trim(), profileForm.avatar_url)
    notice.value = '资料已保存。'
  } catch (e) {
    profileError.value = errText(e, '保存失败')
  } finally {
    savingProfile.value = false
  }
}

async function removeFavorite(item: UserFavoriteItem) {
  error.value = ''
  try {
    await workspace.removeFavorite(item.kind, item.object_id)
  } catch (e) {
    error.value = errText(e, '取消收藏失败')
  }
}

async function clearRecents() {
  if (!window.confirm('确定清空全部最近使用记录吗？此操作不可撤销。')) return
  error.value = ''
  try {
    await workspace.clearRecents()
    notice.value = '最近使用已清空。'
  } catch (e) {
    error.value = errText(e, '清空失败')
  }
}

async function saveSeed() {
  error.value = ''
  notice.value = ''
  if (!seedForm.name.trim() || !seedForm.seed.trim()) {
    error.value = '名称与种子均为必填'
    return
  }
  busy.value = 'seed'
  try {
    await workspace.saveSeed({
      name: seedForm.name.trim(),
      seed: seedForm.seed.trim(),
      note: seedForm.note.trim(),
    })
    seedForm.name = ''
    seedForm.seed = ''
    seedForm.note = ''
    notice.value = '常用种子已保存。'
  } catch (e) {
    error.value = errText(e, '保存失败')
  } finally {
    busy.value = ''
  }
}

async function deleteSeed(id: string) {
  error.value = ''
  try {
    await workspace.deleteSeed(id)
  } catch (e) {
    error.value = errText(e, '删除失败')
  }
}

async function changePassword() {
  error.value = ''
  notice.value = ''
  if (pwdForm.newPassword.length < MIN_USER_PASSWORD_LENGTH) {
    error.value = `新密码长度至少 ${MIN_USER_PASSWORD_LENGTH} 位`
    return
  }
  if (pwdForm.newPassword !== pwdForm.confirm) {
    error.value = '两次输入的新密码不一致'
    return
  }
  busy.value = 'pwd'
  try {
    await userAuth.changePassword(pwdForm.oldPassword, pwdForm.newPassword)
    userAuth.reset()
    workspace.reset()
    await router.push('/login')
  } catch (e) {
    error.value = errText(e, '修改密码失败')
  } finally {
    busy.value = ''
  }
}

async function deleteAccount() {
  if (
    !window.confirm(
      '确定删除账号吗？你的收藏、最近使用与常用种子将被一并永久删除，且无法恢复。',
    )
  ) {
    return
  }
  error.value = ''
  busy.value = 'delete'
  try {
    await userAuth.deleteAccount(deletePassword.value)
    userAuth.reset()
    workspace.reset()
    await router.push('/')
  } catch (e) {
    error.value = errText(e, '删除账号失败（请确认密码正确）')
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <PageContainer maxWidth="max-w-4xl">
    <Breadcrumbs :items="[{ label: '我的工作台' }]" />
    <div class="mb-6">
      <h1 class="mb-1 text-2xl font-bold text-text-primary md:text-3xl">我的工作台</h1>
      <p class="text-sm text-text-secondary">
        当前账号：{{ userAuth.displayName }} ·
        <RouterLink to="/growth" class="text-grass-400 hover:underline">成长中心</RouterLink> ·
        <RouterLink to="/submit-resource" class="text-grass-400 hover:underline">资源投稿</RouterLink> ·
        <button type="button" class="text-grass-400 hover:underline" @click="userAuth.logout()">
          退出登录
        </button>
      </p>
    </div>

    <p v-if="error" class="mb-4 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc" role="alert">
      {{ error }}
    </p>
    <p v-if="notice" class="mb-4 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-3 text-sm text-grass-400" role="status">
      {{ notice }}
    </p>

    <LoadingSkeleton v-if="workspace.loading && !workspace.loaded" :count="3" />

    <template v-else>
      <!-- 收藏 -->
      <section class="mb-8">
        <h2 class="mb-3 text-lg font-bold text-text-primary">我的收藏（{{ workspace.favoriteCount }}）</h2>
        <div v-if="favoriteGroups.length" class="space-y-4">
          <div v-for="group in favoriteGroups" :key="group.kind" class="card p-4">
            <h3 class="mb-2 text-sm font-medium text-text-secondary">{{ group.label }}</h3>
            <ul class="divide-y divide-border-dark">
              <li v-for="item in group.items" :key="item.id" class="flex items-center justify-between gap-3 py-2">
                <div class="min-w-0">
                  <RouterLink v-if="isInternal(item.url)" :to="item.url" class="truncate text-sm text-text-primary hover:text-grass-300">
                    {{ item.label || item.object_id }}
                  </RouterLink>
                  <a v-else-if="item.url" :href="item.url" class="truncate text-sm text-text-primary hover:text-grass-300" target="_blank" rel="noopener noreferrer">
                    {{ item.label || item.object_id }}
                  </a>
                  <span v-else class="truncate text-sm text-text-primary">{{ item.label || item.object_id }}</span>
                  <p class="text-xs text-text-muted">{{ item.kind }} · {{ item.object_id }}</p>
                </div>
                <button
                  type="button"
                  class="shrink-0 rounded-md border border-border-light bg-ink-700 px-2.5 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
                  @click="removeFavorite(item)"
                >
                  取消收藏
                </button>
              </li>
            </ul>
          </div>
        </div>
        <EmptyState
          v-else
          title="还没有收藏"
          description="在工具、物品、资源或百科页面点击「收藏」即可在此查看。"
        />
      </section>

      <!-- 最近使用 -->
      <section class="mb-8">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-lg font-bold text-text-primary">最近使用（{{ workspace.recents.length }}）</h2>
          <button
            v-if="workspace.recents.length"
            type="button"
            class="rounded-md border border-border-light bg-ink-700 px-2.5 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
            @click="clearRecents"
          >
            清空记录
          </button>
        </div>
        <div v-if="workspace.recents.length" class="card divide-y divide-border-dark">
          <div v-for="item in workspace.recents" :key="item.id" class="flex items-center justify-between gap-3 px-4 py-2.5">
            <div class="min-w-0">
              <RouterLink v-if="isInternal(item.url)" :to="item.url" class="truncate text-sm text-text-primary hover:text-grass-300">
                {{ item.label || item.object_id }}
              </RouterLink>
              <span v-else class="truncate text-sm text-text-primary">{{ item.label || item.object_id }}</span>
              <p class="text-xs text-text-muted">{{ item.kind }}</p>
            </div>
            <span class="shrink-0 text-xs text-text-muted">{{ formatTime(item.visited_at) }}</span>
          </div>
        </div>
        <p v-else class="text-sm text-text-muted">暂无最近使用记录（最多保留最近 50 条）。</p>
      </section>

      <!-- 常用种子 -->
      <section class="mb-8">
        <h2 class="mb-3 text-lg font-bold text-text-primary">常用种子 / 查询记录（{{ workspace.seeds.length }}）</h2>
        <form class="card mb-3 grid gap-3 p-4 sm:grid-cols-3" @submit.prevent="saveSeed">
          <input v-model="seedForm.name" :class="inputClass" type="text" placeholder="名称（必填）" />
          <input v-model="seedForm.seed" :class="inputClass" type="text" placeholder="种子（必填）" />
          <input v-model="seedForm.note" :class="inputClass" type="text" placeholder="备注（可选）" />
          <button
            type="submit"
            class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60 sm:col-span-3 sm:w-auto sm:justify-self-start sm:px-6"
            :disabled="busy === 'seed'"
          >
            保存种子
          </button>
        </form>
        <div v-if="workspace.seeds.length" class="card divide-y divide-border-dark">
          <div v-for="seed in workspace.seeds" :key="seed.id" class="flex items-center justify-between gap-3 px-4 py-2.5">
            <div class="min-w-0">
              <p class="truncate text-sm text-text-primary">{{ seed.name }}</p>
              <p class="font-mono text-xs text-text-muted">{{ seed.seed }}{{ seed.note ? ` · ${seed.note}` : '' }}</p>
            </div>
            <button
              type="button"
              class="shrink-0 rounded-md border border-border-light bg-ink-700 px-2.5 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
              @click="deleteSeed(seed.id)"
            >
              删除
            </button>
          </div>
        </div>
        <p v-else class="text-sm text-text-muted">暂无保存的种子。</p>
      </section>

      <!-- 用户信息 -->
      <section>
        <h2 class="mb-3 text-lg font-bold text-text-primary">用户信息</h2>

        <div class="card mb-4 p-5">
          <h3 class="mb-3 text-sm font-medium text-text-secondary">个人资料</h3>
          <div class="mb-4 flex flex-col gap-4 sm:flex-row sm:items-center">
            <img
              v-if="avatarPreview"
              :src="avatarPreview"
              alt="头像预览"
              class="h-16 w-16 shrink-0 rounded-full border border-border-dark object-cover"
              @error="avatarBroken = true"
            />
            <div
              v-else
              class="flex h-16 w-16 shrink-0 items-center justify-center rounded-full border border-border-dark bg-ink-800 text-xl font-semibold text-text-muted"
              aria-hidden="true"
            >
              {{ avatarFallback }}
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <label
                class="inline-flex cursor-pointer items-center gap-2 rounded-md border border-border-light bg-ink-700 px-3 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
              >
                上传图片
                <input
                  type="file"
                  :accept="ALLOWED_AVATAR_TYPES"
                  class="sr-only"
                  @change="onAvatarChange"
                />
              </label>
              <button
                v-if="avatarPreview"
                type="button"
                class="rounded-md border border-danger-mc/40 px-3 py-2 text-sm text-danger-mc transition hover:bg-danger-mc/10"
                @click="removeAvatar"
              >
                移除头像
              </button>
            </div>
          </div>

          <form class="grid gap-3 sm:grid-cols-2" @submit.prevent="saveProfile">
            <input
              v-model="profileForm.nickname"
              :class="inputClass"
              type="text"
              :maxlength="MAX_NICKNAME_LENGTH"
              placeholder="昵称（必填）"
              required
            />
            <button
              type="submit"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary disabled:opacity-60 sm:col-span-2 sm:w-auto sm:justify-self-start sm:px-6"
              :disabled="savingProfile"
            >
              保存资料
            </button>
          </form>
          <p class="mt-2 text-xs text-text-muted">头像仅支持上传图片，建议使用方形图片。</p>
          <p v-if="profileError" class="mt-2 text-xs text-danger-mc" role="alert">
            {{ profileError }}
          </p>
        </div>

        <div class="card mb-4 p-5">
          <h3 class="mb-3 text-sm font-medium text-text-secondary">修改密码</h3>
          <form class="grid gap-3 sm:grid-cols-3" @submit.prevent="changePassword">
            <input v-model="pwdForm.oldPassword" :class="inputClass" type="password" autocomplete="current-password" placeholder="当前密码" required />
            <input v-model="pwdForm.newPassword" :class="inputClass" type="password" autocomplete="new-password" placeholder="新密码（至少 8 位）" required />
            <input v-model="pwdForm.confirm" :class="inputClass" type="password" autocomplete="new-password" placeholder="确认新密码" required />
            <button
              type="submit"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary disabled:opacity-60 sm:col-span-3 sm:w-auto sm:justify-self-start sm:px-6"
              :disabled="busy === 'pwd'"
            >
              修改密码
            </button>
          </form>
          <p class="mt-2 text-xs text-text-muted">修改成功后所有设备需用新密码重新登录。</p>
        </div>

        <div class="rounded-card border border-danger-mc/40 bg-danger-mc/5 p-5">
          <h3 class="mb-2 text-sm font-medium text-danger-mc">删除账号</h3>
          <p class="mb-3 text-xs text-text-secondary">
            删除后你的账号、收藏、最近使用与常用种子将被永久清除，无法恢复。请输入密码确认。
          </p>
          <form class="flex flex-col gap-3 sm:flex-row" @submit.prevent="deleteAccount">
            <input v-model="deletePassword" :class="inputClass" type="password" autocomplete="current-password" placeholder="当前密码" required />
            <button
              type="submit"
              class="shrink-0 rounded-md border border-danger-mc/60 bg-danger-mc/10 px-4 py-2 text-sm font-medium text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-60"
              :disabled="busy === 'delete'"
            >
              永久删除账号
            </button>
          </form>
        </div>
      </section>
    </template>
  </PageContainer>
</template>
