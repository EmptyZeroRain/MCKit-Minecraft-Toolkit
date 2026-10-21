<script setup lang="ts">
// 统一账号管理（gj.md §10）：普通用户与后台账号共用一个列表。
//
// 权限点仍独立判定（accounts.admins / accounts.users），前端显隐只是体验优化：
//   - 有 accounts.admins：可查看全部账号、分配后台角色、管理后台账号；
//   - 只有 accounts.users：列表自动收敛为普通用户，且不能分配后台角色。
// 后端会按目标账号角色二次校验，直接改请求参数无法越权。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import FormField from '@/components/common/FormField.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import {
  listAccounts,
  createAccount,
  getAccountDetail,
  updateAccount,
  resetAccountPassword,
  deleteAccount,
  deleteAccountPreview,
  resetAccountMFA,
  exportAccountsCsv,
  type AccountDeletePreview,
  type AccountDetail,
  type AccountRow,
  type AccountScope,
} from '@/api/admin-accounts'
import { listRoles } from '@/api/admin-roles'
import type { RoleRow } from '@/types/admin'
import { ApiError } from '@/types/api'
import { timeAgo } from '@/utils/format'

const route = useRoute()
const auth = useAuthStore()
const ui = useUiStore()

const canManageAdmins = computed(() => auth.hasPermission('accounts.admins'))
const canManageUsers = computed(() => auth.hasPermission('accounts.users'))

// initialScope 兼容旧入口：/admin/admins?scope=admin、/admin/users?scope=user；
// 同时保留 tab=admins / tab=users 历史书签语义。
function initialScope(): AccountScope {
  const raw = route.query.scope
  if (raw === 'admin' || raw === 'user') return raw
  if (route.query.tab === 'admins') return 'admin'
  if (route.query.tab === 'users') return 'user'
  if (canManageAdmins.value && canManageUsers.value) return ''
  return canManageAdmins.value ? 'admin' : 'user'
}

const filters = reactive<{
  q: string
  status: '' | 'active' | 'disabled'
  scope: AccountScope
  page: number
  pageSize: number
}>({
  q: '',
  status: '',
  scope: initialScope(),
  page: 1,
  pageSize: 15,
})

const items = ref<AccountRow[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const exporting = ref(false)

// 行内操作进行中标记（避免重复点击）
const busyId = ref('')

// 可选角色（来自「角色管理」，含自定义角色）
const roles = ref<RoleRow[]>([])

// 可分配的后台角色：排除内置 user（普通用户由列表角色「普通用户」表示）。
const adminRoles = computed(() => roles.value.filter((r) => r.name !== 'user'))

function roleLabel(name: string): string {
  if (name === 'user') return '普通用户'
  return roles.value.find((r) => r.name === name)?.label ?? name
}

// 当前账号可分配的角色：普通用户 + 后台角色。
const assignableRoles = computed(() => {
  const out: { name: string; label: string }[] = []
  if (canManageUsers.value) out.push({ name: 'user', label: '普通用户' })
  if (canManageAdmins.value) {
    out.push(...adminRoles.value.map((r) => ({ name: r.name, label: r.label })))
  }
  return out
})

// 详情抽屉
const detailTarget = ref<AccountRow | null>(null)
const detail = ref<AccountDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')

// 待删除 / 待重置密码目标
const toDelete = ref<AccountRow | null>(null)
const deleting = ref(false)
const resetTarget = ref<AccountRow | null>(null)
const resetPassword = ref('')
const resetting = ref(false)

// 新建账号表单：角色决定密码强度与用户名规则。
const createForm = reactive<{ username: string; password: string; role: string }>({
  username: '',
  password: '',
  role: 'user',
})
const creating = ref(false)

// kindLabel 个人数据条目类型的中文标签（未知类型回退原值）。
const KIND_LABELS: Record<string, string> = {
  tool: '工具',
  item: '物品',
  resource: '资源',
  wiki: '百科',
  potion: '药水',
  villager: '村民',
  mob: '生物',
  redstone_design: '红石设计',
  mod: 'Mod',
  modpack: '整合包',
  devdoc: '开发文档',
}
function kindLabel(kind: string): string {
  return KIND_LABELS[kind] ?? kind
}

function canOperate(item: AccountRow): boolean {
  return item.role === 'user' ? canManageUsers.value : canManageAdmins.value
}

function minPasswordLength(role: string): number {
  return role === 'user' ? 8 : 12
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listAccounts({
      q: filters.q || undefined,
      status: filters.status || undefined,
      scope: filters.scope,
      page: filters.page,
      page_size: filters.pageSize,
    })
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

// loadRoles 拉取可选角色；失败不阻断账号列表（仅角色下拉为空）。
async function loadRoles() {
  if (!canManageAdmins.value) return
  try {
    roles.value = await listRoles()
    if (!assignableRoles.value.some((r) => r.name === createForm.role)) {
      createForm.role = assignableRoles.value[0]?.name ?? 'user'
    }
  } catch {
    roles.value = []
  }
}

function onSearch() {
  filters.page = 1
  load()
}

function onScopeChange() {
  filters.page = 1
  load()
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

function isSelf(item: AccountRow): boolean {
  return item.id === auth.user?.id
}

async function handleCreate() {
  if (creating.value) return
  const username = createForm.username.trim()
  const role = createForm.role
  if (role === 'user') {
    if (!/^[A-Za-z][A-Za-z0-9_-]{2,39}$/.test(username)) {
      ui.warning('普通用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符')
      return
    }
  } else if (!username || username.length > 80) {
    ui.warning('用户名长度需在 1-80 之间')
    return
  }
  const min = minPasswordLength(role)
  if (createForm.password.length < min) {
    ui.warning(`密码长度至少 ${min} 位`)
    return
  }
  creating.value = true
  try {
    await createAccount({ username, password: createForm.password, role })
    ui.success(role === 'user' ? '普通用户已创建' : '后台账号已创建')
    createForm.username = ''
    createForm.password = ''
    filters.page = 1
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '创建失败')
  } finally {
    creating.value = false
  }
}

async function toggleActive(item: AccountRow) {
  if (busyId.value || !canOperate(item)) return
  busyId.value = item.id
  try {
    const updated = await updateAccount({ id: item.id, is_active: !item.is_active })
    item.is_active = updated.is_active
    ui.success(updated.is_active ? '账号已启用' : '账号已停用，其会话已失效')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busyId.value = ''
  }
}

// changeRole 修改角色：普通用户 ↔ 后台角色。取消时把下拉框还原。
async function changeRole(item: AccountRow, target: HTMLSelectElement) {
  const role = target.value
  if (role === item.role || busyId.value) {
    target.value = item.role
    return
  }
  const label = roleLabel(role)
  const message =
    role === 'user'
      ? `确定将「${item.username}」降级为普通用户吗？\n该账号将失去全部后台权限，已有会话立即失效。`
      : `确定将「${item.username}」设为「${label}」吗？\n该账号将获得后台权限，已有会话立即失效，并需在下次登录时修改密码。`
  if (!window.confirm(message)) {
    target.value = item.role
    return
  }
  busyId.value = item.id
  try {
    const updated = await updateAccount({ id: item.id, role })
    item.role = updated.role
    ui.success('角色已更新，该账号需重新登录')
  } catch (err) {
    target.value = item.role
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busyId.value = ''
  }
}

// roleOptions 行内下拉：可分配角色 + 当前角色（角色被改名/删除时仍可正确展示）。
function roleOptions(item: AccountRow): { name: string; label: string }[] {
  const options = assignableRoles.value.map((r) => ({ ...r }))
  if (item.role && !options.some((o) => o.name === item.role)) {
    options.push({ name: item.role, label: roleLabel(item.role) })
  }
  return options
}

async function openDetail(item: AccountRow) {
  if (!canOperate(item)) return
  detailTarget.value = item
  detail.value = null
  detailError.value = ''
  detailLoading.value = true
  try {
    detail.value = await getAccountDetail(item.id)
  } catch (err) {
    detailError.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    detailLoading.value = false
  }
}

function closeDetail() {
  detailTarget.value = null
  detail.value = null
  detailError.value = ''
}

function openReset(item: AccountRow) {
  if (!canOperate(item)) return
  resetTarget.value = item
  resetPassword.value = ''
}

async function confirmReset() {
  if (!resetTarget.value || resetting.value) return
  const min = minPasswordLength(resetTarget.value.role)
  if (resetPassword.value.length < min) {
    ui.warning(`密码长度至少 ${min} 位`)
    return
  }
  resetting.value = true
  try {
    await resetAccountPassword({ id: resetTarget.value.id, new_password: resetPassword.value })
    ui.success('密码已重置，该账号会话已全部失效')
    resetTarget.value = null
    resetPassword.value = ''
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '重置失败')
  } finally {
    resetting.value = false
  }
}

// 删除影响预览：先展示将一并清理的数据范围，再允许确认删除。
const deletePreview = ref<AccountDeletePreview | null>(null)
const previewing = ref(false)

async function openDelete(item: AccountRow) {
  if (!canOperate(item)) return
  toDelete.value = item
  deletePreview.value = null
  previewing.value = true
  try {
    deletePreview.value = await deleteAccountPreview(item.id)
  } catch (err) {
    // 预览失败不阻断删除，但明确告知影响范围未知。
    ui.error(err instanceof ApiError ? err.message : '加载删除影响预览失败')
  } finally {
    previewing.value = false
  }
}

const deleteMessage = computed(() => {
  const a = toDelete.value
  if (!a) return ''
  const p = deletePreview.value
  if (!p) {
    return `确定删除「${a.username}」（${roleLabel(a.role)}）吗？删除为物理删除，不可恢复。`
  }
  const risks: string[] = []
  if (p.is_self) risks.push('不能删除当前登录账号')
  if (p.is_last_admin) risks.push('系统必须保留至少一个启用状态的 admin 账号')
  const risk = risks.length > 0 ? `注意：${risks.join('；')}。` : ''
  return (
    `确定删除「${a.username}」（${roleLabel(a.role)}）吗？将一并清除：收藏 ${p.favorites} 条、` +
    `最近使用 ${p.recents} 条、常用种子 ${p.seeds} 条、投稿 ${p.submissions} 条，` +
    `并撤销 ${p.active_sessions} 个登录会话。${risk}此操作不可恢复。`
  )
})

function cancelDelete() {
  toDelete.value = null
  deletePreview.value = null
}

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  try {
    await deleteAccount(toDelete.value.id)
    ui.success('账号已删除，其个人数据已一并清除')
    toDelete.value = null
    deletePreview.value = null
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

/** 账号恢复：解绑目标账号的 MFA（高权限操作，后端写审计）。 */
async function handleResetMFA(item: AccountRow) {
  if (busyId.value) return
  if (
    !window.confirm(
      `确定解绑「${item.username}」的双因子认证吗？\n解绑后该账号登录只需密码，安全性下降；请通知本人尽快重新绑定。`,
    )
  ) {
    return
  }
  busyId.value = item.id
  try {
    await resetAccountMFA(item.id)
    ui.success('已解绑该账号的双因子认证')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '解绑失败')
  } finally {
    busyId.value = ''
  }
}

async function handleExport() {
  if (exporting.value) return
  exporting.value = true
  try {
    const { blob, truncated } = await exportAccountsCsv({
      q: filters.q || undefined,
      status: filters.status || undefined,
      scope: filters.scope,
    })
    triggerDownload(blob)
    ui.success(truncated ? '已导出（超出上限，仅含前 10000 条）' : '已导出 CSV')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '导出失败')
  } finally {
    exporting.value = false
  }
}

// triggerDownload 通过临时 <a> 触发浏览器下载。
function triggerDownload(blob: Blob) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `accounts-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

onMounted(async () => {
  await loadRoles()
  load()
})
</script>

<template>
  <div class="space-y-5">
    <!-- 新建账号 -->
    <div class="card p-5">
      <h2 class="mb-4 font-semibold text-text-primary">新建账号</h2>
      <form class="grid gap-4 sm:grid-cols-3" novalidate @submit.prevent="handleCreate">
        <FormField label="用户名" required>
          <template #default="{ id }">
            <input
              :id="id"
              v-model="createForm.username"
              type="text"
              aria-label="新账号用户名"
              autocomplete="off"
              :maxlength="createForm.role === 'user' ? 40 : 80"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField :label="`初始密码（≥${minPasswordLength(createForm.role)} 位）`" required>
          <template #default="{ id }">
            <input
              :id="id"
              v-model="createForm.password"
              type="password"
              aria-label="新账号初始密码"
              autocomplete="new-password"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField label="角色">
          <template #default="{ id }">
            <select
              :id="id"
              v-model="createForm.role"
              :disabled="assignableRoles.length <= 1"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none disabled:opacity-60"
            >
              <option v-for="r in assignableRoles" :key="r.name" :value="r.name">
                {{ r.label }}{{ r.name === 'user' ? '' : `（${r.name}）` }}
              </option>
            </select>
          </template>
        </FormField>
        <div class="sm:col-span-3">
          <button
            type="submit"
            class="rounded-md bg-grass-600 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
            :disabled="creating"
          >
            {{ creating ? '创建中…' : '创建账号' }}
          </button>
        </div>
      </form>
      <p class="mt-3 text-xs text-text-muted">
        普通用户名需以字母开头，3-40 位，仅含字母、数字、下划线或连字符；后台角色用户名 1-80 位。
        创建后普通用户与后台账号均可从统一登录页登录（后台账号按配置要求 MFA）。
      </p>
    </div>

    <ListShell title="账号列表" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <input
          v-model="filters.q"
          type="search"
          aria-label="搜索用户名"
          placeholder="搜索用户名 / 用户 ID"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="onSearch"
        />
        <select
          v-model="filters.status"
          aria-label="按状态筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onSearch"
        >
          <option value="">全部状态</option>
          <option value="active">仅启用</option>
          <option value="disabled">仅禁用</option>
        </select>
        <select
          v-if="canManageAdmins && canManageUsers"
          v-model="filters.scope"
          aria-label="按账号类型筛选"
          class="h-9 rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
          @change="onScopeChange"
        >
          <option value="">全部账号</option>
          <option value="admin">后台账号</option>
          <option value="user">普通用户</option>
        </select>
        <button
          type="button"
          class="ml-auto rounded-md border border-border-light px-3 py-2 text-sm text-text-primary transition hover:bg-ink-700 disabled:opacity-50"
          :disabled="exporting"
          @click="handleExport"
        >
          {{ exporting ? '导出中…' : '导出 CSV' }}
        </button>
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[1200px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">用户 ID</th>
              <th scope="col" class="px-4 py-2.5">用户名</th>
              <th scope="col" class="px-4 py-2.5">角色</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">最后登录</th>
              <th scope="col" class="px-4 py-2.5">注册</th>
              <th scope="col" class="px-4 py-2.5">邮箱</th>
              <th scope="col" class="px-4 py-2.5 text-right">礼物币</th>
              <th scope="col" class="px-4 py-2.5 text-right">等级</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3 font-mono text-xs text-text-secondary">{{ item.user_no || '—' }}</td>
              <td class="px-4 py-3">
                <span class="font-medium text-text-primary">{{ item.username }}</span>
                <span
                  v-if="item.nickname && item.nickname !== item.username"
                  class="ml-2 text-xs text-text-muted"
                >
                  {{ item.nickname }}
                </span>
                <span
                  v-if="isSelf(item)"
                  class="ml-2 rounded bg-grass-500/15 px-1.5 py-0.5 text-xs text-grass-300"
                >
                  我
                </span>
              </td>
              <td class="px-4 py-3">
                <select
                  v-if="canManageAdmins"
                  :value="item.role"
                  class="h-8 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary focus:border-grass-500 focus:outline-none disabled:opacity-50"
                  :disabled="busyId === item.id"
                  :aria-label="`修改 ${item.username} 的角色`"
                  @change="changeRole(item, $event.target as HTMLSelectElement)"
                >
                  <option v-for="r in roleOptions(item)" :key="r.name" :value="r.name">{{ r.label }}</option>
                </select>
                <span
                  v-else
                  class="rounded px-2 py-0.5 text-xs"
                  :class="item.role === 'user' ? 'bg-ink-700 text-text-secondary' : 'bg-grass-500/15 text-grass-300'"
                >
                  {{ roleLabel(item.role) }}
                </span>
              </td>
              <td class="px-4 py-3">
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="item.is_active ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
                >
                  {{ item.is_active ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">
                {{ item.last_login_at ? timeAgo(item.last_login_at) : '从未登录' }}
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">{{ timeAgo(item.created_at) }}</td>
              <td class="max-w-[220px] truncate px-4 py-3 text-xs text-text-secondary" :title="item.email">
                {{ item.email || '—' }}
              </td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.gift_coins }}</td>
              <td class="px-4 py-3 text-right text-text-secondary">Lv.{{ item.level }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-text-secondary hover:bg-ink-700 disabled:opacity-40"
                    :disabled="!canOperate(item)"
                    @click="openDetail(item)"
                  >
                    详情
                  </button>
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700 disabled:opacity-40"
                    :disabled="busyId === item.id || !canOperate(item) || (isSelf(item) && item.is_active)"
                    :title="isSelf(item) && item.is_active ? '不能停用当前登录账号' : ''"
                    @click="toggleActive(item)"
                  >
                    {{ item.is_active ? '停用' : '启用' }}
                  </button>
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700 disabled:opacity-40"
                    :disabled="!!resetTarget || !canOperate(item)"
                    @click="openReset(item)"
                  >
                    重置密码
                  </button>
                  <button
                    v-if="canManageAdmins && item.role !== 'user'"
                    type="button"
                    class="rounded border border-amber-500/40 px-2 py-1 text-amber-300 hover:bg-amber-500/10 disabled:opacity-40"
                    :disabled="busyId === item.id"
                    title="账号恢复：解绑该账号的双因子认证（换手机且恢复码丢失时使用）"
                    @click="handleResetMFA(item)"
                  >
                    重置 MFA
                  </button>
                  <button
                    type="button"
                    class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10 disabled:opacity-40"
                    :disabled="isSelf(item) || !canOperate(item)"
                    :title="isSelf(item) ? '不能删除当前登录账号' : ''"
                    @click="openDelete(item)"
                  >
                    删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无账号。</div>
    </ListShell>

    <Pagination
      :page="filters.page"
      :page-size="filters.pageSize"
      :total="total"
      :total-pages="totalPages"
      @change="onPageChange"
    />

    <!-- 删除确认 -->
    <ConfirmDialog
      :open="!!toDelete"
      title="删除账号"
      :message="deleteMessage"
      confirm-text="确认删除"
      dangerous
      :confirm-disabled="deleting || previewing"
      @confirm="confirmDelete"
      @cancel="cancelDelete"
    />

    <!-- 重置密码 -->
    <Teleport to="body">
      <div
        v-if="resetTarget"
        class="fixed inset-0 z-[90] flex items-center justify-center bg-black/60 p-4"
        role="dialog"
        aria-modal="true"
        aria-label="重置账号密码"
        @click.self="resetTarget = null"
      >
        <div class="card w-full max-w-md p-6 shadow-2xl">
          <h2 class="mb-2 text-lg font-semibold text-text-primary">重置密码</h2>
          <p class="mb-4 text-sm text-text-secondary">
            为「{{ resetTarget.username }}」（{{ roleLabel(resetTarget.role) }}）设置新密码（≥{{
              minPasswordLength(resetTarget.role)
            }}
            位）。重置后该账号所有会话将立即失效，并要求下次登录修改密码。
          </p>
          <FormField label="新密码" required>
            <template #default="{ id }">
              <input
                :id="id"
                v-model="resetPassword"
                type="password"
                autocomplete="new-password"
                class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
              />
            </template>
          </FormField>
          <div class="mt-6 flex justify-end gap-3">
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600"
              @click="resetTarget = null"
            >
              取消
            </button>
            <button
              type="button"
              class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white hover:bg-grass-500 disabled:opacity-50"
              :disabled="resetting"
              @click="confirmReset"
            >
              {{ resetting ? '重置中…' : '确认重置' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 详情抽屉 -->
    <Teleport to="body">
      <div
        v-if="detailTarget"
        class="fixed inset-0 z-[90] flex justify-end bg-black/60"
        role="dialog"
        aria-modal="true"
        aria-label="账号详情"
        @click.self="closeDetail"
      >
        <div class="flex h-full w-full max-w-xl flex-col overflow-y-auto bg-ink-900 shadow-2xl">
          <header class="flex items-start justify-between border-b border-border-dark p-5">
            <div>
              <h2 class="flex items-center gap-2 text-lg font-semibold text-text-primary">
                {{ detailTarget.username }}
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="
                    detailTarget.role === 'user'
                      ? 'bg-ink-700 text-text-secondary'
                      : 'bg-grass-500/15 text-grass-300'
                  "
                >
                  {{ roleLabel(detailTarget.role) }}
                </span>
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="detailTarget.is_active ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
                >
                  {{ detailTarget.is_active ? '启用' : '禁用' }}
                </span>
              </h2>
              <p class="mt-1 text-xs text-text-muted">
                用户 ID <span class="font-mono">{{ detailTarget.user_no }}</span> ·
                注册 {{ timeAgo(detailTarget.created_at) }} ·
                最后登录 {{ detailTarget.last_login_at ? timeAgo(detailTarget.last_login_at) : '从未登录' }}
              </p>
            </div>
            <button
              type="button"
              class="rounded border border-border-light px-2 py-1 text-xs text-text-secondary hover:bg-ink-700"
              aria-label="关闭"
              @click="closeDetail"
            >
              关闭
            </button>
          </header>

          <div class="space-y-6 p-5">
            <p v-if="detailLoading" class="text-sm text-text-muted" role="status" aria-live="polite">加载中…</p>
            <p v-else-if="detailError" class="text-sm text-danger-mc" role="alert">{{ detailError }}</p>
            <template v-else-if="detail">
              <!-- 收藏 -->
              <section>
                <h3 class="mb-2 text-sm font-semibold text-text-primary">
                  收藏（{{ detail.favorite_count }}）
                </h3>
                <ul v-if="detail.favorites.length" class="space-y-1.5">
                  <li
                    v-for="f in detail.favorites"
                    :key="f.id"
                    class="flex items-center justify-between gap-3 rounded border border-border-dark bg-ink-800 px-3 py-2 text-sm"
                  >
                    <span class="min-w-0 truncate text-text-primary">
                      <span class="mr-2 rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">{{ kindLabel(f.kind) }}</span>
                      {{ f.label || f.object_id }}
                    </span>
                    <span class="shrink-0 text-xs text-text-muted">{{ timeAgo(f.created_at) }}</span>
                  </li>
                </ul>
                <p v-else class="text-sm text-text-muted">暂无收藏。</p>
              </section>

              <!-- 最近使用 -->
              <section>
                <h3 class="mb-2 text-sm font-semibold text-text-primary">
                  最近使用（{{ detail.recent_count }}）
                </h3>
                <ul v-if="detail.recents.length" class="space-y-1.5">
                  <li
                    v-for="r in detail.recents"
                    :key="r.id"
                    class="flex items-center justify-between gap-3 rounded border border-border-dark bg-ink-800 px-3 py-2 text-sm"
                  >
                    <span class="min-w-0 truncate text-text-primary">
                      <span class="mr-2 rounded bg-ink-700 px-1.5 py-0.5 text-xs text-text-muted">{{ kindLabel(r.kind) }}</span>
                      {{ r.label || r.object_id }}
                    </span>
                    <span class="shrink-0 text-xs text-text-muted">{{ timeAgo(r.visited_at) }}</span>
                  </li>
                </ul>
                <p v-else class="text-sm text-text-muted">暂无记录。</p>
              </section>

              <!-- 常用种子 -->
              <section>
                <h3 class="mb-2 text-sm font-semibold text-text-primary">
                  常用种子（{{ detail.seed_count }}）
                </h3>
                <ul v-if="detail.seeds.length" class="space-y-1.5">
                  <li
                    v-for="s in detail.seeds"
                    :key="s.id"
                    class="rounded border border-border-dark bg-ink-800 px-3 py-2 text-sm"
                  >
                    <div class="flex items-center justify-between gap-3">
                      <span class="min-w-0 truncate font-medium text-text-primary">{{ s.name || '未命名' }}</span>
                      <span class="shrink-0 text-xs text-text-muted">{{ timeAgo(s.created_at) }}</span>
                    </div>
                    <div class="mt-1 break-all font-mono text-xs text-text-secondary">{{ s.seed }}</div>
                    <div v-if="s.note" class="mt-1 text-xs text-text-muted">{{ s.note }}</div>
                  </li>
                </ul>
                <p v-else class="text-sm text-text-muted">暂无常用种子。</p>
              </section>
            </template>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
