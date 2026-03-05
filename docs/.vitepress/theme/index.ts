import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import OSTabs from './components/OSTabs.vue'
import './styles/os-tabs.css'

const theme: Theme = {
  ...DefaultTheme,
  enhanceApp({ app }) {
    app.component('OSTabs', OSTabs)
  }
}

export default theme
