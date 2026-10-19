<script setup lang="ts">
// 管理员账号管理（需求 6.1 身份认证 / 6.2 权限）
// 需 accounts.admins 权限（侧边栏过滤 + 后端 403 兜底）。
// 统一账号体系下 role='user' 属「普通用户管理」，此处角色下拉必须排除，
// 否则选中会被后端以「内置角色 user 属普通用户」拒绝（422）。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import FormField from '@/components/common/FormField.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import {
  listAdmins,
  createAdmin,
  updateAdmin,
  resetAdminPassword,
  deleteAdmin,
  deleteAdminPreview,
  resetAdminMFA,
  type AdminDeletePreview,
} from '@/api/admin-admins'
import { listRoles } from '@/api/admin-roles'
import type { AdminUser, RoleRow } from '@/types/admin'
import type { AdminRole } from '@/types/common'
import { ApiError } from '@/types/api'
import { timeAgo } from '@/utils/format'

const auth = useAuthStore()
const ui = useUiStore()

const filters = reactive({ q: '', page: 1, pageSize: 15 })
const items = ref<AdminUser[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')

// 可选角色（来自「角色管理」，含自定义角色）
const roles = ref<RoleRow[]>([])

// 可分配角色：排除内置 user（普通用户不走后台账号体系）。
const assignableRoles = computed(() => roles.value.filter((r) => r.name !== 'user'))

// 创建表单
const createForm = reactive<{ username: string; password: string; role: AdminRole }>({
  username: '',
  password: '',
  role: 'editor',
})
const creating = ref(false)

// 待删除 / 待重置密码目标
const toDelete = ref<AdminUser | null>(null)
const deleting = ref(false)
const resetTarget = ref<AdminUser | null>(null)
const resetPassword = ref('')
const resetting = ref(false)

// 行内操作进行中标记（避免重复点击）
const busyId = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listAdmins({
      q: filters.q || undefined,
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
  try {
    roles.value = await listRoles()
    const options = assignableRoles.value
    if (!options.some((r) => r.name === createForm.role) && options.length) {
      createForm.role = options.find((r) => r.name === 'editor')?.name ?? options[0].name
    }
  } catch {
    roles.value = []
  }
}

// roleOptions 行内下拉选项：以可分配角色为准，并保留当前账号的角色（角色被改名/删除时仍可正确展示）。
function roleOptions(item: AdminUser): { name: string; label: string }[] {
  const options = assignableRoles.value.map((r) => ({ name: r.name, label: r.label }))
  if (item.role && !options.some((o) => o.name === item.role)) {
    options.push({ name: item.role, label: item.role })
  }
  return options
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

function onSearch() {
  filters.page = 1
  load()
}

function isSelf(item: AdminUser): boolean {
  return item.id === auth.user?.id
}

async function handleCreate() {
  if (creating.value) return
  const username = createForm.username.trim()
  if (!username) {
    ui.warning('请填写用户名')
    return
  }
  if (createForm.password.length < 12) {
    ui.warning('密码长度至少 12 位')
    return
  }
  creating.value = true
  try {
    await createAdmin({ username, password: createForm.password, role: createForm.role })
    ui.success('管理员账号已创建')
    handleCreateReset()
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '创建失败')
  } finally {
    creating.value = false
  }
}

async function toggleActive(item: AdminUser) {
  if (busyId.value) return
  busyId.value = item.id
  try {
    const updated = await updateAdmin({ id: item.id, is_active: !item.is_active })
    item.is_active = updated.is_active
    ui.success(updated.is_active ? '账号已启用' : '账号已停用，其会话已失效')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busyId.value = ''
  }
}

async function changeRole(item: AdminUser, role: AdminRole) {
  if (role === item.role || busyId.value) return
  busyId.value = item.id
  try {
    const updated = await updateAdmin({ id: item.id, role })
    item.role = updated.role
    ui.success('角色已更新，该账号需重新登录')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busyId.value = ''
  }
}

function openReset(item: AdminUser) {
  resetTarget.value = item
  resetPassword.value = ''
}

async function confirmReset() {
  if (!resetTarget.value || resetting.value) return
  if (resetPassword.value.length < 12) {
    ui.warning('密码长度至少 12 位')
    return
  }
  resetting.value = true
  try {
    await resetAdminPassword({ id: resetTarget.value.id, new_password: resetPassword.value })
    ui.success('密码已重置，该账号会话已全部失效')
    resetTarget.value = null
    resetPassword.value = ''
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '重置失败')
  } finally {
    resetting.value = false
  }
}

// 删除影响预览：是否自己 / 最后一名 admin / 活动会话数。
const deletePreview = ref<AdminDeletePreview | null>(null)
const previewing = ref(false)

async function openDelete(admin: AdminUser) {
  toDelete.value = admin
  deletePreview.value = null
  previewing.value = true
  try {
    deletePreview.value = await deleteAdminPreview(admin.id)
  } catch (err) {
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
    return `确定删除「${a.username}」吗？该账号所有会话将立即失效，此操作不可恢复。`
  }
  const risks: string[] = []
  if (p.is_self) risks.push('不能删除当前登录账号')
  if (p.is_last_admin) risks.push('系统必须保留至少一个启用状态的 admin 账号')
  const risk = risks.length > 0 ? `注意：${risks.join('；')}。` : ''
  return (
    `确定删除「${a.username}」（角色 ${p.role}）吗？将撤销 ${p.active_sessions} 个登录会话。` +
    `${risk}此操作不可恢复。`
  )
})

function cancelDelete() {
  toDelete.value = null
  deletePreview.value = null
}

/** 账号恢复：解绑目标账号的 MFA（高权限操作，后端写审计）。 */
async function handleResetMFA(admin: AdminUser) {
  if (busyId.value) return
  if (
    !window.confirm(
      `确定解绑「${admin.username}」的双因子认证吗？\n解绑后该账号登录只需密码，安全性下降；请通知本人尽快重新绑定。`,
    )
  ) {
    return
  }
  busyId.value = admin.id
  try {
    await resetAdminMFA(admin.id)
    ui.success('已解绑该账号的双因子认证')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '解绑失败')
  } finally {
    busyId.value = ''
  }
}

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  try {
    await deleteAdmin(toDelete.value.id)
    ui.success('账号已删除')
    toDelete.value = null
    deletePreview.value = null
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

function handleCreateReset() {
  createForm.username = ''
  createForm.password = ''
  createForm.role =
    assignableRoles.value.find((r) => r.name === 'editor')?.name ??
    assignableRoles.value[0]?.name ??
    'editor'
}

onMounted(() => {
  loadRoles()
  load()
})
</script>

<template>
  <div class="space-y-5">
    <!-- 创建账号 -->
    <div class="card p-5">
      <h2 class="mb-4 font-semibold text-text-primary">新建管理员账号</h2>
      <form class="grid gap-4 sm:grid-cols-3" novalidate @submit.prevent="handleCreate">
        <FormField label="用户名" required>
          <template #default="{ id }">
            <input
              :id="id"
              v-model="createForm.username"
              type="text"
              autocomplete="off"
              maxlength="80"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField label="初始密码（≥12 位）" required>
          <template #default="{ id }">
            <input
              :id="id"
              v-model="createForm.password"
              type="password"
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
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            >
              <option v-for="r in assignableRoles" :key="r.name" :value="r.name">{{ r.label }}（{{ r.name }}）</option>
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
    </div>

    <!-- 账号列表 -->
    <ListShell title="管理员账号" :loading="loading" :error="error" @retry="load">
      <div class="mb-4">
        <input
          v-model="filters.q"
          type="search"
          aria-label="搜索用户名"
          placeholder="搜索用户名"
          class="h-9 w-full max-w-xs rounded-md border border-border-dark bg-ink-800 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
          @change="onSearch"
        />
      </div>

      <div v-if="items.length" class="card overflow-x-auto">
        <table class="w-full min-w-[760px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">用户名</th>
              <th scope="col" class="px-4 py-2.5">角色</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">最后登录</th>
              <th scope="col" class="px-4 py-2.5">创建</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3">
                <span class="font-medium text-text-primary">{{ item.username }}</span>
                <span v-if="isSelf(item)" class="ml-2 rounded bg-grass-500/15 px-1.5 py-0.5 text-xs text-grass-300">我</span>
              </td>
              <td class="px-4 py-3">
                <select
                  :value="item.role"
                  class="h-8 rounded-md border border-border-dark bg-ink-800 px-2 text-xs text-text-primary focus:border-grass-500 focus:outline-none disabled:opacity-50"
                  :disabled="busyId === item.id"
                  @change="changeRole(item, ($event.target as HTMLSelectElement).value as AdminRole)"
                >
                  <option v-for="r in roleOptions(item)" :key="r.name" :value="r.name">{{ r.label }}</option>
                </select>
              </td>
              <td class="px-4 py-3">
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="item.is_active ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
                >
                  {{ item.is_active ? '启用' : '停用' }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">
                {{ item.last_login_at ? timeAgo(item.last_login_at) : '从未登录' }}
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">{{ timeAgo(item.created_at) }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700 disabled:opacity-40"
                    :disabled="busyId === item.id || isSelf(item)"
                    :title="isSelf(item) ? '不能停用当前登录账号' : ''"
                    @click="toggleActive(item)"
                  >
                    {{ item.is_active ? '停用' : '启用' }}
                  </button>
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700 disabled:opacity-40"
                    :disabled="!!resetTarget"
                    @click="openReset(item)"
                  >
                    重置密码
                  </button>
                  <button
                    type="button"
                    class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10 disabled:opacity-40"
                    :disabled="isSelf(item)"
                    :title="isSelf(item) ? '不能删除当前登录账号' : ''"
                    @click="openDelete(item)"
                  >
                    删除
                  </button>
                  <button
                    type="button"
                    class="rounded border border-amber-500/40 px-2 py-1 text-amber-300 hover:bg-amber-500/10 disabled:opacity-40"
                    :disabled="busyId === item.id"
                    title="账号恢复：解绑该账号的双因子认证（换手机且恢复码丢失时使用）"
                    @click="handleResetMFA(item)"
                  >
                    重置 MFA
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
      title="删除管理员账号"
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
        @click.self="resetTarget = null"
      >
        <div class="card w-full max-w-md p-6 shadow-2xl">
          <h2 class="mb-2 text-lg font-semibold text-text-primary">重置密码</h2>
          <p class="mb-4 text-sm text-text-secondary">
            为「{{ resetTarget.username }}」设置新密码（≥12 位）。重置后该账号所有会话将立即失效。
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
  </div>
</template>
