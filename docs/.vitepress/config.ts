import { defineConfig } from 'vitepress'
import { shared } from './shared'
import { en } from './locales/en'
import { zh } from './locales/zh'

export default defineConfig({
  ...shared,

  // English is the root locale; Simplified Chinese is served from /zh/.
  locales: {
    root: { ...en, link: '/' },
    zh: { ...zh, link: '/zh/' },
  },

  themeConfig: {
    ...shared.themeConfig,

    // Local search needs its own per-locale strings; the `root` key maps to
    // English and `zh` to the /zh/ tree.
    search: {
      provider: 'local',
      options: {
        locales: {
          zh: {
            translations: {
              button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
              modal: {
                displayDetails: '显示详细列表',
                resetButtonTitle: '清除查询条件',
                backButtonTitle: '返回',
                noResultsText: '无法找到相关结果',
                footer: {
                  selectText: '选择',
                  selectKeyAriaLabel: '回车',
                  navigateText: '切换',
                  navigateUpKeyAriaLabel: '上箭头',
                  navigateDownKeyAriaLabel: '下箭头',
                  closeText: '关闭',
                  closeKeyAriaLabel: 'esc',
                },
              },
            },
          },
        },
      },
    },
  },
})
