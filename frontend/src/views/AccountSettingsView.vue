<script setup lang="ts">
// 账户设置：从工作台迁移出的个人资料、密码与删除账号操作。
// 复用现有用户接口与业务规则，不新增账号能力。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import { ApiError } from '@/types/api'
import { MAX_NICKNAME_LENGTH, MIN_USER_PASSWORD_LENGTH } from '@/types/user'
import { useUserAuthStore } from '@/stores/user-auth'
import { useUserWorkspaceStore } from '@/stores/user-workspace'
import { ALLOWED_AVATAR_TYPES, fileToAvatarDataUrl } from '@/utils/image'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '账户设置',
  description: '管理个人资料、密码与账号安全。',
  canonicalPath: '/settings',
})

const router = useRouter()
const userAuth = useUserAuthStore()
const workspace = useUserWorkspaceStore()

const inputClass =
  'w-full min-w-0 rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const profileForm = reactive({ nickname: '', avatar_url: '' })
const pwdForm = reactive({ oldPassword: '', newPassword: '', confirm: '' })
const deletePassword = ref('')
const error = ref('')
const notice = ref('')
const busy = ref('')
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

onMounted(async () => {
  if (!userAuth.initialized) await userAuth.restoreSession()
  profileForm.nickname = userAuth.user?.nickname ?? ''
  profileForm.avatar_url = userAuth.user?.avatar_url ?? ''
})

function errText(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

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
    <Breadcrumbs :items="[{ label: '我的工作台', to: '/workspace' }, { label: '账户设置' }]" />

    <header class="mb-6">
      <h1 class="text-2xl font-bold tracking-tight text-text-primary md:text-3xl">账户设置</h1>
      <p class="mt-1.5 text-sm text-text-secondary">管理个人资料、密码与账号安全操作。</p>
    </header>

    <p
      v-if="error"
      class="mb-5 rounded-card border border-danger-mc/40 bg-danger-mc/10 px-4 py-3 text-sm text-danger-mc"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-if="notice"
      class="mb-5 rounded-card border border-grass-500/40 bg-grass-500/10 px-4 py-3 text-sm text-grass-400"
      role="status"
    >
      {{ notice }}
    </p>

    <div class="space-y-5">
      <section class="card p-5 sm:p-6" aria-labelledby="settings-profile-title">
        <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 id="settings-profile-title" class="text-base font-semibold text-text-primary">
              个人资料
            </h2>
            <p class="mt-0.5 text-xs text-text-muted">头像与昵称会展示在社区和个人资料中。</p>
          </div>
          <RouterLink to="/workspace" class="text-xs text-grass-400 hover:underline">
            返回工作台
          </RouterLink>
        </div>

        <div class="flex flex-col gap-4 sm:flex-row sm:items-center">
          <div
            class="flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-grass-500/30 bg-ink-700 ring-4 ring-grass-500/10"
          >
            <img
              v-if="avatarPreview"
              :src="avatarPreview"
              alt="头像预览"
              class="h-full w-full object-cover"
              @error="avatarBroken = true"
            />
            <span v-else class="text-xl font-bold text-grass-400" aria-hidden="true">
              {{ avatarFallback }}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <label
              class="inline-flex h-9 cursor-pointer items-center rounded-md border border-border-light bg-ink-700 px-3 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
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
              class="h-9 rounded-md border border-danger-mc/40 px-3 text-sm text-danger-mc transition hover:bg-danger-mc/10"
              @click="removeAvatar"
            >
              移除头像
            </button>
            <span class="text-xs text-text-muted">支持常见图片格式，建议使用方形图片。</span>
          </div>
        </div>

        <form class="mt-5 grid gap-3 sm:grid-cols-[1fr_auto]" @submit.prevent="saveProfile">
          <label>
            <span class="mb-1 block text-xs font-medium text-text-secondary">昵称</span>
            <input
              v-model="profileForm.nickname"
              :class="inputClass"
              type="text"
              :maxlength="MAX_NICKNAME_LENGTH"
              placeholder="昵称（必填）"
              required
            />
          </label>
          <div class="flex items-end">
            <button
              type="submit"
              class="h-[38px] w-full rounded-md bg-grass-600 px-5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60 sm:w-auto"
              :disabled="savingProfile"
            >
              保存资料
            </button>
          </div>
        </form>
        <p v-if="profileError" class="mt-2 text-xs text-danger-mc" role="alert">
          {{ profileError }}
        </p>
      </section>

      <section class="card p-5 sm:p-6" aria-labelledby="settings-password-title">
        <h2 id="settings-password-title" class="text-base font-semibold text-text-primary">
          修改密码
        </h2>
        <p class="mt-0.5 text-xs text-text-muted">修改成功后所有设备需使用新密码重新登录。</p>

        <form class="mt-4 grid gap-3 md:grid-cols-3" @submit.prevent="changePassword">
          <label>
            <span class="mb-1 block text-xs font-medium text-text-secondary">当前密码</span>
            <input
              v-model="pwdForm.oldPassword"
              :class="inputClass"
              type="password"
              autocomplete="current-password"
              placeholder="当前密码"
              required
            />
          </label>
          <label>
            <span class="mb-1 block text-xs font-medium text-text-secondary">新密码</span>
            <input
              v-model="pwdForm.newPassword"
              :class="inputClass"
              type="password"
              autocomplete="new-password"
              :placeholder="`新密码（至少 ${MIN_USER_PASSWORD_LENGTH} 位）`"
              required
            />
          </label>
          <label>
            <span class="mb-1 block text-xs font-medium text-text-secondary">确认新密码</span>
            <input
              v-model="pwdForm.confirm"
              :class="inputClass"
              type="password"
              autocomplete="new-password"
              placeholder="再次输入新密码"
              required
            />
          </label>
          <div class="md:col-span-3">
            <button
              type="submit"
              class="h-[38px] rounded-md border border-border-light bg-ink-700 px-5 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary disabled:opacity-60"
              :disabled="busy === 'pwd'"
            >
              修改密码
            </button>
          </div>
        </form>
      </section>

      <section
        class="rounded-card border border-danger-mc/40 bg-danger-mc/5 p-5 sm:p-6"
        aria-labelledby="settings-delete-title"
      >
        <h2 id="settings-delete-title" class="text-base font-semibold text-danger-mc">
          删除账号
        </h2>
        <p class="mt-1 max-w-2xl text-xs leading-relaxed text-text-secondary">
          删除后你的账号、收藏、最近使用与常用种子将被永久清除，无法恢复。请输入当前密码确认。
        </p>
        <form class="mt-4 flex flex-col gap-3 sm:flex-row" @submit.prevent="deleteAccount">
          <label class="min-w-0 flex-1 sm:max-w-xs">
            <span class="mb-1 block text-xs font-medium text-text-secondary">当前密码</span>
            <input
              v-model="deletePassword"
              :class="inputClass"
              type="password"
              autocomplete="current-password"
              placeholder="当前密码"
              required
            />
          </label>
          <div class="flex items-end">
            <button
              type="submit"
              class="h-[38px] w-full rounded-md border border-danger-mc/60 bg-danger-mc/10 px-4 text-sm font-medium text-danger-mc transition hover:bg-danger-mc/20 disabled:opacity-60 sm:w-auto"
              :disabled="busy === 'delete'"
            >
              永久删除账号
            </button>
          </div>
        </form>
      </section>
    </div>
  </PageContainer>
</template>
