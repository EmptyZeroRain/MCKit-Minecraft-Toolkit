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
    await auth.login(form.username.trim(), form.password)
    ui.success('登录成功')
    const redirect = (route.query.redirect as string) || '/admin'
    router.push(redirect)
  } catch (err) {
    ui.error(err instanceof Error ? err.message : '登录失败，请检查用户名或密码')
  } finally {
    submitting.value = false
  }
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
      </form>

      <div class="mt-4 text-center text-sm">
        <RouterLink to="/" class="text-grass-400 hover:underline">← 返回前台</RouterLink>
      </div>
    </div>
  </div>
</template>