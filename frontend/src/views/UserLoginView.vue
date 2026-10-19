<script setup lang="ts">
// 普通用户登录 / 注册（需求 §12）
// 说明：登录态使用服务端 HttpOnly Cookie；前端不保存令牌。
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import { ApiError } from '@/types/api'
import { MIN_USER_PASSWORD_LENGTH, USERNAME_RE } from '@/types/user'
import { useUserAuthStore } from '@/stores/user-auth'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '登录 / 注册',
  description: '注册并登录后可在个人工作台收藏工具、物品、资源与百科，并同步最近使用与常用种子。',
  canonicalPath: '/login',
})

const route = useRoute()
const router = useRouter()
const userAuth = useUserAuthStore()

const mode = ref<'login' | 'register'>('login')
const username = ref('')
const password = ref('')
const passwordConfirm = ref('')
const submitting = ref(false)
const error = ref('')
const notice = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const title = computed(() => (mode.value === 'login' ? '登录' : '注册新账号'))

function switchMode(next: 'login' | 'register') {
  mode.value = next
  error.value = ''
  notice.value = ''
  password.value = ''
  passwordConfirm.value = ''
}

/** 客户端预校验（后端仍会独立校验） */
function validate(): string {
  const name = username.value.trim()
  if (!USERNAME_RE.test(name)) {
    return '用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符'
  }
  if (password.value.length < MIN_USER_PASSWORD_LENGTH) {
    return `密码长度至少 ${MIN_USER_PASSWORD_LENGTH} 位`
  }
  if (mode.value === 'register' && password.value !== passwordConfirm.value) {
    return '两次输入的密码不一致'
  }
  return ''
}

function errText(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

async function submit() {
  if (submitting.value) return
  error.value = ''
  notice.value = ''
  const invalid = validate()
  if (invalid) {
    error.value = invalid
    return
  }

  submitting.value = true
  try {
    if (mode.value === 'register') {
      await userAuth.register(username.value.trim(), password.value)
      switchMode('login')
      username.value = username.value.trim()
      notice.value = '注册成功，请使用新账号登录。'
      return
    }

    await userAuth.login(username.value.trim(), password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/workspace'
    await router.push(redirect)
  } catch (e) {
    error.value = errText(e, mode.value === 'login' ? '登录失败，请稍后重试' : '注册失败，请稍后重试')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <PageContainer maxWidth="max-w-md">
    <Breadcrumbs :items="[{ label: '登录 / 注册' }]" />
    <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">
      {{ title }}
    </h1>
    <p class="mb-6 text-sm text-text-secondary">
      登录后可在
      <RouterLink to="/workspace" class="text-grass-400 hover:underline">个人工作台</RouterLink>
      收藏工具、物品、资源与百科，并同步最近使用与常用种子。不登录也可正常使用全部公开工具。
    </p>

    <div class="card p-5">
      <!-- 模式切换 -->
      <div class="mb-5 inline-flex rounded-md border border-border-dark p-0.5" role="tablist">
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'login'"
          class="rounded px-4 py-1.5 text-sm transition"
          :class="mode === 'login' ? 'bg-ink-700 text-grass-300' : 'text-text-secondary hover:text-text-primary'"
          @click="switchMode('login')"
        >
          登录
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="mode === 'register'"
          class="rounded px-4 py-1.5 text-sm transition"
          :class="mode === 'register' ? 'bg-ink-700 text-grass-300' : 'text-text-secondary hover:text-text-primary'"
          @click="switchMode('register')"
        >
          注册
        </button>
      </div>

      <form class="space-y-4" @submit.prevent="submit">
        <div>
          <label for="user-username" class="mb-1.5 block text-sm font-medium text-text-primary">
            用户名
          </label>
          <input
            id="user-username"
            v-model="username"
            :class="inputClass"
            type="text"
            autocomplete="username"
            placeholder="3-40 位，字母开头"
            required
          />
        </div>
        <div>
          <label for="user-password" class="mb-1.5 block text-sm font-medium text-text-primary">
            密码
          </label>
          <input
            id="user-password"
            v-model="password"
            :class="inputClass"
            type="password"
            :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
            placeholder="至少 8 位"
            required
          />
        </div>
        <div v-if="mode === 'register'">
          <label for="user-password2" class="mb-1.5 block text-sm font-medium text-text-primary">
            确认密码
          </label>
          <input
            id="user-password2"
            v-model="passwordConfirm"
            :class="inputClass"
            type="password"
            autocomplete="new-password"
            required
          />
        </div>

        <p v-if="error" class="text-sm text-danger-mc" role="alert">{{ error }}</p>
        <p v-if="notice" class="text-sm text-grass-400" role="status">{{ notice }}</p>

        <button
          type="submit"
          class="w-full rounded-md bg-grass-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          :disabled="submitting"
        >
          {{ submitting ? '处理中…' : title }}
        </button>
      </form>

      <p class="mt-4 text-xs text-text-muted">
        我们仅保存用户名与密码哈希，以及你主动创建的收藏 / 最近使用 / 常用种子。
        可在工作台随时清理或删除账号（删除后个人数据一并清除）。
        详见
        <RouterLink to="/privacy" class="text-grass-400 hover:underline">隐私政策</RouterLink>。
      </p>
    </div>
  </PageContainer>
</template>
