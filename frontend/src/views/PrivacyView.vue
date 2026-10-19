<script setup lang="ts">
// 隐私政策（需求 11.2）
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '隐私政策',
  canonicalPath: '/privacy',
})
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <Breadcrumbs :items="[{ label: '隐私政策' }]" />
    <h1 class="mb-6 text-2xl font-bold text-text-primary md:text-3xl">隐私政策</h1>

    <div class="card space-y-6 p-6 text-sm leading-relaxed text-text-secondary">
      <section>
        <h2 class="mb-2 font-semibold text-text-primary">1. 账号可选，公开工具免登录</h2>
        <p>
          浏览与使用本站全部公开工具无需注册、无需登录，也无需提供任何个人信息。
          注册普通用户账号是<strong>可选</strong>功能，仅用于收藏与个人工作台的跨设备同步。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">2. 浏览器本地存储</h2>
        <p>未登录时，收藏与最近使用功能将数据保存在你的浏览器 localStorage 中，仅在你的设备上，不会上传服务器。</p>
        <p class="mt-2">你可以随时在浏览器设置中清除站点数据来删除这些本地记录。本地数据损坏时页面会自动安全重置，不影响正常使用。</p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">3. 账号数据与保留期限</h2>
        <p>
          若你注册账号，我们仅保存：用户名、昵称、注册邮箱、密码哈希（Argon2id，不可逆，不保存明文密码），
          以及你主动创建的收藏、最近使用与常用种子。
          邮箱仅用于登录与账号找回，不会公开展示；我们不会额外采集手机号、设备标识等不必要信息。
        </p>
        <p class="mt-2">保留期限：</p>
        <ul class="mt-1 list-disc space-y-1 pl-5">
          <li>最近使用：仅保留<strong>最近 50 条</strong>，超出后自动裁剪旧记录；你可随时一键清空。</li>
          <li>收藏与常用种子：保留至你主动删除为止。</li>
          <li>删除账号：账号及其全部个人数据（收藏 / 最近使用 / 常用种子）<strong>立即永久清除</strong>，不可恢复。</li>
        </ul>
        <p class="mt-2">
          登录态通过服务端会话 Cookie 维持，Cookie 为 HttpOnly，前端 JavaScript 无法读取；
          会话过期后需重新登录。写操作均需 CSRF 校验，登录失败有频率限制。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">4. 反馈信息</h2>
        <p>
          当你提交反馈时，我们会收集你填写的反馈类型、标题、内容，以及可选的关联 URL
          和联系方式。联系方式仅用于处理反馈所需，不会用于其他用途。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">5. 资源投稿</h2>
        <p>
          当你提交资源投稿时，我们会收集你填写的资源信息（名称、简介、描述、类型、作者、
          来源平台与来源链接、许可协议、游戏版本、适用版本与加载器、安装说明）、给审核者的说明，
          以及投稿授权条款的版本与确认时间。投稿内容默认进入<strong>待审核</strong>状态，
          审核通过后由本站整理为资源记录（默认草稿，核验后发布）。
        </p>
        <p class="mt-2">
          投稿与你账号绑定：我们会记录提交者用户名快照；删除账号时你的投稿记录一并删除
          （已生成的资源不随投稿删除，仅解除关联）。投稿仅用于内容整理与版权追溯，不用于其他用途。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">6. 日志与安全</h2>
        <p>
          为保障网站安全与排障，服务端会记录基础的访问日志（时间、路径、状态码、请求 ID、
          来源 IP）。日志不会记录密码、Cookie、Authorization 头或敏感请求内容。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">7. 匿名使用统计</h2>
        <p>
          为了解「哪些工具真被使用、哪些环节在出错」，本站会以<strong>匿名、按天聚合</strong>的方式统计使用情况，
          用于运营看板（仅管理员可见）与功能改进。我们尽量少收集：
        </p>
        <ul class="mt-2 list-disc space-y-1 pl-5">
          <li>
            <strong>工具使用</strong>：内置工具页面的访问、成功使用与失败次数（“使用”以执行核心动作或复制工具产出为信号）。
          </li>
          <li><strong>资源与搜索</strong>：资源详情页访问量、搜索次数，以及<strong>归一化后的搜索关键词</strong>与无结果 / 失败计数。</li>
          <li><strong>接口健康</strong>：接口请求量、状态码分布与<strong>分桶近似</strong>的延迟（不记录逐次耗时）。</li>
        </ul>
        <p class="mt-2">
          统计数据<strong>不包含</strong> IP 地址、会话与用户标识、Cookie、User-Agent、请求体或完整查询内容；
          接口统计只保留路由模板（例如 <code>/api/v1/resources/{slug}</code>），不保留具体参数值；
          也<strong>不建立</strong>与你个人的关联。聚合数据保留期限默认 <strong>180 天</strong>，到期自动清理。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">8. 第三方外链</h2>
        <p>
          本站资源详情页会跳转到第三方原始发布平台。跳转后你在第三方网站上的行为受该网站隐私政策约束。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">9. 政策更新</h2>
        <p>本政策可能随功能演进更新，重大变更会在页面显著位置说明。</p>
      </section>
    </div>
  </PageContainer>
</template>