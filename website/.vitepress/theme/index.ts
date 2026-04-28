import DefaultTheme from 'vitepress/theme'
import { h, onMounted } from 'vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  Layout() {
    onMounted(() => {
      const path = window.location.pathname
      const isRoot = path === '/' || path === '/index.html'
      if (!isRoot) {
        return
      }

      const languages = navigator.languages?.length ? navigator.languages : [navigator.language]
      const prefersChinese = languages.some((lang) => String(lang).toLowerCase().startsWith('zh'))
      if (prefersChinese) {
        window.location.replace('/zh/')
      }
    })

    return h(DefaultTheme.Layout)
  }
}
