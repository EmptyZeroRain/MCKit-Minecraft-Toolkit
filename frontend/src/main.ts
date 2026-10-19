import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/theme'
import './style.css'

const app = createApp(App)

const pinia = createPinia()
app.use(pinia)
app.use(router)

// 挂载前恢复主题，避免亮色闪烁；在 index.html 中已有内联脚本兜底
useThemeStore(pinia).init()

// 全局错误处理：避免未捕获错误导致白屏
app.config.errorHandler = (err, _instance, info) => {
  console.error('[vue error]', info, err)
}

app.mount('#app')