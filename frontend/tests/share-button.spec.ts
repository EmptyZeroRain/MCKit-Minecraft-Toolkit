// 分享按钮组件测试（TODO §14）
// 核心断言：复制到剪贴板的内容一定不含机密查询参数或 hash 中的 token。
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import ShareButton from '@/components/common/ShareButton.vue'
import { useUiStore } from '@/stores/ui'

async function mockClipboard(ok = true) {
  const writeText = vi.fn().mockResolvedValue(ok ? undefined : Promise.reject(new Error('denied')))
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
  Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true })
  return writeText
}

describe('ShareButton', () => {
  it('复制自定义 URL，并剥离机密参数', async () => {
    setActivePinia(createPinia())
    const writeText = await mockClipboard()
    const wrapper = mount(ShareButton, {
      props: { url: 'https://mc.example.com/seed-map?seed=42&csrf_token=leak#access_token=x' },
    })

    await wrapper.trigger('click')

    expect(writeText).toHaveBeenCalledWith('https://mc.example.com/seed-map?seed=42')
    expect(wrapper.text()).toContain('已复制')
  })

  it('未传 url 时复制当前地址栏链接（Cookie / CSRF 从不在 URL 中）', async () => {
    setActivePinia(createPinia())
    const writeText = await mockClipboard()
    window.history.replaceState({}, '', '/block-ids?q=stone')
    const wrapper = mount(ShareButton)

    await wrapper.trigger('click')

    expect(writeText).toHaveBeenCalledTimes(1)
    const copied = writeText.mock.calls[0][0] as string
    expect(copied).toContain('/block-ids?q=stone')
    expect(copied).not.toMatch(/token|csrf|session|cookie|password/i)
  })

  it('复制失败时提示手动复制，且不显示“已复制”', async () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    await mockClipboard(false)
    const wrapper = mount(ShareButton, { props: { url: 'https://mc.example.com/a' } })

    await wrapper.trigger('click')
    await new Promise((r) => setTimeout(r, 10))

    expect(wrapper.text()).not.toContain('已复制')
    expect(ui.toasts.at(-1)?.type).toBe('error')
  })

  it('支持自定义按钮文案（可访问性名称同步更新）', async () => {
    setActivePinia(createPinia())
    await mockClipboard()
    const wrapper = mount(ShareButton, { props: { url: 'https://mc.example.com/a', label: '分享此配置' } })

    expect(wrapper.text()).toContain('分享此配置')
    expect(wrapper.attributes('aria-label')).toBe('分享此配置')
  })
})
