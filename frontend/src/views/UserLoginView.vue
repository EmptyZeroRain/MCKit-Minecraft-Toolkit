<script setup lang="ts">
// 统一登录 / 注册（需求 §12）
// 说明：登录态使用服务端 HttpOnly Cookie；前端不保存令牌。
// 普通用户与后台账号共用同一登录入口：服务端按账号角色分流，
// 后台账号若启用 MFA，会在密码通过后进入第二因子步骤。
// 注册需填写昵称、邮箱并通过图形验证码（验证码一次性，失败后需重新获取）。
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import { ApiError } from '@/types/api'
import {
  EMAIL_RE,
  MAX_NICKNAME_LENGTH,
  MIN_USER_PASSWORD_LENGTH,
  USERNAME_RE,
} from '@/types/user'
import { fetchCaptcha } from '@/api/user'
import type { UnifiedLoginSuccess } from '@/api/user'
import { useUserAuthStore } from '@/stores/user-auth'
import { useAuthStore } from '@/stores/auth'
import { applyPageMeta } from '@/utils/seo'
import type { AdminUser } from '@/types/admin'

applyPageMeta({
  title: '登录 / 注册',
  description: '普通用户与管理员共用登录入口。登录后可收藏工具、物品、资源与百科，并同步最近使用与常用种子。',
  canonicalPath: '/login',
})

const route = useRoute()
const router = useRouter()
const userAuth = useUserAuthStore()
const auth = useAuthStore()

const mode = ref<'login' | 'register'>('login')
// 后台账号启用 MFA 时的第二因子步骤（此时尚未登录）。
const step = ref<'password' | 'mfa'>('password')
const username = ref('')
const nickname = ref('')
const email = ref('')
const password = ref('')
const passwordConfirm = ref('')
const submitting = ref(false)
const error = ref('')
const notice = ref('')

// 图形验证码
const captchaId = ref('')
const captchaImage = ref('')
const captchaAnswer = ref('')
const captchaLoading = ref(false)
const captchaError = ref('')

// 第二因子
const mfaTicket = ref('')
const mfaCode = ref('')
const mfaError = ref('')

const inputClass =
  'w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none'

const title = computed(() => {
  if (step.value === 'mfa') return '双因子验证'
  return mode.value === 'login' ? '登录' : '注册新账号'
})

/** 仅允许站内绝对路径，避免 redirect 参数被用于协议相对跳转。 */
function safeRedirect(value: unknown): string {
  if (typeof value !== 'string') return ''
  if (!value.startsWith('/') || value.startsWith('//')) return ''
  return value
}

/** 获取（或刷新）图形验证码；一次性，校验后失效 */
async function loadCaptcha() {
  captchaLoading.value = true
  captchaError.value = ''
  try {
    const challenge = await fetchCaptcha()
    captchaId.value = challenge.captcha_id
    captchaImage.value = challenge.image
    captchaAnswer.value = ''
  } catch {
    captchaId.value = ''
    captchaImage.value = ''
    captchaError.value = '验证码加载失败，请点击重试'
  } finally {
    captchaLoading.value = false
  }
}

function switchMode(next: 'login' | 'register') {
  mode.value = next
  step.value = 'password'
  error.value = ''
  notice.value = ''
  password.value = ''
  passwordConfirm.value = ''
  mfaTicket.value = ''
  mfaCode.value = ''
  mfaError.value = ''

  // 让「注册」直达链接（/login?mode=register）与页签状态保持一致。
  const shouldBeRegister = next === 'register'
  if (shouldBeRegister !== (route.query.mode === 'register')) {
    const query = { ...route.query }
    if (shouldBeRegister) query.mode = 'register'
    else delete query.mode
    void router.replace({ query })
  }

  if (next === 'register' && !captchaImage.value) {
    void loadCaptcha()
  }
}

/** 客户端预校验（后端仍会独立校验） */
function validate(): string {
  const name = username.value.trim()
  if (!name) {
    return mode.value === 'login' ? '请输入用户名或邮箱' : '请输入用户名'
  }
  if (!password.value) {
    return '请输入密码'
  }
  if (mode.value === 'register') {
    if (!USERNAME_RE.test(name)) {
      return '用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符'
    }
    if (password.value.length < MIN_USER_PASSWORD_LENGTH) {
      return `密码长度至少 ${MIN_USER_PASSWORD_LENGTH} 位`
    }
    const nick = nickname.value.trim()
    if (!nick || Array.from(nick).length > MAX_NICKNAME_LENGTH) {
      return `昵称需为 1-${MAX_NICKNAME_LENGTH} 个字符`
    }
    if (!EMAIL_RE.test(email.value.trim())) {
      return '请输入有效的邮箱地址'
    }
    if (password.value !== passwordConfirm.value) {
      return '两次输入的密码不一致'
    }
    if (!captchaAnswer.value.trim()) {
      return '请输入图形验证码'
    }
  }
  return ''
}

function errText(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

/** 登录成功后的跳转：后台账号默认进入后台，普通用户进入工作台。 */
async function completeLogin(role: string, session?: UnifiedLoginSuccess) {
  const target = safeRedirect(route.query.redirect)
  if (role !== 'user') {
    if (session) {
      // 统一登录响应与 /admin/auth/login 同构，直接写入后台登录态。
      auth.applySession(session as unknown as AdminUser)
    }
    await router.push(target || '/admin')
    return
  }
  const canReturn = target && !target.startsWith('/admin') && !target.startsWith('/login')
  await router.push(canReturn ? target : '/workspace')
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
      await userAuth.register({
        username: username.value.trim(),
        nickname: nickname.value.trim(),
        email: email.value.trim(),
        password: password.value,
        captcha_id: captchaId.value,
        captcha_answer: captchaAnswer.value.trim(),
      })
      switchMode('login')
      username.value = username.value.trim()
      notice.value = '注册成功，请使用新账号登录。'
      return
    }

    const outcome = await userAuth.loginUnified(username.value.trim(), password.value)
    if ('mfa_required' in outcome) {
      mfaTicket.value = outcome.mfa_ticket
      mfaCode.value = ''
      mfaError.value = ''
      step.value = 'mfa'
      return
    }
    await completeLogin(outcome.role, outcome)
  } catch (e) {
    error.value = errText(e, mode.value === 'login' ? '登录失败，请稍后重试' : '注册失败，请稍后重试')
    // 验证码一次性：任何到达服务端的注册请求都会消耗它，失败后必须重新获取。
    if (mode.value === 'register') {
      void loadCaptcha()
    }
  } finally {
    submitting.value = false
  }
}

/** 第二因子校验（后台账号）：成功后回填前台 / 后台登录态并跳转。 */
async function submitMfa() {
  if (submitting.value) return
  const code = mfaCode.value.trim()
  if (!code) {
    mfaError.value = '请输入验证码'
    return
  }
  mfaError.value = ''
  submitting.value = true
  try {
    const admin = await auth.verifyMFA(mfaTicket.value, code)
    await userAuth.reloadSession()
    await completeLogin(admin.role)
  } catch (e) {
    // 票据为一次性 / 有限次重试，失效时需要重新输入密码。
    mfaError.value = errText(e, '验证码校验失败')
  } finally {
    submitting.value = false
  }
}

/** 返回上一步：丢弃票据，重新输入密码（避免在过期票据上反复尝试）。 */
function backToPassword() {
  step.value = 'password'
  mfaTicket.value = ''
  mfaCode.value = ''
  mfaError.value = ''
  password.value = ''
}

onMounted(() => {
  if (route.query.mode === 'register') {
    switchMode('register')
    return
  }
  if (mode.value === 'register') {
    void loadCaptcha()
  }
})
</script>

<template>
  <PageContainer maxWidth="max-w-md">
    <Breadcrumbs :items="[{ label: '登录 / 注册' }]" />
    <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">
      {{ title }}
    </h1>
    <p class="mb-6 text-sm text-text-secondary">
      <template v-if="step === 'mfa'">
        账号已启用双因子认证，请输入认证器 App 中的 6 位验证码；也可使用一次性恢复码。
      </template>
      <template v-else>
        普通用户与管理员共用此入口，管理员登录后自动进入管理后台。登录后可在
        <RouterLink to="/workspace" class="text-grass-400 hover:underline">个人工作台</RouterLink>
        收藏工具、物品、资源与百科，并同步最近使用与常用种子。不登录也可正常使用全部公开工具。
      </template>
    </p>

    <div class="card p-5">
      <!-- 第二因子：密码通过后进入，此时尚未建立会话 -->
      <form v-if="step === 'mfa'" class="space-y-4" @submit.prevent="submitMfa">
        <div>
          <label for="user-mfa-code" class="mb-1.5 block text-sm font-medium text-text-primary">
            验证码
          </label>
          <input
            id="user-mfa-code"
            v-model="mfaCode"
            :class="inputClass"
            type="text"
            autocomplete="one-time-code"
            placeholder="6 位验证码或恢复码"
            required
          />
        </div>

        <p v-if="mfaError" class="text-sm text-danger-mc" role="alert">{{ mfaError }}</p>

        <button
          type="submit"
          class="w-full rounded-md bg-grass-600 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-60"
          :disabled="submitting"
          @click.prevent="submitMfa"
        >
          {{ submitting ? '校验中…' : '验证并登录' }}
        </button>
        <button
          type="button"
          class="w-full text-center text-sm text-text-secondary hover:text-text-primary"
          @click="backToPassword"
        >
          返回重新输入密码
        </button>
      </form>

      <template v-else>
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
              {{ mode === 'login' ? '用户名 / 邮箱' : '用户名' }}
            </label>
            <input
              id="user-username"
              v-model="username"
              :class="inputClass"
              type="text"
              autocomplete="username"
              :placeholder="mode === 'login' ? '用户名、注册邮箱或管理员账号' : '3-40 位，字母开头'"
              required
            />
          </div>

          <div v-if="mode === 'register'">
            <label for="user-nickname" class="mb-1.5 block text-sm font-medium text-text-primary">
              昵称
            </label>
            <input
              id="user-nickname"
              v-model="nickname"
              :class="inputClass"
              type="text"
              :maxlength="MAX_NICKNAME_LENGTH"
              placeholder="1-60 个字符，将作为展示名"
              required
            />
          </div>

          <div v-if="mode === 'register'">
            <label for="user-email" class="mb-1.5 block text-sm font-medium text-text-primary">
              邮箱
            </label>
            <input
              id="user-email"
              v-model="email"
              :class="inputClass"
              type="email"
              autocomplete="email"
              placeholder="用于登录与账号找回，全局唯一"
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

          <div v-if="mode === 'register'">
            <label for="user-captcha" class="mb-1.5 block text-sm font-medium text-text-primary">
              图形验证码
            </label>
            <div class="flex items-center gap-3">
              <input
                id="user-captcha"
                v-model="captchaAnswer"
                :class="inputClass"
                type="text"
                autocomplete="off"
                placeholder="输入图中字符"
                maxlength="8"
                required
              />
              <button
                type="button"
                class="h-[38px] shrink-0 overflow-hidden rounded-md border border-border-dark bg-ink-900 px-1"
                title="点击刷新验证码"
                aria-label="刷新验证码"
                :disabled="captchaLoading"
                @click="loadCaptcha"
              >
                <img
                  v-if="captchaImage"
                  :src="captchaImage"
                  alt="图形验证码"
                  class="h-full w-auto"
                />
                <span v-else class="px-3 text-xs text-text-muted">
                  {{ captchaLoading ? '加载中…' : '点击获取' }}
                </span>
              </button>
            </div>
            <p v-if="captchaError" class="mt-1 text-xs text-danger-mc">{{ captchaError }}</p>
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
      </template>

      <p class="mt-4 text-xs text-text-muted">
        我们仅保存用户名、昵称、邮箱与密码哈希，以及你主动创建的收藏 / 最近使用 / 常用种子。
        邮箱仅用于登录与账号找回，不会公开展示。
        可在账户设置中随时清理或删除账号（删除后个人数据一并清除）。
        详见
        <RouterLink to="/privacy" class="text-grass-400 hover:underline">隐私政策</RouterLink>。
      </p>
    </div>
  </PageContainer>
</template>
