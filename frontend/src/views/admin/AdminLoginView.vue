<script setup lang="ts">
// 管理员登录（需求 6.1）
import { reactive, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import FormField from '@/components/common/FormField.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { applyNoIndexMeta } from '@/utils/seo'

applyNoIndexMeta()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const ui = useUiStore()

const form = reactive({ username: '', password: '' })
const errors = reactive<Record<string, string>>({})
const submitting = ref(false)

// 第二因子：密码通过后若账号启用了 MFA，进入验证码步骤（此时尚未登录）。
const step = ref<'password' | 'mfa'>('password')
const mfaTicket = ref('')
const mfaCode = ref('')
const mfaError = ref('')

function validate(): boolean {
  errors.username = ''
  errors.password = ''
  let ok = true
  if (!form.username.trim()) {
    errors.username = '请输入用户名'
    ok = false
  }
  if (!form.password) {
    errors.password = '请输入密码'
    ok = false
  }
  return ok
}

async function handleLogin() {
  if (!validate() || submitting.value) return
  submitting.value = true
  try {
    const outcome = await auth.login(form.username.trim(), form.password)
    if (outcome.mfa_required) {
      mfaTicket.value = outcome.mfa_ticket
      mfaCode.value = ''
      mfaError.value = ''
      step.value = 'mfa'
      return
    }
    ui.success('登录成功')
    const redirect = (route.query.redirect as string) || '/admin'
    router.push(redirect)
  } catch (err) {
    ui.error(err instanceof Error ? err.message : '登录失败，请检查用户名或密码')
  } finally {
    submitting.value = false
  }
}

async function handleVerify() {
  if (submitting.value) return
  const code = mfaCode.value.trim()
  if (!code) {
    mfaError.value = '请输入验证码'
    return
  }
  mfaError.value = ''
  submitting.value = true
  try {
    await auth.verifyMFA(mfaTicket.value, code)
    ui.success('登录成功')
    const redirect = (route.query.redirect as string) || '/admin'
    router.push(redirect)
  } catch (err) {
    // 票据为一次性 / 有限次重试，失效时需要重新输入密码。
    mfaError.value = err instanceof Error ? err.message : '验证码校验失败'
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
  form.password = ''
}
</script>

<template>
  <div class="mc-grid-bg flex min-h-screen items-center justify-center bg-ink-950 px-4">
    <div class="w-full max-w-sm">
      <div class="mb-8 text-center">
        <img src="/favicon.svg" alt="" class="mx-auto mb-3 h-14 w-14" />
        <h1 class="text-xl font-bold text-text-primary">MC 工具站 · 管理后台</h1>
        <p class="mt-1 text-sm text-text-secondary">请使用管理员账号登录</p>
      </div>

      <form class="card space-y-5 p-6" novalidate @submit.prevent="handleLogin">
        <template v-if="step === 'password'">
          <FormField label="用户名" required :error="errors.username">
            <template #default="{ id }">
              <input
                :id="id"
                v-model="form.username"
                type="text"
                autocomplete="username"
                placeholder="管理员用户名"
                class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              />
            </template>
          </FormField>

          <FormField label="密码" required :error="errors.password">
            <template #default="{ id }">
              <input
                :id="id"
                v-model="form.password"
                type="password"
                autocomplete="current-password"
                placeholder="密码"
                class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              />
            </template>
          </FormField>

          <button
            type="submit"
            class="w-full rounded-md bg-grass-600 py-2.5 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="submitting"
          >
            {{ submitting ? '登录中…' : '登录' }}
          </button>
        </template>

        <template v-else>
          <div class="text-sm text-text-secondary">
            账号已启用双因子认证，请输入认证器 App 中的 6 位验证码；也可使用一次性恢复码。
          </div>

          <FormField label="验证码" required :error="mfaError">
            <template #default="{ id }">
              <input
                :id="id"
                v-model="mfaCode"
                type="text"
                inputmode="numeric"
                autocomplete="one-time-code"
                placeholder="6 位验证码或恢复码"
                class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm tracking-[0.2em] text-text-primary placeholder:tracking-normal placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
              />
            </template>
          </FormField>

          <button
            type="submit"
            class="w-full rounded-md bg-grass-600 py-2.5 font-medium text-white transition hover:bg-grass-500 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="submitting"
            @click.prevent="handleVerify"
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
        </template>
      </form>

      <div class="mt-4 text-center text-sm">
        <RouterLink to="/" class="text-grass-400 hover:underline">← 返回前台</RouterLink>
      </div>
    </div>
  </div>
</template>