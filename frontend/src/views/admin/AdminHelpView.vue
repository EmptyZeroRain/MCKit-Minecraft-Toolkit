<script setup lang="ts">
// 后台操作手册（gj.md 阶段四「持续完善可访问性、操作帮助和管理文档」）。
//
// 设计取舍：手册放在页面内而不是只放外链，是因为运营最需要「当前这个按钮点了会发生什么」；
// 内容按「最常见的疑问」组织，每条都给出对应页面入口。
import { RouterLink } from 'vue-router'

interface Section {
  title: string
  intro: string
  items: { q: string; a: string; to?: string; link?: string }[]
}

const sections: Section[] = [
  {
    title: '权限与账号',
    intro: '后台按权限点展示入口，前端隐藏只是减少误操作，后端每个接口都会再校验一次。',
    items: [
      {
        q: '为什么我看不到某个菜单？',
        a: '说明当前角色没有对应权限点。需要新权限时由管理员在「角色管理」里勾选，重新登录后生效（权限缓存在会话中）。',
        to: '/admin/roles',
      },
      {
        q: '首次登录被要求改密码 / 绑定 MFA 怎么办？',
        a: '改密必须先完成：未改密前后台接口会被拦截（写操作额外拦截），改完会注销当前会话，需重新登录。MFA 绑定后请立即保存恢复码，恢复码只显示一次。',
        to: '/admin/profile',
      },
      {
        q: '怀疑账号被盗怎么处理？',
        a: '在「用户信息」里撤销全部会话，然后改密并检查「操作日志 → 安全事件」与登录来源 IP；安全事件视图只显示登录 / 改密 / MFA / 会话撤销。',
        to: '/admin/audit-logs',
      },
    ],
  },
  {
    title: '内容审核',
    intro: '审核动作会记录审核人、结果与备注，并生成审计日志。',
    items: [
      {
        q: '如何一次处理多条投稿？',
        a: '在列表左侧勾选后使用底部批量工具条（批量通过 / 驳回 + 备注），单次上限 50 条。每条独立事务，部分失败属正常情况，失败明细会逐条列出，成功的不会回滚。',
        to: '/admin/submissions',
      },
      {
        q: '改错了内容怎么回滚？',
        a: '资料编辑页有「历史版本」，可查看差异并回滚。回滚不是删除历史，而是把旧快照写回并追加一条 rollback 版本，随时可以再滚回来。',
        to: '/admin/resources',
      },
      {
        q: '删除前怎么知道影响范围？',
        a: '删除确认弹窗会先做一次只读的影响预览（关联数据、引用数）。预览失败不会阻止删除，但会明确提示「影响未知」，此时建议先查日志再决定。',
        to: '/admin/resources',
      },
    ],
  },
  {
    title: '运营与运维',
    intro: '告警负责「发现问题」，任务负责「把耗时动作跑完」，两者都不需要人盯着。',
    items: [
      {
        q: '收到告警后应该做什么？',
        a: '在「运营告警」点「确认」表示已有人接手（不会被自动关闭）；问题消失后系统会自动关闭并标记 resolved_by=system。确认与关闭都会写审计，历史不删除。',
        to: '/admin/alerts',
      },
      {
        q: '任务失败了怎么重试？',
        a: '「后台任务」里可看到失败详情（错误信息已脱敏）。自动重试有次数上限；人工重试会新建一条任务并保留原始失败现场，便于对比。执行中的任务不能取消。',
        to: '/admin/jobs',
      },
      {
        q: 'MC 源码缓存占用太高怎么办？',
        a: '缓存是可重建的派生数据，清理不会丢业务数据。可对单个版本点「重建」（清理 → 重新下载 → 重建索引，以后台任务执行），或整体清空；占用达到阈值会触发容量告警。',
        to: '/admin/mcsrc-cache',
      },
      {
        q: '数据质量问题谁来跟进？',
        a: '每个检查项可指派负责人、写处理备注、标记已解决 / 忽略。检查结果实时重算，所以「已解决」不会自动关闭；问题再次出现时按 snapshot_count 对比处理进度。',
        to: '/admin/data-quality',
      },
    ],
  },
  {
    title: '导出与数据',
    intro: '导出全部为只读操作，单次上限 5000 条（用户导出 10000 条），超出会在响应头标记截断。',
    items: [
      {
        q: '导出的 CSV 用 Excel 打开乱码？',
        a: '文件已带 UTF-8 BOM，若仍乱码请用「数据 → 从文本/CSV」导入并选择 UTF-8。以 = + - @ 开头的内容会被自动加单引号，防止公式注入。',
      },
      {
        q: '导出会不会泄露敏感信息？',
        a: '不会：审计日志不记录密码、Cookie、Authorization 与连接串；导出动作本身会写一条 audit.export 审计（含条数与筛选条件）。',
        to: '/admin/audit-logs',
      },
    ],
  },
  {
    title: '无障碍与键盘操作',
    intro: '后台面向长时间高频操作，键盘可达性与可读性是硬要求。',
    items: [
      {
        q: '可以用键盘完成审核吗？',
        a: '可以：所有按钮与表单控件都是原生元素（按钮 / 输入框 / 下拉），Tab 顺序与视觉顺序一致；对话框打开后焦点会被限制在框内，Esc 关闭。',
      },
      {
        q: '颜色之外还有别的提示吗？',
        a: '状态除了颜色还有文字标签（如「已发布」「失败」），结果列同时给出文字；图表类页面有数值文本，不依赖颜色区分。',
      },
    ],
  },
]

// 仓库文档（不放外链，避免指向不存在的路由）：需要细节时按文件名查 docs/ 目录。
const docs = [
  '后台功能.md（页面 / 接口 / 权限总览）',
  'docs/后台操作手册.md',
  'docs/账号与密码策略.md',
  'docs/管理员MFA.md',
  'docs/部署与会话方案.md',
  'docs/备份恢复与迁移规程.md',
]
</script>

<template>
  <div class="mx-auto max-w-4xl space-y-6">
    <header>
      <h1 class="text-xl font-bold text-text-primary md:text-2xl">后台操作手册</h1>
      <p class="mt-1 text-sm text-text-secondary">
        按「常见疑问」组织，每条都指向对应页面。规范细节见项目仓库 gj.md 与 docs/ 目录。
      </p>
    </header>

    <section
      v-for="section in sections"
      :key="section.title"
      class="card p-5"
      :aria-labelledby="`help-${section.title}`"
    >
      <h2 :id="`help-${section.title}`" class="text-base font-semibold text-text-primary">
        {{ section.title }}
      </h2>
      <p class="mt-1 text-sm text-text-secondary">{{ section.intro }}</p>

      <dl class="mt-4 space-y-4">
        <div v-for="item in section.items" :key="item.q">
          <dt class="text-sm font-medium text-text-primary">{{ item.q }}</dt>
          <dd class="mt-1 text-sm leading-relaxed text-text-secondary">
            {{ item.a }}
            <RouterLink
              v-if="item.to"
              :to="item.to"
              class="ml-1 whitespace-nowrap text-grass-300 hover:underline"
            >
              前往 →
            </RouterLink>
          </dd>
        </div>
      </dl>
    </section>

    <section class="card p-5" aria-labelledby="help-docs">
      <h2 id="help-docs" class="text-base font-semibold text-text-primary">延伸阅读</h2>
      <ul class="mt-3 list-inside list-disc space-y-1 text-sm text-text-secondary">
        <li v-for="doc in docs" :key="doc">{{ doc }}</li>
      </ul>
    </section>
  </div>
</template>
