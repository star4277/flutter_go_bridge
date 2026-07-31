import type { DefaultTheme, LocaleSpecificConfig } from 'vitepress'
import { repo } from '../shared'

const guide: DefaultTheme.SidebarItem = {
  text: '指南',
  collapsed: false,
  items: [
    { text: 'flutter_go_bridge 是什么', link: '/zh/guide/' },
    { text: '快速开始', link: '/zh/guide/getting-started' },
    { text: '配置', link: '/zh/guide/configuration' },
    { text: '输出结构', link: '/zh/guide/output-structure' },
    { text: 'CLI', link: '/zh/guide/cli' },
    { text: '开发服务器（run）', link: '/zh/guide/dev-server' },
    { text: '用 Gokit 构建', link: '/zh/guide/gokit' },
    { text: '文档站开发', link: '/zh/guide/docs-development' },
  ],
}

const concepts: DefaultTheme.SidebarItem = {
  text: '概念',
  collapsed: false,
  items: [
    { text: '设计约定', link: '/zh/concepts/' },
    { text: '序列化策略', link: '/zh/concepts/serialization' },
    { text: '同步与异步', link: '/zh/concepts/sync-async' },
    { text: '稳定 ABI', link: '/zh/concepts/stable-abi' },
    { text: '生成的支持包', link: '/zh/concepts/support-package' },
  ],
}

const reference: DefaultTheme.SidebarItem = {
  text: '参考',
  collapsed: false,
  items: [
    { text: '类型映射', link: '/zh/reference/type-mapping' },
    { text: '指令与字段 tag', link: '/zh/reference/directives' },
    { text: '结构体与接口', link: '/zh/reference/structs-interfaces' },
    { text: '返回值与 error', link: '/zh/reference/returns-errors' },
    { text: 'Stream', link: '/zh/reference/stream' },
    { text: 'Dart 闭包回调', link: '/zh/reference/callbacks' },
  ],
}

export const zh: LocaleSpecificConfig<DefaultTheme.Config> & { label: string; link?: string } = {
  label: '简体中文',
  lang: 'zh-CN',
  description: '面向 Gokit 的 Go → Dart/Flutter 代码生成器，不依赖 Flutter SDK。',

  themeConfig: {
    nav: [
      { text: '指南', link: '/zh/guide/', activeMatch: '^/zh/guide/' },
      { text: '概念', link: '/zh/concepts/', activeMatch: '^/zh/concepts/' },
      { text: '参考', link: '/zh/reference/type-mapping', activeMatch: '^/zh/reference/' },
      { text: 'Gokit', link: '/zh/guide/gokit' },
      { text: '文档开发', link: '/zh/guide/docs-development' },
    ],

    sidebar: {
      '/zh/guide/': [guide, concepts, reference],
      '/zh/concepts/': [guide, concepts, reference],
      '/zh/reference/': [guide, concepts, reference],
    },

    editLink: {
      pattern: `${repo}/edit/main/docs/:path`,
      text: '在 GitHub 上编辑此页',
    },

    docFooter: { prev: '上一页', next: '下一页' },
    outline: { label: '本页目录' },
    lastUpdated: { text: '最后更新于' },
    darkModeSwitchLabel: '主题',
    lightModeSwitchTitle: '切换到浅色主题',
    darkModeSwitchTitle: '切换到深色主题',
    sidebarMenuLabel: '菜单',
    returnToTopLabel: '回到顶部',
    langMenuLabel: '切换语言',

    footer: {
      message: '基于 MIT 许可发布。',
      copyright: `版权所有 © ${new Date().getFullYear()} flutter_go_bridge 贡献者`,
    },
  },
}
