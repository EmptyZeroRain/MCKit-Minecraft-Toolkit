<script setup lang="ts">
// 关于本站（需求 0.1 第 9 条：非官方声明）
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '关于本站',
  canonicalPath: '/about',
})
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <Breadcrumbs :items="[{ label: '关于本站' }]" />
    <h1 class="mb-6 text-2xl font-bold text-text-primary md:text-3xl">关于本站</h1>

    <div class="card space-y-5 p-6 leading-relaxed text-text-secondary">
      <section>
        <h2 class="mb-2 font-semibold text-text-primary">非官方声明</h2>
        <p>
          MC 工具站是<strong class="text-text-primary">非官方社区项目</strong>，与 Mojang Studios、Microsoft
          及其关联公司无任何从属或赞助关系。Minecraft 及相关商标归
          Mojang Studios / Microsoft 所有，其名称与素材的使用仅用于识别和导航目的。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">项目介绍</h2>
        <p>
          本站面向 Minecraft Java 版玩家、整合包玩家、服务器管理者和 Mod 开发者，提供常用在线工具
          （合成配方查询、坐标换算、物品 ID 查询、指令格式辅助）与资源导航（Mod、整合包、资源包、光影等）。
        </p>
        <p class="mt-2">
          资源仅做索引与原始来源跳转，不托管、不代理、不镜像第三方文件；内容版权归原作者所有。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">数据与合规</h2>
        <ul class="list-disc space-y-1 pl-5">
          <li>收藏与最近使用仅保存在你的浏览器本地，不上传服务器。</li>
          <li>页面展示的数据带版本与来源标注，未核实信息不写成确定事实。</li>
          <li>本站不使用未经授权的第三方数据库或受版权保护的抓取内容。</li>
        </ul>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">联系方式</h2>
        <p>如发现错误、失效链接或有侵权反馈，欢迎前往 <RouterLink to="/feedback" class="text-grass-400 hover:underline">问题反馈</RouterLink> 页面提交。</p>
      </section>
    </div>
  </PageContainer>
</template>