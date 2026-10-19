// 确认对话框的可访问性（gj.md 阶段四：无障碍持续完善）
//   1) 打开后焦点落在确认按钮；
//   2) Tab / Shift+Tab 在对话框内循环，不会跑到背景页面；
//   3) Esc 取消；
//   4) 关闭后焦点还原到打开前的元素。
import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

async function settle() {
  await nextTick()
  await new Promise((r) => requestAnimationFrame(() => r(null)))
  await nextTick()
}

describe('ConfirmDialog 可访问性', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('打开后聚焦确认按钮，关闭后还原焦点', async () => {
    const trigger = document.createElement('button')
    trigger.textContent = '删除'
    document.body.appendChild(trigger)
    trigger.focus()
    expect(document.activeElement).toBe(trigger)

    const wrapper = mount(ConfirmDialog, {
      props: { open: false, title: '确认删除', message: '不可撤销' },
      attachTo: document.body,
    })
    await wrapper.setProps({ open: true })
    await settle()

    const confirm = document.querySelector<HTMLButtonElement>('button[data-confirm]')
    expect(document.activeElement).toBe(confirm)

    await wrapper.setProps({ open: false })
    await settle()
    expect(document.activeElement).toBe(trigger)
  })

  it('Tab 在对话框内循环，Shift+Tab 反向循环', async () => {
    const wrapper = mount(ConfirmDialog, {
      props: { open: true, title: '确认', message: '内容' },
      attachTo: document.body,
    })
    await settle()

    const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button'))
    expect(buttons.length).toBe(2)
    const [cancel, confirm] = buttons

    confirm.focus()
    confirm.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    expect(document.activeElement).toBe(cancel)

    cancel.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true }),
    )
    expect(document.activeElement).toBe(confirm)

    wrapper.unmount()
  })

  it('Esc 触发取消', async () => {
    const wrapper = mount(ConfirmDialog, {
      props: { open: true, title: '确认', message: '内容' },
      attachTo: document.body,
    })
    await settle()

    document
      .querySelector<HTMLButtonElement>('button[data-confirm]')!
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))

    expect(wrapper.emitted('cancel')).toBeTruthy()
    wrapper.unmount()
  })
})
