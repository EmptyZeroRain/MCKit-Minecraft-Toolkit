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
        // MC 源码浏览器（mcsrc 整合 · 切片 1）：按版本浏览官方 jar 的类索引。
        // 后端下载并缓存 jar、建立类索引；源码正文（反编译）见后续迭代。
        path: 'mcsrc',
        name: 'mcsrc',
        component: () => import('@/views/McsrcView.vue'),
        meta: { title: 'MC 源码浏览器' },
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
      {
        // 成长中心（jingyan.md §11.1）：经验 / 等级 / 每日任务 / 成就展示。
        path: 'growth',
        name: 'growth',
        component: () => import('@/views/GrowthCenterView.vue'),
        meta: { title: '成长中心', requiresUser: true, noindex: true },
      },
      {
        path: 'submit-resource',
        name: 'submit-resource',
        component: () => import('@/views/SubmitResourceView.vue'),
        meta: { title: '资源投稿', requiresUser: true, noindex: true },
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
        // 分类与标签合并页（gj.md §2.2）：同一页面两个 Tab，权限点独立判断。
        path: 'taxonomy',
        name: 'admin-taxonomy',
        component: () => import('@/views/admin/AdminTaxonomyView.vue'),
        meta: { title: '分类与标签', requiresAdmin: true, noindex: true },
      },
      // 兼容旧入口：重定向到合并页并定位到对应 Tab。
      { path: 'categories', redirect: { name: 'admin-taxonomy', query: { tab: 'categories' } } },
      { path: 'tags', redirect: { name: 'admin-taxonomy', query: { tab: 'tags' } } },
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
        // 运营分析合并页（gj.md §2.2）：运营看板 + 搜索词统计，权限点独立。
        path: 'insights',
        name: 'admin-insights',
        component: () => import('@/views/admin/AdminInsightsView.vue'),
        meta: { title: '运营分析', requiresAdmin: true, noindex: true },
      },
      { path: 'search-terms', redirect: { name: 'admin-insights', query: { tab: 'search-terms' } } },
      {
        // 内容建设任务（gj.md §8.3 搜索词闭环）：搜索缺口 → 可指派/可跟踪的运营任务。
        path: 'content-tasks',
        name: 'admin-content-tasks',
        component: () => import('@/views/admin/AdminContentTasksView.vue'),
        meta: { title: '内容建设任务', requiresAdmin: true, noindex: true },
      },
      {
        // 运营告警（gj.md 阶段三）：规则评估 + 确认 / 关闭 + 历史。
        path: 'alerts',
        name: 'admin-alerts',
        component: () => import('@/views/admin/AdminAlertsView.vue'),
        meta: { title: '运营告警', requiresAdmin: true, noindex: true },
      },
      {
        // 后台操作手册（gj.md 阶段四）：按常见疑问组织，替代群内口头答疑。
        path: 'help',
        name: 'admin-help',
        component: () => import('@/views/admin/AdminHelpView.vue'),
        meta: { title: '后台操作手册', requiresAdmin: true, noindex: true },
      },
      {
        // 后台运维任务（gj.md 阶段二）：异步执行 + 自动重试 + 失败详情。
        path: 'jobs',
        name: 'admin-jobs',
        component: () => import('@/views/admin/AdminJobsView.vue'),
        meta: { title: '后台任务', requiresAdmin: true, noindex: true },
      },
      {
        path: 'link-health',
        name: 'admin-link-health',
        component: () => import('@/views/admin/AdminLinkHealthView.vue'),
        meta: { title: '链接健康', requiresAdmin: true, noindex: true },
      },
      {
        // 投稿审核合并页（gj.md §2.2）：红石设计 + Mod/整合包，状态机各自独立。
        path: 'review',
        name: 'admin-review',
        component: () => import('@/views/admin/AdminReviewView.vue'),
        meta: { title: '投稿审核', requiresAdmin: true, noindex: true },
      },
      { path: 'redstone', redirect: { name: 'admin-review', query: { tab: 'redstone' } } },
      { path: 'mods', redirect: { name: 'admin-review', query: { tab: 'mods' } } },
      { path: 'analytics', redirect: { name: 'admin-insights', query: { tab: 'analytics' } } },
      {
        // MC 源码缓存运维（需 ops.mcsrc 权限；后端 403 兜底）。
        path: 'mcsrc-cache',
        name: 'admin-mcsrc-cache',
        component: () => import('@/views/admin/AdminMcsrcView.vue'),
        meta: { title: 'MC 源码缓存', requiresAdmin: true, noindex: true },
      },
      {
        // 账号管理合并页（gj.md §2.2）：管理员 + 普通用户，权限点独立、后端按角色隔离。
        path: 'accounts',
        name: 'admin-accounts',
        component: () => import('@/views/admin/AdminAccountsView.vue'),
        meta: { title: '账号管理', requiresAdmin: true, noindex: true },
      },
      { path: 'admins', redirect: { name: 'admin-accounts', query: { tab: 'admins' } } },
      { path: 'users', redirect: { name: 'admin-accounts', query: { tab: 'users' } } },
      {
        // 角色管理（需 roles.manage 权限点；后端 403 兜底）。
        path: 'roles',
        name: 'admin-roles',
        component: () => import('@/views/admin/AdminRolesView.vue'),
        meta: { title: '角色管理', requiresAdmin: true, noindex: true },
      },
      {
        // 用户成长（jingyan.md §10，需 ops.growth 权限点；后端 403 兜底）。
        path: 'growth',
        name: 'admin-growth',
        component: () => import('@/views/admin/AdminGrowthView.vue'),
        meta: { title: '用户成长', requiresAdmin: true, noindex: true },
      },
      {
        path: 'profile',
        name: 'admin-profile',
        component: () => import('@/views/admin/AdminProfileView.vue'),
        meta: { title: '用户信息', requiresAdmin: true, noindex: true },
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

  // 必须改密：除「用户信息」页（改密锚点）外，全部后台页面强制跳转。
  // 后端对所有需要权限点的接口同样返回 403 PASSWORD_CHANGE_REQUIRED，前端跳转仅为体验优化。
  if (
    requiresAdmin &&
    auth.isLoggedIn &&
    auth.user?.must_change_password &&
    to.path !== '/admin/profile'
  ) {
    return { path: '/admin/profile', hash: '#password' }
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