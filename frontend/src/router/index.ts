// ===== 路由配置（需求 3.1 / 3.2，路由级懒加载）=====
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import AdminLayout from '@/layouts/AdminLayout.vue'
import { useAuthStore } from '@/stores/auth'
import { useUserAuthStore } from '@/stores/user-auth'
import { applyNoIndexMeta } from '@/utils/seo'
import { builtinToolRoute } from '@/utils/builtin-tools'
import { reportToolVisit } from '@/utils/tool-stats'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    component: DefaultLayout,
    children: [
      {
        path: '',
        name: 'home',
        component: () => import('@/views/HomeView.vue'),
        meta: { title: 'MC 工具站 · Minecraft Java 版工具与资源导航' },
      },
      {
        path: 'tools',
        name: 'tools',
        component: () => import('@/views/ToolsView.vue'),
        meta: { title: '工具中心' },
      },
      {
        path: 'tools/:slug',
        name: 'tool-detail',
        component: () => import('@/views/ToolDetailView.vue'),
        meta: { title: '工具详情' },
        // 内置核心工具（坐标换算/配方/物品ID/指令）统一重定向到其专用页，
        // 避免同一功能出现两套界面（需求：功能与 UI 单一来源）。
        beforeEnter: (to) => {
          const slug = to.params.slug as string
          if (typeof slug === 'string') {
            const route = builtinToolRoute(slug)
            if (route) return route
          }
          return true
        },
      },
      {
        path: 'recipes',
        name: 'recipes',
        component: () => import('@/views/RecipesView.vue'),
        meta: { title: '合成配方查询' },
      },
      {
        path: 'items',
        name: 'items',
        component: () => import('@/views/ItemsView.vue'),
        meta: { title: '物品 ID 查询' },
      },
      {
        // 物品独立详情页（统一物品资料库 TODO §1）；item_id 含冒号属合法路径字符。
        path: 'items/:itemId',
        name: 'item-detail',
        component: () => import('@/views/ItemDetailView.vue'),
        meta: { title: '物品详情' },
      },
      {
        path: 'potions',
        name: 'potions',
        component: () => import('@/views/PotionsView.vue'),
        meta: { title: '药水与酿造查询' },
      },
      {
        // 药水独立详情页（TODO §9A：效果时长、酿造步骤、关联资料）。
        path: 'potions/:slug',
        name: 'potion-detail',
        component: () => import('@/views/PotionDetailView.vue'),
        meta: { title: '药水详情' },
      },
      {
        // 村民交易资料（TODO §9B）：职业 / 交易等级 / 交易表。
        path: 'villagers',
        name: 'villagers',
        component: () => import('@/views/VillagersView.vue'),
        meta: { title: '村民交易查询' },
      },
      {
        // 村民职业详情页（TODO §9B：等级规则、分组交易、关联物品）。
        path: 'villagers/:slug',
        name: 'villager-detail',
        component: () => import('@/views/VillagerDetailView.vue'),
        meta: { title: '村民职业详情' },
      },
      {
        // 生物资料（TODO §9C）：掉落 / 生成 / 驯服 / 繁殖。
        path: 'mobs',
        name: 'mobs',
        component: () => import('@/views/MobsView.vue'),
        meta: { title: '生物资料查询' },
      },
      {
        // 生物详情页（TODO §9C：掉落清单、生成与驯服繁殖说明、关联物品）。
        path: 'mobs/:slug',
        name: 'mob-detail',
        component: () => import('@/views/MobDetailView.vue'),
        meta: { title: '生物详情' },
      },
      {
        // 红石与机器设计资料库（TODO §15）：元件 / 设计双 tab 检索。
        path: 'redstone',
        name: 'redstone',
        component: () => import('@/views/RedstoneView.vue'),
        meta: { title: '红石与机器设计' },
      },
      {
        // 红石元件详情（TODO §15：工作机制与信号说明、适用版本与来源核验）。
        path: 'redstone/components/:slug',
        name: 'redstone-component-detail',
        component: () => import('@/views/RedstoneDetailView.vue'),
        props: { kind: 'component' },
        meta: { title: '红石元件详情' },
      },
      {
        // 机器设计详情（TODO §15：材料清单、搭建步骤、来源与授权）。
        path: 'redstone/designs/:slug',
        name: 'redstone-design-detail',
        component: () => import('@/views/RedstoneDetailView.vue'),
        props: { kind: 'design' },
        meta: { title: '机器设计详情' },
      },
      {
        // Mod 与整合包兼容性查询（TODO §16）：Mod / 整合包双 tab 检索。
        path: 'mods',
        name: 'mods',
        component: () => import('@/views/ModsView.vue'),
        meta: { title: 'Mod 与整合包' },
      },
      {
        // Mod 详情（TODO §16：发布版本、依赖与冲突、来源与验证状态）。
        path: 'mods/:slug',
        name: 'mod-detail',
        component: () => import('@/views/ModDetailView.vue'),
        meta: { title: 'Mod 详情' },
      },
      {
        // 整合包详情（TODO §16：包内 Mod 清单，已收录才链接）。
        path: 'modpacks/:slug',
        name: 'modpack-detail',
        component: () => import('@/views/ModpackDetailView.vue'),
        meta: { title: '整合包详情' },
      },
      {
        // 我的世界开发栏目（TODO §21）：开发文档知识库。
        path: 'dev',
        name: 'dev-docs',
        component: () => import('@/views/DevDocsView.vue'),
        meta: { title: '我的世界开发文档' },
      },
      {
        path: 'dev/:slug',
        name: 'dev-doc-detail',
        component: () => import('@/views/DevDocDetailView.vue'),
        meta: { title: '开发文档详情' },
      },
      {
        // 版本映射查询（TODO §21）：类 / 方法 / 字段三列名字对照。
        path: 'mappings',
        name: 'mappings',
        component: () => import('@/views/MappingsView.vue'),
        meta: { title: '版本映射查询' },
      },
      {
        path: 'commands',
        name: 'commands',
        component: () => import('@/views/CommandsView.vue'),
        meta: { title: '指令格式辅助' },
      },
      {
        path: 'command-generator',
        name: 'command-generator',
        component: () => import('@/views/CommandGeneratorView.vue'),
        meta: { title: '可视化指令生成器' },
      },
      {
        path: 'enchant-calculator',
        name: 'enchant-calculator',
        component: () => import('@/views/EnchantCalculatorView.vue'),
        meta: { title: '附魔与装备计算器' },
      },
      {
        path: 'coordinate',
        name: 'coordinate',
        component: () => import('@/views/CoordinateView.vue'),
        meta: { title: '坐标换算' },
      },
      {
        path: 'versions',
        name: 'versions',
        component: () => import('@/views/VersionsView.vue'),
        meta: { title: 'MC 版本信息' },
      },
      {
        path: 'block-ids',
        name: 'block-ids',
        component: () => import('@/views/BlockIdsView.vue'),
        meta: { title: '物品 ID 对照表' },
      },
      {
        path: 'seed-map',
        name: 'seed-map',
        component: () => import('@/views/SeedMapView.vue'),
        meta: { title: '种子地图' },
      },
      {
        path: 'seed-tools',
        name: 'seed-tools',
        component: () => import('@/views/SeedToolsView.vue'),
        meta: { title: '种子与结构工具' },
      },
      {
        path: 'region-inspect',
        name: 'region-inspect',
        component: () => import('@/views/RegionInspectView.vue'),
        meta: { title: '存档诊断' },
      },
      {
        path: 'resources',
        name: 'resources',
        component: () => import('@/views/ResourcesView.vue'),
        meta: { title: '资源中心' },
      },
      {
        path: 'resources/:slug',
        name: 'resource-detail',
        component: () => import('@/views/ResourceDetailView.vue'),
        meta: { title: '资源详情' },
      },
      {
        path: 'wiki/:slug',
        name: 'wiki-detail',
        component: () => import('@/views/WikiDetailView.vue'),
        meta: { title: '百科详情' },
      },
      {
        path: 'search',
        name: 'search',
        component: () => import('@/views/SearchResultsView.vue'),
        meta: { title: '搜索结果' },
      },
      {
        path: 'about',
        name: 'about',
        component: () => import('@/views/AboutView.vue'),
        meta: { title: '关于本站' },
      },
      {
        path: 'privacy',
        name: 'privacy',
        component: () => import('@/views/PrivacyView.vue'),
        meta: { title: '隐私政策' },
      },
      {
        path: 'copyright',
        name: 'copyright',
        component: () => import('@/views/CopyrightView.vue'),
        meta: { title: '版权声明' },
      },
      {
        path: 'feedback',
        name: 'feedback',
        component: () => import('@/views/FeedbackView.vue'),
        meta: { title: '问题反馈' },
      },
      {
        path: 'login',
        name: 'user-login',
        component: () => import('@/views/UserLoginView.vue'),
        meta: { title: '登录 / 注册', noindex: true },
      },
      {
        path: 'workspace',
        name: 'workspace',
        component: () => import('@/views/WorkspaceView.vue'),
        meta: { title: '我的工作台', requiresUser: true, noindex: true },
      },
    ],
  },
  // ===== 管理后台 =====
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/views/admin/AdminLoginView.vue'),
    meta: { title: '管理员登录', requiresAdmin: false, noindex: true },
  },
  {
    path: '/admin',
    component: AdminLayout,
    meta: { requiresAdmin: true, noindex: true },
    children: [
      {
        path: '',
        name: 'admin-dashboard',
        component: () => import('@/views/admin/AdminDashboardView.vue'),
        meta: { title: '管理后台', requiresAdmin: true, noindex: true },
      },
      {
        path: 'resources',
        name: 'admin-resources',
        component: () => import('@/views/admin/AdminResourcesView.vue'),
        meta: { title: '资源管理', requiresAdmin: true, noindex: true },
      },
      {
        path: 'resources/new',
        name: 'admin-resource-new',
        component: () => import('@/views/admin/AdminResourceEditView.vue'),
        meta: { title: '新建资源', requiresAdmin: true, noindex: true },
      },
      {
        path: 'resources/:id/edit',
        name: 'admin-resource-edit',
        component: () => import('@/views/admin/AdminResourceEditView.vue'),
        meta: { title: '编辑资源', requiresAdmin: true, noindex: true },
      },
      {
        path: 'tools',
        name: 'admin-tools',
        component: () => import('@/views/admin/AdminToolsView.vue'),
        meta: { title: '工具管理', requiresAdmin: true, noindex: true },
      },
      {
        path: 'tools/new',
        name: 'admin-tool-new',
        component: () => import('@/views/admin/AdminToolEditView.vue'),
        meta: { title: '新建工具', requiresAdmin: true, noindex: true },
      },
      {
        path: 'tools/:id/edit',
        name: 'admin-tool-edit',
        component: () => import('@/views/admin/AdminToolEditView.vue'),
        meta: { title: '编辑工具', requiresAdmin: true, noindex: true },
      },
      {
        path: 'wiki',
        name: 'admin-wiki',
        component: () => import('@/views/admin/AdminWikiView.vue'),
        meta: { title: '百科管理', requiresAdmin: true, noindex: true },
      },
      {
        path: 'wiki/new',
        name: 'admin-wiki-new',
        component: () => import('@/views/admin/AdminWikiEditView.vue'),
        meta: { title: '新建百科', requiresAdmin: true, noindex: true },
      },
      {
        path: 'wiki/:id/edit',
        name: 'admin-wiki-edit',
        component: () => import('@/views/admin/AdminWikiEditView.vue'),
        meta: { title: '编辑百科', requiresAdmin: true, noindex: true },
      },
      {
        path: 'categories',
        name: 'admin-categories',
        component: () => import('@/views/admin/AdminCategoriesView.vue'),
        meta: { title: '分类管理', requiresAdmin: true, noindex: true },
      },
      {
        path: 'tags',
        name: 'admin-tags',
        component: () => import('@/views/admin/AdminTagsView.vue'),
        meta: { title: '标签管理', requiresAdmin: true, noindex: true },
      },
      {
        path: 'feedback',
        name: 'admin-feedback',
        component: () => import('@/views/admin/AdminFeedbackView.vue'),
        meta: { title: '反馈管理', requiresAdmin: true, noindex: true },
      },
      {
        path: 'audit-logs',
        name: 'admin-audit-logs',
        component: () => import('@/views/admin/AdminAuditLogsView.vue'),
        meta: { title: '操作日志', requiresAdmin: true, noindex: true },
      },
      {
        path: 'data-quality',
        name: 'admin-data-quality',
        component: () => import('@/views/admin/AdminDataQualityView.vue'),
        meta: { title: '数据质量监控', requiresAdmin: true, noindex: true },
      },
      {
        path: 'search-terms',
        name: 'admin-search-terms',
        component: () => import('@/views/admin/AdminSearchTermsView.vue'),
        meta: { title: '搜索词统计', requiresAdmin: true, noindex: true },
      },
      {
        path: 'link-health',
        name: 'admin-link-health',
        component: () => import('@/views/admin/AdminLinkHealthView.vue'),
        meta: { title: '链接健康', requiresAdmin: true, noindex: true },
      },
      {
        // 红石机器设计内容审核（TODO §15，admin 专属）。
        path: 'redstone',
        name: 'admin-redstone',
        component: () => import('@/views/admin/AdminRedstoneView.vue'),
        meta: { title: '红石设计审核', requiresAdmin: true, noindex: true },
      },
      {
        // Mod 与整合包内容审核（TODO §16，admin 专属）。
        path: 'mods',
        name: 'admin-mods',
        component: () => import('@/views/admin/AdminModsView.vue'),
        meta: { title: 'Mod 审核', requiresAdmin: true, noindex: true },
      },
      {
        path: 'analytics',
        name: 'admin-analytics',
        component: () => import('@/views/admin/AdminAnalyticsView.vue'),
        meta: { title: '运营看板', requiresAdmin: true, noindex: true },
      },
      {
        path: 'admins',
        name: 'admin-admins',
        component: () => import('@/views/admin/AdminAdminsView.vue'),
        meta: { title: '管理员账号', requiresAdmin: true, noindex: true },
      },
      {
        path: 'profile',
        name: 'admin-profile',
        component: () => import('@/views/admin/AdminProfileView.vue'),
        meta: { title: '账号设置', requiresAdmin: true, noindex: true },
      },
    ],
  },
  // 404 兜底
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFoundView.vue'),
    meta: { title: '页面不存在' },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior(to, _from, savedPosition) {
    if (savedPosition) return savedPosition
    if (to.hash) return { el: to.hash, top: 80 }
    return { top: 0 }
  },
})

// 管理路由守卫：仅改善前端体验；真正权限校验在后端
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  const requiresAdmin = to.matched.some((r) => r.meta.requiresAdmin)

  if (requiresAdmin && !auth.initialized) {
    await auth.restoreSession()
  }

  // 未登录访问管理页 -> 登录页（带 redirect）
  if (requiresAdmin && !auth.isLoggedIn) {
    return { path: '/admin/login', query: { redirect: to.fullPath } }
  }

  // 已登录访问登录页 -> 管理首页
  if (to.name === 'admin-login' && auth.isLoggedIn) {
    return { path: '/admin' }
  }

  // 普通用户受限页（工作台）：同样是前端体验优化，后端仍按会话鉴权。
  const userAuth = useUserAuthStore()
  const requiresUser = to.matched.some((r) => r.meta.requiresUser)
  if (requiresUser && !userAuth.initialized) {
    await userAuth.restoreSession()
  }
  if (requiresUser && !userAuth.isLoggedIn) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'user-login' && userAuth.isLoggedIn) {
    return { path: '/workspace' }
  }

  return true
})

// 页面标题与 noindex 管理
router.afterEach((to) => {
  const title =
    (to.meta.title as string | undefined) ?? 'MC 工具站'
  document.title = title === 'MC 工具站' ? title : `${title} · MC 工具站`

  if (to.meta.noindex) {
    applyNoIndexMeta()
  }

  // TODO §13：内置工具页访问量（匿名、best-effort；非内置工具路径自动忽略）。
  reportToolVisit(to.path)
})

export default router