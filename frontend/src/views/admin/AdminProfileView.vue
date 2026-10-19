<script setup lang="ts">
// 账号设置：当前管理员自助修改密码（需求 6.1）
// 成功后后端会使该管理员全部会话失效，此处清理本地状态并跳转登录页。
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import FormField from '@/components/common/FormField.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { changePassword } from '@/api/admin-auth'
import { setCsrfToken } from '@/api/http'
import { ApiError } from '@/types/api'

const router = useRouter()
const auth = useAuthStore()
const ui = useUiStore()

const form = reactive({ old_password: '', new_password: '', confirm: '' })
const errors = reactive<Record<string, string>>({})
const submitting = ref(false)

function validate(): boolean {
  errors.old_password = ''
  errors.new_password = ''
  errors.confirm = ''
  let ok = true
  if (!form.old_password) {
    errors.old_password = '请输入当前密码'
    ok = false
  }
  if (form.new_password.length < 8) {
    errors.new_password = '新密码长度至少 8 位'
    ok = false
  }
  if (form.new_password !== form.confirm) {
    errors.confirm = '两次输入的新密码不一致'
    ok = false
  }
  return ok
}

async function handleSubmit() {
  if (!validate() || submitting.value) return
  submitting.value = true
  try {
    await changePassword({ old_password: form.old_password, new_password: form.new_password })
    ui.success('密码已修改，请使用新密码重新登录')
    // 会话已被服务端全部注销，清理本地状态后回到登录页。
    setCsrfToken('')
    auth.reset()
    router.push({ path: '/admin/login' })
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '修改失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="max-w-lg">
    <div class="card p-6">
      <h2 class="mb-1 text-lg font-semibold text-text-primary">修改密码</h2>
      <p class="mb-5 text-sm text-text-secondary">
        修改成功后，当前账号在所有设备上的登录都会失效，需要用新密码重新登录。
      </p>

      <form class="space-y-4" novalidate @submit.prevent="handleSubmit">
        <FormField label="当前密码" required :error="errors.old_password">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.old_password"
              type="password"
              autocomplete="current-password"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField label="新密码（≥8 位）" required :error="errors.new_password">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.new_password"
              type="password"
              autocomplete="new-password"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField label="确认新密码" required :error="errors.confirm">
          <template #default="{ id }">
            <input
              :id="id"
              v-model="form.confirm"
              type="password"
              autocomplete="new-password"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>

        <button
          type="submit"
          class="rounded-md bg-grass-600 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
          :disabled="submitting"
        >
          {{ submitting ? '提交中…' : '修改密码' }}
        </button>
      </form>
    </div>
  </div>
</template>
