<script setup lang="ts">
import PageContainer from '@/components/layout/PageContainer.vue'
import Breadcrumbs from '@/components/layout/Breadcrumbs.vue'
import { applyPageMeta } from '@/utils/seo'

applyPageMeta({
  title: '版权与侵权',
  canonicalPath: '/copyright',
})
</script>

<template>
  <PageContainer maxWidth="max-w-3xl">
    <Breadcrumbs :items="[{ label: '版权与侵权' }]" />
    <h1 class="mb-6 text-2xl font-bold text-text-primary md:text-3xl">版权与侵权联系</h1>

    <div class="card space-y-6 p-6 text-sm leading-relaxed text-text-secondary">
      <section>
        <h2 class="mb-2 font-semibold text-text-primary">版权声明</h2>
        <p>
          本站为<strong class="text-text-primary">非官方社区项目</strong>。Minecraft 及相关商标、
          标识归 Mojang Studios / Microsoft 所有。网站界面与独立工具逻辑的版权归本站所有。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">资源内容版权</h2>
        <p>
          本站提供的资源均为<strong class="text-text-primary">索引链接</strong>，不存储任何第三方文件。
          各资源的文本、图片、Mod 本体等版权归原作者或权利方所有。
        </p>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">侵权处理流程</h2>
        <ol class="list-decimal space-y-1 pl-5">
          <li>通过反馈页面提交<strong>侵权通知</strong>，请附上权利证明与具体资源链接。</li>
          <li>收到通知后我们会在合理时间内核实并处理（隐藏相关链接或下架条目）。</li>
          <li>处理完成后会通过你留下的联系方式（如有）反馈结果。</li>
        </ol>
      </section>

      <section>
        <h2 class="mb-2 font-semibold text-text-primary">提交入口</h2>
        <p>
          请前往 <RouterLink to="/feedback" class="text-grass-400 hover:underline">问题反馈</RouterLink>
          页面，类型选择“侵权通知”提交。
        </p>
      </section>
    </div>
  </PageContainer>
</template>