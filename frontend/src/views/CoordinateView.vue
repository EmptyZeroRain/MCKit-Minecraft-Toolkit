<script setup lang="ts">
// 坐标换算工具（需求 5.3）
// 主世界 <-> 下界：X / Z 除以 8（主世界→下界），乘以 8（下界→主世界）。
// Y 不参与换算。支持小数输入。空值、NaN、无穷值报错。
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import FormField from '@/components/common/FormField.vue'
import CopyButton from '@/components/common/CopyButton.vue'
import ShareButton from '@/components/common/ShareButton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useRecentToolsStore } from '@/stores/recent-tools'
import {
  NETHER_RATIO,
  convertWithValidation,
  formatCoordNumber,
  portalPair,
  routeHint,
  type CoordDirection,
} from '@/utils/coordinate'
import { applyPageMeta } from '@/utils/seo'
import { changedOnly, queryString } from '@/utils/share'

const route = useRoute()
const router = useRouter()
const recent = useRecentToolsStore()
recent.record('coordinate', '坐标换算', '计算类')

applyPageMeta({
  title: '坐标换算',
  description: 'Minecraft 主世界与下界坐标换算工具，一键换算 X/Z 坐标（÷8 或 ×8），Y 坐标保持不变。',
  canonicalPath: '/coordinate',
})

// TODO §14：换算方向与输入坐标同步到 URL，使换算结果与线路规划可直接分享复现。
// 注意：`<input type="number">` 上的 v-model 会被 Vue 自动转为 number，
// 因此输入统一按字符串读取（`xText/yText/zText`），避免 `.trim()` 在数字上抛错。
const direction = ref<CoordDirection>(
  queryString(route.query.dir) === 'netherToOverworld' ? 'netherToOverworld' : 'overworldToNether',
)
const x = ref<string | number>(queryString(route.query.x))
const y = ref<string | number>(queryString(route.query.y))
const z = ref<string | number>(queryString(route.query.z))

/** 数字输入 → 字符串（空值归一为空串），工具函数与 URL 均要求字符串。 */
function asText(value: string | number | null | undefined): string {
  return value === null || value === undefined ? '' : String(value)
}

const xText = computed(() => asText(x.value))
const yText = computed(() => asText(y.value))
const zText = computed(() => asText(z.value))
/** 三个坐标是否至少填了一个（决定结果区与按钮可用性） */
const hasInput = computed(() =>
  [xText.value, yText.value, zText.value].some((v) => v.trim() !== ''),
)

/** 将换算与线路规划状态统一写入 URL（仅保留非默认值；两类参数互不覆盖）。 */
function syncUrl() {
  router.replace({
    path: '/coordinate',
    query: {
      ...changedOnly(
        { dir: direction.value, x: xText.value, y: yText.value, z: zText.value },
        { dir: 'overworldToNether', x: '', y: '', z: '' },
      ),
      ...changedOnly(
        {
          fx: routeForm.fromX,
          fz: routeForm.fromZ,
          tx: routeForm.toX,
          tz: routeForm.toZ,
        },
        { fx: '', fz: '', tx: '', tz: '' },
      ),
    },
  })
}

watch([direction, x, y, z], syncUrl)

const result = computed(() => {
  if (!hasInput.value) return null
  const out = convertWithValidation(
    { x: xText.value, y: yText.value, z: zText.value },
    direction.value,
    false,
  )
  if (!out.ok) return null
  return out.result
})

const issues = computed(() => {
  if (!hasInput.value) return []
  return convertWithValidation({ x: xText.value, y: yText.value, z: zText.value }, direction.value, false).ok
    ? []
    : convertWithValidation({ x: xText.value, y: yText.value, z: zText.value }, direction.value, false).issues
})

function swapDirection() {
  direction.value =
    direction.value === 'overworldToNether' ? 'netherToOverworld' : 'overworldToNether'
}

function clearAll() {
  x.value = ''
  y.value = ''
  z.value = ''
}

function fillExample() {
  // 验收示例：主世界 (800, 70, -240) 对应下界理论坐标 (100, 70, -30)
  direction.value = 'overworldToNether'
  x.value = '800'
  y.value = '70'
  z.value = '-240'
}

const directionLabel = computed(() =>
  direction.value === 'overworldToNether' ? '主世界 → 下界' : '下界 → 主世界',
)

const factorLabel = computed(() =>
  direction.value === 'overworldToNether'
    ? `X、Z 除以 ${NETHER_RATIO}`
    : `X、Z 乘以 ${NETHER_RATIO}`,
)

const outputText = computed(() => {
  if (!result.value) return ''
  const r = result.value
  return `X: ${formatCoordNumber(r.x ?? NaN)}, Y: ${formatCoordNumber(r.y ?? NaN)}, Z: ${formatCoordNumber(r.z ?? NaN)}`
})

function goTools() {
  router.push('/tools')
}

// ===== 传送门理论配对（todo §10 第 4 项）=====
/** 输入坐标与另一维度的理论配对坐标；输入不完整时为 null */
const portal = computed(() => {
  if (xText.value.trim() === '' || zText.value.trim() === '') return null
  const xi = Number(xText.value)
  const zi = Number(zText.value)
  if (!Number.isFinite(xi) || !Number.isFinite(zi)) return null
  const dim = direction.value === 'overworldToNether' ? 'overworld' : 'nether'
  return portalPair({ x: xi, z: zi }, dim)
})

const portalText = computed(() => {
  if (!portal.value) return ''
  const p = portal.value
  return `主世界 (${formatCoordNumber(p.overworld.x)}, ${formatCoordNumber(p.overworld.z)}) ↔ 下界 (${formatCoordNumber(p.nether.x)}, ${formatCoordNumber(p.nether.z)})`
})

// ===== 线路规划 =====
// TODO §14：线路规划输入同样同步到 URL（`fx/fz/tx/tz`），便于分享固定路线。
const routeForm = reactive({
  fromX: queryString(route.query.fx),
  fromZ: queryString(route.query.fz),
  toX: queryString(route.query.tx),
  toZ: queryString(route.query.tz),
})

const routePlan = computed(() => {
  const raw = [routeForm.fromX, routeForm.fromZ, routeForm.toX, routeForm.toZ]
  if (raw.some((v) => String(v).trim() === '')) return null
  const [fx, fz, tx, tz] = raw.map(Number)
  if (![fx, fz, tx, tz].every(Number.isFinite)) return null
  const hint = routeHint({ x: fx, z: fz }, { x: tx, z: tz })
  // 下界移动速度为主世界 8 倍：同样的方块位移在下界对应 8 倍主世界距离。
  return { ...hint, netherDistance: hint.distance / NETHER_RATIO }
})

watch(routeForm, syncUrl)

function fillRouteExample() {
  routeForm.fromX = '0'
  routeForm.fromZ = '0'
  routeForm.toX = '800'
  routeForm.toZ = '-240'
}
</script>

<template>
  <PageContainer maxWidth="max-w-5xl">
    <Breadcrumbs :items="[{ label: '工具中心', to: '/tools' }, { label: '坐标换算' }]" />

    <div class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="mb-2 text-2xl font-bold text-text-primary md:text-3xl">坐标换算</h1>
        <p class="text-text-secondary">
          在主世界与下界之间换算坐标：{{ factorLabel }}；Y 坐标不参与换算。
        </p>
      </div>
      <ShareButton />
    </div>

    <div class="grid gap-6 lg:grid-cols-5">
      <!-- 输入区 -->
      <section class="card p-6 lg:col-span-3" aria-labelledby="coord-input-title">
        <div class="mb-5 flex items-center justify-between">
          <h2 id="coord-input-title" class="font-semibold text-text-primary">输入坐标</h2>
          <button
            type="button"
            class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
            @click="swapDirection"
          >
            切换方向：{{ directionLabel }}
          </button>
        </div>

        <div class="mb-4 rounded-lg border border-grass-500/30 bg-grass-500/10 px-4 py-2 text-xs text-grass-300">
          当前换算：{{ directionLabel }}（{{ factorLabel }}）
        </div>

        <form class="space-y-4" novalidate @submit.prevent>
          <div class="grid gap-4 sm:grid-cols-3">
            <FormField label="X 坐标" :error="issues.find((i) => i.field === 'x')?.reason">
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="x"
                  type="number"
                  step="any"
                  inputmode="decimal"
                  placeholder="如 800"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </template>
            </FormField>
            <FormField label="Y 坐标" :error="issues.find((i) => i.field === 'y')?.reason">
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="y"
                  type="number"
                  step="any"
                  inputmode="decimal"
                  placeholder="如 70"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </template>
            </FormField>
            <FormField label="Z 坐标" :error="issues.find((i) => i.field === 'z')?.reason">
              <template #default="{ id }">
                <input
                  :id="id"
                  v-model="z"
                  type="number"
                  step="any"
                  inputmode="decimal"
                  placeholder="如 -240"
                  class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary placeholder:text-text-muted focus:border-grass-500 focus:outline-none"
                />
              </template>
            </FormField>
          </div>

          <p v-if="issues.length" class="text-xs text-danger-mc" role="alert">
            {{ issues[0].reason }}
          </p>

          <div class="flex flex-wrap items-center gap-3">
            <button
              type="button"
              class="rounded-md bg-grass-600 px-5 py-2.5 text-sm font-medium text-white transition hover:bg-grass-500 disabled:opacity-50"
              :disabled="!hasInput"
            >
              换算结果见右侧
            </button>
            <button
              type="button"
              class="rounded-md border border-border-light bg-ink-700 px-4 py-2.5 text-sm text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
              @click="fillExample"
            >
              填入示例
            </button>
            <button
              type="button"
              class="text-sm text-text-muted transition hover:text-text-primary"
              @click="clearAll"
            >
              清空
            </button>
          </div>
        </form>
      </section>

      <!-- 输出区 -->
      <section class="card p-6 lg:col-span-2" aria-labelledby="coord-output-title">
        <div class="mb-4 flex items-center justify-between">
          <h2 id="coord-output-title" class="font-semibold text-text-primary">换算结果</h2>
          <CopyButton tool="coordinate" v-if="outputText" :text="outputText" />
        </div>

        <div v-if="result" class="space-y-4">
          <div class="code-block p-4">
            <div class="mb-2 text-xs text-text-muted">{{ directionLabel }}</div>
            <div class="font-mono text-lg text-grass-300">
              X: {{ formatCoordNumber(result.x ?? NaN) }}
            </div>
            <div class="mt-1 font-mono text-lg text-grass-300">
              Y: {{ formatCoordNumber(result.y ?? NaN) }}
            </div>
            <div class="mt-1 font-mono text-lg text-grass-300">
              Z: {{ formatCoordNumber(result.z ?? NaN) }}
            </div>
          </div>

          <div class="rounded-lg border border-border-dark bg-ink-900 px-4 py-2 text-xs leading-relaxed text-text-secondary">
            <strong class="text-text-primary">提示：</strong>
            这是理论对应关系；实际传送门链接位置会受生成位置、空间占用和游戏机制影响。
          </div>

          <div v-if="portal" class="rounded-lg border border-grass-500/30 bg-grass-500/10 p-4">
            <div class="mb-2 flex items-center justify-between">
              <h3 class="text-sm font-medium text-text-primary">传送门理论配对</h3>
              <CopyButton tool="coordinate" :text="portalText" />
            </div>
            <div class="space-y-1 font-mono text-sm text-grass-300">
              <div>主世界：{{ formatCoordNumber(portal.overworld.x) }}, {{ formatCoordNumber(portal.overworld.z) }}</div>
              <div>下界：{{ formatCoordNumber(portal.nether.x) }}, {{ formatCoordNumber(portal.nether.z) }}</div>
            </div>
            <p class="mt-2 text-xs text-text-secondary">
              比例 1:{{ NETHER_RATIO }}；Y 高度不参与换算，实际连接会就近寻找已有传送门。
            </p>
          </div>
        </div>

        <EmptyState
          v-else
          title="等待输入坐标"
          description="填写主世界或下界的坐标，结果会自动换算"
          action-label="浏览更多工具"
          @action="goTools"
        />

        <div class="mt-4 rounded-lg border border-border-dark bg-ink-900 p-4 text-sm">
          <h3 class="mb-2 font-medium text-text-primary">使用说明</h3>
          <ul class="list-disc space-y-1 pl-5 text-xs leading-relaxed text-text-secondary">
            <li>主世界 → 下界：X、Z 坐标除以 8，Y 保持不变。</li>
            <li>下界 → 主世界：X、Z 坐标乘以 8，Y 保持不变。</li>
            <li>支持小数输入；可以只填写需要换算的坐标轴，留空的坐标不参与换算。</li>
          </ul>
        </div>
      </section>
    </div>

    <!-- 线路规划 -->
    <section class="card mt-6 p-6" aria-labelledby="route-title">
      <div class="mb-4 flex items-center justify-between">
        <h2 id="route-title" class="font-semibold text-text-primary">线路规划</h2>
        <button
          type="button"
          class="rounded-md border border-border-light bg-ink-700 px-3 py-1.5 text-xs text-text-secondary transition hover:bg-ink-600 hover:text-text-primary"
          @click="fillRouteExample"
        >
          填入示例
        </button>
      </div>
      <p class="mb-4 text-xs text-text-muted">
        同一维度内两点的水平直线距离与方位；下界距离按 8 倍效率换算（经下界穿行可缩短路程）。
      </p>

      <div class="grid gap-4 sm:grid-cols-4">
        <label class="block text-sm">
          <span class="mb-1.5 block font-medium text-text-primary">起点 X</span>
          <input v-model="routeForm.fromX" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </label>
        <label class="block text-sm">
          <span class="mb-1.5 block font-medium text-text-primary">起点 Z</span>
          <input v-model="routeForm.fromZ" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </label>
        <label class="block text-sm">
          <span class="mb-1.5 block font-medium text-text-primary">终点 X</span>
          <input v-model="routeForm.toX" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </label>
        <label class="block text-sm">
          <span class="mb-1.5 block font-medium text-text-primary">终点 Z</span>
          <input v-model="routeForm.toZ" type="number" class="h-10 w-full rounded-md border border-border-dark bg-ink-900 px-3 font-mono text-sm text-text-primary focus:border-grass-500 focus:outline-none" />
        </label>
      </div>

      <div v-if="routePlan" class="mt-4 grid gap-3 sm:grid-cols-3">
        <div class="rounded-lg border border-border-dark bg-ink-900 p-4">
          <div class="text-xs text-text-muted">水平直线距离</div>
          <div class="font-mono text-lg text-grass-300">{{ routePlan.distance.toFixed(1) }} 格</div>
        </div>
        <div class="rounded-lg border border-border-dark bg-ink-900 p-4">
          <div class="text-xs text-text-muted">方位</div>
          <div class="font-mono text-lg text-grass-300">{{ routePlan.direction }}（{{ routePlan.bearing.toFixed(0) }}°）</div>
        </div>
        <div class="rounded-lg border border-border-dark bg-ink-900 p-4">
          <div class="text-xs text-text-muted">经下界理论距离</div>
          <div class="font-mono text-lg text-grass-300">{{ routePlan.netherDistance.toFixed(1) }} 格</div>
        </div>
      </div>
      <p v-else class="mt-4 text-sm text-text-muted">填写起点与终点坐标后显示距离与方位。</p>
    </section>
  </PageContainer>
</template>