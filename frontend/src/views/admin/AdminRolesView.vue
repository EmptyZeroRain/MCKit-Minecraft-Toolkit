<script setup lang="ts">
// 角色管理（后台）：角色 CRUD + 权限点勾选（需求 §6.2）。
// 仅具备 roles.manage 权限的角色可访问（侧边栏过滤 + 后端 403 兜底）。
// 内置角色不可删除、不可改名；内置 admin 必须保留全量权限（后端校验，前端同步禁用勾选）。
import { computed, onMounted, reactive, ref } from 'vue'
import ListShell from '@/components/admin/list-shell.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import FormField from '@/components/common/FormField.vue'
import { useUiStore } from '@/stores/ui'
import { createRole, deleteRole, listPermissions, listRoles, updateRole } from '@/api/admin-roles'
import type { PermissionGroup, RoleRow } from '@/types/admin'
import { ApiError } from '@/types/api'

const ui = useUiStore()

const roles = ref<RoleRow[]>([])
const groups = ref<PermissionGroup[]>([])
const loading = ref(false)
const error = ref('')

// 编辑弹窗：editing 为 null 表示新建
const dialogOpen = ref(false)
const editing = ref<RoleRow | null>(null)
const saving = ref(false)
const form = reactive({ name: '', label: '', description: '' })
const selected = ref<Set<string>>(new Set())

const isEdit = computed(() => editing.value !== null)
/** 内置 admin 角色：必须保留全部权限，勾选全部禁用（后端亦会拒绝削权）。 */
const lockPermissions = computed(() => editing.value?.name === 'admin')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [roleList, groupList] = await Promise.all([listRoles(), listPermissions()])
    roles.value = roleList
    groups.value = groupList
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.name = ''
  form.label = ''
  form.description = ''
  selected.value = new Set()
  dialogOpen.value = true
}

function openEdit(role: RoleRow) {
  editing.value = role
  form.name = role.name
  form.label = role.label
  form.description = role.description
  selected.value = new Set(role.permissions)
  dialogOpen.value = true
}

function isChecked(key: string): boolean {
  return selected.value.has(key)
}

function toggle(key: string) {
  if (lockPermissions.value) return
  const next = new Set(selected.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  selected.value = next
}

function groupAllChecked(group: PermissionGroup): boolean {
  return group.permissions.length > 0 && group.permissions.every((p) => selected.value.has(p.key))
}

function toggleGroup(group: PermissionGroup) {
  if (lockPermissions.value) return
  const next = new Set(selected.value)
  const all = groupAllChecked(group)
  for (const p of group.permissions) {
    if (all) next.delete(p.key)
    else next.add(p.key)
  }
  selected.value = next
}

async function save() {
  if (saving.value) return
  const label = form.label.trim()
  if (!label) {
    ui.warning('请填写角色名称')
    return
  }
  if (!isEdit.value && !/^[a-z][a-z0-9_-]{0,39}$/.test(form.name.trim())) {
    ui.warning('角色标识需以字母开头，仅含小写字母、数字、下划线或连字符')
    return
  }

  saving.value = true
  try {
    const permissions = [...selected.value]
    if (editing.value) {
      await updateRole({
        name: editing.value.name,
        label,
        description: form.description.trim(),
        permissions,
      })
      ui.success('角色已更新，使用该角色的账号需重新登录生效')
    } else {
      await createRole({
        name: form.name.trim(),
        label,
        description: form.description.trim(),
        permissions,
      })
      ui.success('角色已创建')
    }
    dialogOpen.value = false
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '保存失败')
  } finally {
    saving.value = false
  }
}

const toDelete = ref<RoleRow | null>(null)
const deleting = ref(false)

async function confirmDelete() {
  if (!toDelete.value) return
  deleting.value = true
  try {
    await deleteRole(toDelete.value.name)
    ui.success('角色已删除')
    toDelete.value = null
    load()
  } catch (err) {
    ui.error(err instanceof ApiError ? err.message : '删除失败')
  } finally {
    deleting.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-lg font-semibold text-text-primary">角色管理</h1>
        <p class="mt-1 text-sm text-text-secondary">
          角色决定后台入口与接口的可访问范围。内置角色不可删除、不可改名；自定义角色可自由组合权限点。
        </p>
      </div>
      <button
        type="button"
        class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-grass-500"
        @click="openCreate"
      >
        新建角色
      </button>
    </div>

    <ListShell title="角色" :loading="loading" :error="error" @retry="load">
      <div v-if="roles.length" class="card overflow-x-auto">
        <table class="w-full min-w-[760px] text-left text-sm">
          <thead class="border-b border-border-dark bg-ink-900 text-xs text-text-muted">
            <tr>
              <th scope="col" class="px-4 py-2.5">角色名称</th>
              <th scope="col" class="px-4 py-2.5">标识</th>
              <th scope="col" class="px-4 py-2.5">说明</th>
              <th scope="col" class="px-4 py-2.5 text-right">权限点</th>
              <th scope="col" class="px-4 py-2.5">类型</th>
              <th scope="col" class="px-4 py-2.5 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border-dark">
            <tr v-for="role in roles" :key="role.name" class="hover:bg-ink-800/60">
              <td class="px-4 py-3 font-medium text-text-primary">{{ role.label }}</td>
              <td class="px-4 py-3 font-mono text-xs text-text-muted">{{ role.name }}</td>
              <td class="max-w-md px-4 py-3 text-xs text-text-secondary">{{ role.description || '—' }}</td>
              <td class="px-4 py-3 text-right text-text-secondary">{{ role.permissions.length }}</td>
              <td class="px-4 py-3">
                <span
                  class="rounded px-2 py-0.5 text-xs"
                  :class="role.is_system ? 'bg-ink-700 text-text-muted' : 'bg-grass-500/15 text-grass-300'"
                >
                  {{ role.is_system ? '内置' : '自定义' }}
                </span>
              </td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1.5 text-xs">
                  <button
                    type="button"
                    class="rounded border border-border-light px-2 py-1 text-grass-400 hover:bg-ink-700"
                    @click="openEdit(role)"
                  >
                    编辑
                  </button>
                  <button
                    type="button"
                    class="rounded border border-danger-mc/40 px-2 py-1 text-danger-mc hover:bg-danger-mc/10 disabled:opacity-40"
                    :disabled="role.is_system"
                    :title="role.is_system ? '内置角色不可删除' : ''"
                    @click="toDelete = role"
                  >
                    删除
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else-if="!loading" class="card p-10 text-center text-sm text-text-muted">暂无角色。</div>
    </ListShell>

    <!-- 新建 / 编辑角色 -->
    <Teleport to="body">
      <div
        v-if="dialogOpen"
        class="fixed inset-0 z-[90] flex items-center justify-center bg-black/60 p-4"
        role="dialog"
        aria-modal="true"
        @click.self="dialogOpen = false"
      >
        <div class="card max-h-[85vh] w-full max-w-2xl overflow-y-auto p-6 shadow-2xl">
          <h2 class="mb-4 text-lg font-semibold text-text-primary">
            {{ isEdit ? `编辑角色：${form.label}` : '新建角色' }}
          </h2>

          <div class="grid gap-4 sm:grid-cols-2">
            <FormField label="角色标识" :required="!isEdit" hint="创建后不可修改；仅小写字母、数字、下划线、连字符">
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="form.name"
                  type="text"
                  autocomplete="off"
                  maxlength="40"
                  :disabled="isEdit"
                  placeholder="例如 reviewer"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none disabled:opacity-60"
                />
              </template>
            </FormField>
            <FormField label="角色名称" required hint="展示用中文名">
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="form.label"
                  type="text"
                  autocomplete="off"
                  maxlength="80"
                  placeholder="例如 内容审核员"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </template>
            </FormField>
            <div class="sm:col-span-2">
              <FormField label="说明">
                <template #default="{ id }">
                  <textarea
                    :id="id"
                    v-model="form.description"
                    rows="2"
                    maxlength="200"
                    class="w-full rounded-md border border-border-dark bg-ink-900 px-3 py-2 text-sm text-text-primary focus:border-grass-500 focus:outline-none"
                  />
                </template>
              </FormField>
            </div>
          </div>

          <div class="mt-5">
            <div class="mb-2 flex items-center justify-between">
              <h3 class="text-sm font-semibold text-text-primary">权限点</h3>
              <span class="text-xs text-text-muted">已选 {{ selected.size }} 项</span>
            </div>
            <p v-if="lockPermissions" class="mb-3 rounded border border-border-dark bg-ink-900 px-3 py-2 text-xs text-text-muted">
              内置 admin 角色必须保留全部权限（避免系统无人可管理），权限点不可调整。
            </p>
            <div class="space-y-3">
              <fieldset
                v-for="group in groups"
                :key="group.key"
                class="rounded-md border border-border-dark bg-ink-900/60 p-3"
              >
                <legend class="px-1 text-xs font-semibold text-text-secondary">
                  <button
                    type="button"
                    class="hover:text-grass-300 disabled:opacity-50"
                    :disabled="lockPermissions"
                    @click="toggleGroup(group)"
                  >
                    {{ group.label }}（{{ groupAllChecked(group) ? '取消全选' : '全选' }}）
                  </button>
                </legend>
                <div class="mt-1 grid gap-2 sm:grid-cols-2">
                  <label
                    v-for="p in group.permissions"
                    :key="p.key"
                    class="flex cursor-pointer items-center gap-2 text-sm text-text-secondary"
                    :class="{ 'cursor-not-allowed opacity-60': lockPermissions }"
                  >
                    <input
                      type="checkbox"
                      class="h-4 w-4 accent-grass-600"
                      :checked="isChecked(p.key)"
                      :disabled="lockPermissions"
                      @change="toggle(p.key)"
                    />
                    <span>{{ p.label }}</span>
                    <span class="font-mono text-xs text-text-muted">{{ p.key }}</span>
                  </label>
                </div>
              </fieldset>
            </div>
          </div>

          <div class="mt-6 flex justify-end gap-3">
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2 text-sm text-text-primary hover:bg-ink-600"
              @click="dialogOpen = false"
            >
              取消
            </button>
            <button
              type="button"
              class="rounded-md bg-grass-600 px-4 py-2 text-sm font-medium text-white hover:bg-grass-500 disabled:opacity-50"
              :disabled="saving"
              @click="save"
            >
              {{ saving ? '保存中…' : '保存' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 删除确认 -->
    <ConfirmDialog
      :open="!!toDelete"
      title="删除角色"
      :message="`确定删除角色「${toDelete?.label}」吗？仍被账号使用的角色无法删除。此操作不可撤销。`"
      confirm-text="确认删除"
      dangerous
      :confirm-disabled="deleting"
      @confirm="confirmDelete"
      @cancel="toDelete = null"
    />
  </div>
</template>
