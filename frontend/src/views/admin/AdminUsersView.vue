<script setup lang="ts">
// 普通用户管理（后台）：账号列表 + 个人数据计数 + 启用/禁用、重置密码、删除、详情、导出 CSV。
// 需 accounts.users 权限（侧边栏过滤 + 后端 403 兜底）。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import FormField from '@/components/common/FormField.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useUiStore } from '@/stores/ui'
import {
  listUsers,
  createUser,
  getUserDetail,
  updateUser,
  resetUserPassword,
  deleteUser,
  deleteUserPreview,
  exportUsersCsv,
  type UserDeletePreview,
  type UserListQuery,
} from '@/api/admin-users'
import type { PlatformUserDetail, PlatformUserRow } from '@/types/admin'
import { ApiError } from '@/types/api'
import { timeAgo } from '@/utils/format'

const ui = useUiStore()

const filters = reactive<{ q: string; status: '' | 'active' | 'disabled'; page: number; pageSize: number }>({
  q: '',
  status: '',
  page: 1,
  pageSize: 15,
})

const items = ref<PlatformUserRow[]>([])
const total = ref(0)
const totalPages = ref(1)
const loading = ref(false)
const error = ref('')
const exporting = ref(false)

// 行内操作进行中标记（避免重复点击）
const busyId = ref('')

// 详情抽屉
const detailTarget = ref<PlatformUserRow | null>(null)
const detail = ref<PlatformUserDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref('')

// 待删除 / 待重置密码目标
const toDelete = ref<PlatformUserRow | null>(null)
const deleting = ref(false)
const resetTarget = ref<PlatformUserRow | null>(null)
const resetPassword = ref('')
const resetting = ref(false)

// 新建用户表单（角色固定为普通用户，无需选择）
const createForm = reactive({ username: '', password: '' })
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

async function load() {
  loading.value = true
  error.value = ''
  try {
    const params: UserListQuery = {
      q: filters.q || undefined,
      status: filters.status || undefined,
      page: filters.page,
      page_size: filters.pageSize,
    }
    const res = await listUsers(params)
    items.value = res.items
    total.value = res.total
    totalPages.value = res.totalPages || Math.max(1, Math.ceil(res.total / filters.pageSize))
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function onSearch() {
  filters.page = 1
  load()
}

function onPageChange(p: number) {
  filters.page = p
  load()
}

// handleCreate 新建普通用户：角色固定为普通用户，用户名规则与注册一致。
async function handleCreate() {
  if (creating.value) return
  const username = createForm.username.trim()
  if (!/^[A-Za-z][A-Za-z0-9_-]{2,39}$/.test(username)) {
    ui.warning('用户名需为 3-40 位，以字母开头，仅含字母、数字、下划线或连字符')
    return
  }
  if (createForm.password.length < 8) {
    ui.warning('密码长度至少 8 位')
    return
  }
  creating.value = true
  try {
    await createUser({ username, password: createForm.password })
    ui.success('普通用户已创建')
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

async function toggleActive(item: PlatformUserRow) {
  if (busyId.value) return
  busyId.value = item.id
  try {
    const updated = await updateUser({ id: item.id, is_active: !item.is_active })
    item.is_active = updated.is_active
    ui.success(updated.is_active ? '用户已启用' : '用户已禁用，其会话已失效')
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '操作失败')
  } finally {
    busyId.value = ''
  }
}

async function openDetail(item: PlatformUserRow) {
  detailTarget.value = item
  detail.value = null
  detailError.value = ''
  detailLoading.value = true
  try {
    detail.value = await getUserDetail(item.id)
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

function openReset(item: PlatformUserRow) {
  resetTarget.value = item
  resetPassword.value = ''
}

async function confirmReset() {
  if (!resetTarget.value || resetting.value) return
  if (resetPassword.value.length < 8) {
    ui.warning('密码长度至少 8 位')
    return
  }
  resetting.value = true
  try {
    await resetUserPassword({ id: resetTarget.value.id, new_password: resetPassword.value })
    ui.success('密码已重置，该用户会话已全部失效')
    resetTarget.value = null
    resetPassword.value = ''
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '重置失败')
  } finally {
    resetting.value = false
  }
}

// 删除影响预览：先展示将一并清理的数据范围，再允许确认删除。
const deletePreview = ref<UserDeletePreview | null>(null)
const previewing = ref(false)

async function openDelete(user: PlatformUserRow) {
  toDelete.value = user
  deletePreview.value = null
  previewing.value = true
  try {
    deletePreview.value = await deleteUserPreview(user.id)
  } catch (err) {
    // 预览失败不阻断删除，但明确告知影响范围未知。
    ui.error(err instanceof ApiError ? err.message : '加载删除影响预览失败')
  } finally {
    previewing.value = false
  }
}

const deleteMessage = computed(() => {
  const u = toDelete.value
  if (!u) return ''
  const p = deletePreview.value
  if (!p) {
    return `确定删除「${u.username}」吗？删除为物理删除，不可恢复。`
  }
  return (
    `确定删除「${u.username}」吗？将一并清除：收藏 ${p.favorites} 条、最近使用 ${p.recents} 条、` +
    `常用种子 ${p.seeds} 条、投稿 ${p.submissions} 条，并撤销 ${p.sessions} 个登录会话。此操作不可恢复。`
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
    await deleteUser(toDelete.value.id)
    ui.success('用户已删除，其个人数据已一并清除')
    toDelete.value = null
    deletePreview.value = null
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

async function handleExport() {
  if (exporting.value) return
  exporting.value = true
  try {
    const { blob, truncated } = await exportUsersCsv({
      q: filters.q || undefined,
      status: filters.status || undefined,
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
  a.download = `users-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

onMounted(load)
</script>

<template>
  <div class="space-y-5">
    <!-- 新建普通用户 -->
    <div class="card p-5">
      <h2 class="mb-4 font-semibold text-text-primary">新建普通用户</h2>
      <form class="grid gap-4 sm:grid-cols-3" novalidate @submit.prevent="handleCreate">
        <FormField label="用户名" required>
          <template #default="{ id }">
            <input
              :id="id"
              v-model="createForm.username"
              type="text"
              aria-label="新用户用户名"
              autocomplete="off"
              maxlength="40"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <FormField label="初始密码（≥8 位）" required>
          <template #default="{ id }">
            <input
              :id="id"
              v-model="createForm.password"
              type="password"
              aria-label="新用户初始密码"
              autocomplete="new-password"
              class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
            />
          </template>
        </FormField>
        <div class="flex items-end">
          <button
            type="submit"
            class="rounded-md bg-grass-600 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
            :disabled="creating"
          >
            {{ creating ? '创建中…' : '创建用户' }}
          </button>
        </div>
      </form>
      <p class="mt-3 text-xs text-text-muted">
        用户名需以字母开头，3-40 位，仅含字母、数字、下划线或连字符；创建后用户可直接在前台登录。
      </p>
    </div>

    <ListShell title="普通用户" :loading="loading" :error="error" @retry="load">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <input
          v-model="filters.q"
          type="search"
          aria-label="搜索用户名"
          placeholder="搜索用户名"
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
        <table class="w-full min-w-[1040px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">用户名</th>
              <th scope="col" class="px-4 py-2.5">状态</th>
              <th scope="col" class="px-4 py-2.5">注册</th>
              <th scope="col" class="px-4 py-2.5">最后登录</th>
              <th scope="col" class="px-4 py-2.5 text-right">收藏</th>
              <th scope="col" class="px-4 py-2.5 text-right">最近使用</th>
              <th scope="col" class="px-4 py-2.5 text-right">常用种子</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="item in items" :key="item.id" class="hover:bg-ink-800/60">
              <td class="px-4 py-3 font-medium text-text-primary">{{ item.username }}</td>
              <td class="px-4 py-3">
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="item.is_active ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
                >
                  {{ item.is_active ? '启用' : '禁用' }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs text-text-muted">{{ timeAgo(item.created_at) }}</td>
              <td class="px-4 py-3 text-xs text-text-muted">
                {{ item.last_login_at ? timeAgo(item.last_login_at) : '从未登录' }}
              </td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.favorite_count }}</td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.recent_count }}</td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ item.seed_count }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-text-secondary hover:bg-ink-700"
                    @click="openDetail(item)"
                  >
                    详情
                  </button>
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700 disabled:opacity-40"
                    :disabled="busyId === item.id"
                    @click="toggleActive(item)"
                  >
                    {{ item.is_active ? '禁用' : '启用' }}
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
                    class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10"
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
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无普通用户。</div>
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
      title="删除普通用户"
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
        aria-label="重置用户密码"
        @click.self="resetTarget = null"
      >
        <div class="card w-full max-w-md p-6 shadow-2xl">
          <h2 class="mb-2 text-lg font-semibold text-text-primary">重置密码</h2>
          <p class="mb-4 text-sm text-text-secondary">
            为「{{ resetTarget.username }}」设置新密码（8–128 位）。重置后该用户所有会话将立即失效。
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
        aria-label="用户详情"
        @click.self="closeDetail"
      >
        <div class="flex h-full w-full max-w-xl flex-col overflow-y-auto bg-ink-900 shadow-2xl">
          <header class="flex items-start justify-between border-b border-border-dark p-5">
            <div>
              <h2 class="flex items-center gap-2 text-lg font-semibold text-text-primary">
                {{ detailTarget.username }}
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="detailTarget.is_active ? 'bg-grass-500/15 text-grass-300' : 'bg-danger-mc/15 text-danger-mc'"
                >
                  {{ detailTarget.is_active ? '启用' : '禁用' }}
                </span>
              </h2>
              <p class="mt-1 text-xs text-text-muted">
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
