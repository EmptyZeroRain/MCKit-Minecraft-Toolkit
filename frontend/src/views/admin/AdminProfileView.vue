<script setup lang="ts">
// 用户信息：后台管理账号自身的安全设置（需求 6.1）。
//
// 只保留三块与「账号安全」直接相关的内容：
//   - 修改密码：改密成功后后端使该账号全部会话失效；
//   - 双因子认证：TOTP 绑定 / 解绑；
//   - 登录历史：最近登录的设备、IP、时间与结果（含失败尝试）。
//
// 刻意不放昵称 / 头像等展示资料：这里是后台管理账号，不是社区前台个人主页。
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminTabs from '@/components/admin/AdminTabs.vue'
import FormField from '@/components/common/FormField.vue'
import { useAdminTabs } from '@/composables/useAdminTabs'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import {
  changePassword,
  listLoginHistory,
  mfaDisable,
  mfaEnable,
  mfaSetup,
  mfaStatus,
  type LoginHistoryItem,
  type MFASetupResult,
  type MFAStatus,
} from '@/api/admin-auth'
import { ApiError } from '@/types/api'
import { describeUserAgent } from '@/utils/user-agent'
import { useSessionLogout } from '@/composables/useSessionLogout'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const ui = useUiStore()
const { clearLocalSession } = useSessionLogout()

// 三个能力互不依赖，平铺会把页面拉得很长，因此改为 Tab：
//   password 修改密码（默认，必须改密时强制停留）
//   mfa      双因子认证
//   history  登录历史
const { tabs, active, select } = useAdminTabs([
  { key: 'password', label: '修改密码' },
  { key: 'mfa', label: '双因子认证（MFA）' },
  { key: 'history', label: '登录历史' },
])

// 后台账号密码口径：与管理员账号管理 / create-admin 一致，至少 12 位。
const MIN_PASSWORD_LENGTH = 12

// ===== 修改密码 =====
const form = reactive({ old_password: '', new_password: '', confirm: '' })
const errors = reactive<Record<string, string>>({})
const submitting = ref(false)
// 密码显示 / 隐藏切换。
const reveal = reactive({ old: false, neu: false, confirm: false })

function validate(): boolean {
  errors.old_password = ''
  errors.new_password = ''
  errors.confirm = ''
  let ok = true
  if (!form.old_password) {
    errors.old_password = '请输入当前密码'
    ok = false
  }
  if (form.new_password.length < MIN_PASSWORD_LENGTH) {
    errors.new_password = `新密码长度至少 ${MIN_PASSWORD_LENGTH} 位`
    ok = false
  } else if (form.new_password === form.old_password) {
    errors.new_password = '新密码不能与当前密码相同'
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
    // 会话已被服务端全部注销，前后台登录态一并清理后回到统一登录页。
    clearLocalSession()
    router.push({ path: '/login' })
  } catch (err) {
    // 把后端字段级错误（如「旧密码不正确」）就近渲染到对应输入框。
    const details = err instanceof ApiError ? err.details : undefined
    let mapped = false
    if (details?.length) {
      for (const d of details) {
        if (d.field && d.field in errors) {
          errors[d.field] = d.message
          mapped = true
        }
      }
    }
    if (!mapped) {
      ui.error(err instanceof ApiError ? err.message : '修改失败')
    }
  } finally {
    submitting.value = false
  }
}

// ===== 登录历史（设备 / IP / 时间）=====
const history = ref<LoginHistoryItem[]>([])
const historyLoading = ref(false)
// 管理员重置密码后，账号必须先完成改密；此处给出显式提示。
const mustChangePassword = computed(() => auth.user?.must_change_password === true)

function formatDateTime(v: string): string {
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString('zh-CN', { hour12: false })
}

async function loadLoginHistory() {
  historyLoading.value = true
  try {
    history.value = await listLoginHistory()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '加载登录历史失败')
  } finally {
    historyLoading.value = false
  }
}

// ===== 双因子认证（MFA / TOTP）=====
// 绑定流程：setup 拿 secret → 用户录入认证器 → 输入 6 位验证码 enable
// → 一次性展示恢复码。解绑需「密码 + 当前验证码」双证据。
const mfaState = ref<MFAStatus | null>(null)
const mfaLoading = ref(false)
const mfaBusy = ref(false)
const mfaSetupResult = ref<MFASetupResult | null>(null)
const mfaCode = ref('')
const recoveryCodes = ref<string[]>([])
const showDisable = ref(false)
const disableForm = reactive({ password: '', code: '' })

async function loadMFA() {
  mfaLoading.value = true
  try {
    mfaState.value = await mfaStatus()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '加载双因子认证状态失败')
  } finally {
    mfaLoading.value = false
  }
}

async function startMFA() {
  if (mfaBusy.value) return
  mfaBusy.value = true
  try {
    mfaSetupResult.value = await mfaSetup()
    mfaCode.value = ''
    recoveryCodes.value = []
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '生成绑定信息失败')
  } finally {
    mfaBusy.value = false
  }
}

function cancelMFA() {
  mfaSetupResult.value = null
  mfaCode.value = ''
}

async function confirmEnableMFA() {
  if (mfaBusy.value) return
  const code = mfaCode.value.trim()
  if (!code) {
    ui.warning('请输入认证器中的 6 位验证码')
    return
  }
  mfaBusy.value = true
  try {
    recoveryCodes.value = await mfaEnable(code)
    mfaSetupResult.value = null
    mfaCode.value = ''
    ui.success('双因子认证已启用，请立即保存恢复码')
    await loadMFA()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '验证码校验失败')
  } finally {
    mfaBusy.value = false
  }
}

async function confirmDisableMFA() {
  if (mfaBusy.value) return
  if (!disableForm.password || !disableForm.code) {
    ui.warning('请填写当前密码与验证码')
    return
  }
  mfaBusy.value = true
  try {
    await mfaDisable(disableForm.password, disableForm.code)
    ui.success('双因子认证已解绑')
    showDisable.value = false
    disableForm.password = ''
    disableForm.code = ''
    recoveryCodes.value = []
    await loadMFA()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '解绑失败')
  } finally {
    mfaBusy.value = false
  }
}

/** 复制到剪贴板；失败时提示手动复制（HTTP 环境可能无 clipboard 权限）。 */
async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ui.success(`${label}已复制`)
  } catch {
    ui.warning('复制失败，请手动选择文本复制')
  }
}

// Tab 按需加载：MFA 状态与登录历史只在首次进入各自 Tab 时请求，
// 避免打开用户信息页就发三份请求。
const mfaLoaded = ref(false)
const historyLoaded = ref(false)

watch(
  active,
  (key) => {
    if (key === 'mfa' && !mfaLoaded.value) {
      mfaLoaded.value = true
      loadMFA()
    }
    if (key === 'history' && !historyLoaded.value) {
      historyLoaded.value = true
      loadLoginHistory()
    }
  },
  { immediate: true },
)

onMounted(() => {
  // 兼容旧链接 /admin/profile#password（顶栏与路由守卫曾用 hash 定位）。
  const legacy = route.hash.replace('#', '')
  if (legacy && legacy !== active.value && tabs.value.some((t) => t.key === legacy)) {
    select(legacy)
  }
  // 必须改密时无论 URL 指向哪个 Tab，都先落到修改密码。
  if (auth.user?.must_change_password && active.value !== 'password') {
    select('password')
  }
})
</script>

<template>
  <div class="max-w-4xl space-y-6">
    <!-- 顶栏「用户 → 用户信息」锚点；后台账号只做安全设置，不放展示资料 -->
    <div id="account" class="scroll-mt-24">
      <p class="text-sm text-text-secondary">后台管理账号的密码与登录安全。</p>
      <p class="mt-1 text-xs text-text-muted">
        当前账号：{{ auth.displayName }}（{{ auth.user?.role ?? '—' }}）
      </p>
    </div>

    <!-- 强制改密提示：管理员重置密码后，完成改密前无法使用其它后台功能 -->
    <div
      v-if="mustChangePassword"
      class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-200"
    >
      你的密码已被管理员重置，请先在下方「修改密码」完成改密后继续使用后台功能。
    </div>

    <AdminTabs :tabs="tabs" :active="active" aria-label="用户信息" @select="select">
      <!-- 修改密码（默认 Tab，必须改密时强制停留） -->
      <section v-if="active === 'password'" id="password" class="card p-7">
        <h2 class="mb-1 text-lg font-semibold text-text-primary">修改密码</h2>
        <p class="mb-6 text-sm text-text-secondary">
          修改成功后，当前账号在所有设备上的登录都会失效，需要使用新密码重新登录。
        </p>

        <form class="space-y-5" novalidate @submit.prevent="handleSubmit">
          <FormField label="当前密码" required :error="errors.old_password">
            <template #default="{ id }">
              <div class="relative">
                <input
                  :id="id"
                  v-model="form.old_password"
                  :type="reveal.old ? 'text' : 'password'"
                  autocomplete="current-password"
                  placeholder="请输入当前密码"
                  class="h-11 w-full rounded-md border border-border-dark bg-ink-900 px-3 pr-16 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs text-text-muted hover:text-text-primary"
                  @click="reveal.old = !reveal.old"
                >
                  {{ reveal.old ? '隐藏' : '显示' }}
                </button>
              </div>
            </template>
          </FormField>

          <FormField
            label="新密码（至少 12 位）"
            required
            hint="建议使用较长且独立的密码。"
            :error="errors.new_password"
          >
            <template #default="{ id }">
              <div class="relative">
                <input
                  :id="id"
                  v-model="form.new_password"
                  :type="reveal.neu ? 'text' : 'password'"
                  autocomplete="new-password"
                  placeholder="请输入新密码"
                  class="h-11 w-full rounded-md border border-border-dark bg-ink-900 px-3 pr-16 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs text-text-muted hover:text-text-primary"
                  @click="reveal.neu = !reveal.neu"
                >
                  {{ reveal.neu ? '隐藏' : '显示' }}
                </button>
              </div>
            </template>
          </FormField>

          <FormField label="确认新密码" required :error="errors.confirm">
            <template #default="{ id }">
              <div class="relative">
                <input
                  :id="id"
                  v-model="form.confirm"
                  :type="reveal.confirm ? 'text' : 'password'"
                  autocomplete="new-password"
                  placeholder="请再次输入新密码"
                  class="h-11 w-full rounded-md border border-border-dark bg-ink-900 px-3 pr-16 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 rounded px-2 py-1 text-xs text-text-muted hover:text-text-primary"
                  @click="reveal.confirm = !reveal.confirm"
                >
                  {{ reveal.confirm ? '隐藏' : '显示' }}
                </button>
              </div>
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
      </section>

      <!-- 双因子认证：TOTP 绑定 / 解绑，另在登录时要求第二步验证 -->
      <section v-else-if="active === 'mfa'" id="mfa" class="card p-7">
        <div class="mb-1 flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-lg font-semibold text-text-primary">双因子认证（MFA）</h2>
          <span
            v-if="mfaState?.enabled"
            class="rounded bg-grass-600/15 px-2 py-0.5 text-xs text-grass-400"
          >
            已启用
          </span>
          <span
            v-else-if="mfaState && !mfaLoading"
            class="rounded bg-ink-700 px-2 py-0.5 text-xs text-text-muted"
          >
            未启用
          </span>
        </div>
        <p class="mb-4 text-sm text-text-secondary">
          启用后，登录时需要再输入认证器 App（如 Aegis / 1Password / Google Authenticator）生成的 6
          位验证码。绑定过程不依赖二维码：可直接把密钥手动添加到认证器。
        </p>

        <p v-if="mfaLoading" class="text-sm text-text-muted">加载中…</p>
        <p
          v-else-if="mfaState && !mfaState.configured"
          class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-200"
        >
          服务端未配置 MFA 加密密钥，暂不可绑定。请联系运维设置
          <code class="font-mono">MFA_ENCRYPTION_KEY</code>。
        </p>

        <template v-else-if="mfaState">
          <!-- 恢复码：仅在启用成功后展示一次 -->
          <div v-if="recoveryCodes.length > 0" class="space-y-3">
            <p class="rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
              以下恢复码<strong>仅显示这一次</strong>。请保存到安全位置：手机丢失时可用它登录一次。
            </p>
            <ul class="grid grid-cols-2 gap-2 font-mono text-sm text-text-primary">
              <li v-for="c in recoveryCodes" :key="c" class="rounded bg-ink-900 px-3 py-1.5">{{ c }}</li>
            </ul>
            <div class="flex gap-2">
              <button
                type="button"
                class="rounded-md border border-border-dark px-4 py-2 text-sm text-text-secondary transition hover:text-text-primary"
                @click="copyText(recoveryCodes.join('\n'), '恢复码')"
              >
                复制全部
              </button>
              <button
                type="button"
                class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
                @click="recoveryCodes = []"
              >
                我已保存
              </button>
            </div>
          </div>

          <!-- 绑定流程：密钥 + 首次验证码 -->
          <div v-else-if="mfaSetupResult" class="space-y-4">
            <div>
              <p class="mb-2 text-sm text-text-secondary">
                1. 在认证器中手动添加账号，密钥填：
              </p>
              <div class="flex flex-wrap items-center gap-2">
                <code class="break-all rounded bg-ink-900 px-3 py-2 font-mono text-sm text-text-primary">
                  {{ mfaSetupResult.secret }}
                </code>
                <button
                  type="button"
                  class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary"
                  @click="copyText(mfaSetupResult.secret, '密钥')"
                >
                  复制
                </button>
              </div>
              <details class="mt-2 text-xs text-text-muted">
                <summary class="cursor-pointer">查看 otpauth URI（可直接粘贴到支持 URI 的 App）</summary>
                <code class="mt-2 block break-all rounded bg-ink-900 p-2 font-mono">
                  {{ mfaSetupResult.uri }}
                </code>
              </details>
            </div>

            <FormField label="2. 输入认证器生成的 6 位验证码" required>
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="mfaCode"
                  type="text"
                  inputmode="numeric"
                  autocomplete="one-time-code"
                  placeholder="例如 123456"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm tracking-[0.2em] text-text-primary placeholder:tracking-normal placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </template>
            </FormField>

            <div class="flex gap-2">
              <button
                type="button"
                class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
                :disabled="mfaBusy"
                @click="confirmEnableMFA"
              >
                {{ mfaBusy ? '校验中…' : '确认启用' }}
              </button>
              <button
                type="button"
                class="rounded-md border border-border-dark px-4 py-2 text-sm text-text-secondary transition hover:text-text-primary"
                :disabled="mfaBusy"
                @click="cancelMFA"
              >
                取消
              </button>
            </div>
          </div>

          <!-- 已启用：状态 + 解绑 -->
          <div v-else-if="mfaState.enabled" class="space-y-3">
            <dl class="grid gap-1 text-sm text-text-secondary sm:grid-cols-2">
              <div>
                <dt class="text-xs uppercase text-text-muted">剩余恢复码</dt>
                <dd class="text-text-primary">{{ mfaState.recovery_codes_left }}</dd>
              </div>
              <div>
                <dt class="text-xs uppercase text-text-muted">最近使用</dt>
                <dd class="text-text-primary">
                  {{ mfaState.last_used_at ? formatDateTime(mfaState.last_used_at) : '—' }}
                </dd>
              </div>
            </dl>
            <p v-if="mfaState.recovery_codes_left === 0" class="text-xs text-amber-300">
              恢复码已全部用完，建议重新绑定以获取新的恢复码。
            </p>

            <button
              type="button"
              class="rounded-md border border-red-500/40 px-4 py-2 text-sm text-red-300 transition hover:bg-red-500/10"
              @click="showDisable = !showDisable"
            >
              {{ showDisable ? '取消解绑' : '解绑双因子认证' }}
            </button>

            <form
              v-if="showDisable"
              class="space-y-3 rounded-lg border border-border-dark/60 p-4"
              @submit.prevent="confirmDisableMFA"
            >
              <p class="text-sm text-text-secondary">解绑需要同时验证密码与当前验证码。</p>
              <FormField label="当前密码" required>
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="disableForm.password"
                    type="password"
                    autocomplete="current-password"
                    class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
                  />
                </template>
              </FormField>
              <FormField label="验证码或恢复码" required>
                <template #default="{ id }">
                  <input
                    :id="id"
                    v-model="disableForm.code"
                    type="text"
                    autocomplete="one-time-code"
                    class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none"
                  />
                </template>
              </FormField>
              <button
                type="button"
                class="rounded-md border border-red-500/40 px-4 py-2 text-sm text-red-300 transition hover:bg-red-500/10 disabled:opacity-50"
                :disabled="mfaBusy"
                @click="confirmDisableMFA"
              >
                {{ mfaBusy ? '处理中…' : '确认解绑' }}
              </button>
            </form>
          </div>

          <!-- 待启用 / 未启用 -->
          <div v-else class="space-y-3">
            <p v-if="mfaState.pending" class="text-sm text-amber-300">
              已生成绑定信息但尚未完成验证，可重新生成。
            </p>
            <button
              type="button"
              class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
              :disabled="mfaBusy"
              @click="startMFA"
            >
              {{ mfaBusy ? '生成中…' : '开始绑定' }}
            </button>
          </div>
        </template>
      </section>

      <!-- 登录历史：设备 / IP / 时间 / 结果，形式对齐审计日志 -->
      <section v-else-if="active === 'history'" id="history" class="card p-7">
        <div class="mb-1 flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-lg font-semibold text-text-primary">登录历史</h2>
          <button
            type="button"
            class="rounded-md border border-border-dark px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-700 hover:text-text-primary disabled:opacity-50"
            :disabled="historyLoading"
            @click="loadLoginHistory"
          >
            {{ historyLoading ? '刷新中…' : '刷新' }}
          </button>
        </div>
        <p class="mb-4 text-sm text-text-secondary">
          最多显示最近 50 次登录记录，按时间倒序，包含失败尝试。设备信息取自登录时的
          User-Agent，历史数据可能缺失。
        </p>

        <p v-if="historyLoading && history.length === 0" class="text-sm text-text-muted">加载中…</p>
        <p v-else-if="history.length === 0" class="text-sm text-text-muted">暂无登录记录</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[560px] text-left text-sm">
            <thead class="border-b border-border-dark text-xs text-text-muted">
              <tr>
                <th class="py-2 pr-4 font-medium">时间</th>
                <th class="py-2 pr-4 font-medium">设备</th>
                <th class="py-2 pr-4 font-medium">IP</th>
                <th class="py-2 font-medium">结果</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-dark">
              <tr v-for="(item, index) in history" :key="`${item.created_at}-${index}`">
                <td class="whitespace-nowrap py-2.5 pr-4 text-xs text-text-secondary">
                  {{ formatDateTime(item.created_at) }}
                </td>
                <td class="py-2.5 pr-4 text-text-primary" :title="item.user_agent || '无 User-Agent'">
                  {{ describeUserAgent(item.user_agent) }}
                </td>
                <td class="py-2.5 pr-4 font-mono text-xs text-text-secondary">
                  {{ item.source_ip || '—' }}
                </td>
                <td class="py-2.5">
                  <span
                    class="text-xs font-medium"
                    :class="item.result === 'success' ? 'text-grass-300' : 'text-danger-mc'"
                  >
                    {{ item.result === 'success' ? '成功' : '失败' }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </AdminTabs>
  </div>
</template>
