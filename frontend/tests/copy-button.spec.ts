// 复制按钮组件测试（需求 13.3：复制按钮反馈）
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CopyButton from '@/components/common/CopyButton.vue'
import { createPinia, setActivePinia } from 'pinia'
import { useUiStore } from '@/stores/ui'

async function mockClipboard(ok: boolean) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: vi.fn().mockResolvedValue(ok ? undefined : Promise.reject(new Error('denied'))),
    },
  })
  Object.defineProperty(window, 'isSecureContext', {
    configurable: true,
    value: true,
  })
}

describe('CopyButton', () => {
  it('点击后调用 Clipboard API 并切换为“已复制”', async () => {
    setActivePinia(createPinia())
    await mockClipboard(true)
    const wrapper = mount(CopyButton, { props: { text: 'minecraft:stone' } })

    await wrapper.trigger('click')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('minecraft:stone')
    expect(wrapper.text()).toContain('已复制')
  })

  it('复制失败时提示手动复制，不切换“已复制”', async () => {
    setActivePinia(createPinia())
    const ui = useUiStore()
    await mockClipboard(false)
    const wrapper = mount(CopyButton, { props: { text: 'abc' } })

    await wrapper.trigger('click')
    await new Promise((r) => setTimeout(r, 10))
    expect(wrapper.text()).not.toContain('已复制')
    expect(ui.toasts.length).toBeGreaterThan(0)
    expect(ui.toasts[0].type).toBe('error')
  })
})