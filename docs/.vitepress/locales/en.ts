import type { DefaultTheme, LocaleSpecificConfig } from 'vitepress'
import { repo } from '../shared'

const guide: DefaultTheme.SidebarItem = {
  text: 'Guide',
  collapsed: false,
  items: [
    { text: 'What is flutter_go_bridge?', link: '/guide/' },
    { text: 'Getting started', link: '/guide/getting-started' },
    { text: 'Configuration', link: '/guide/configuration' },
    { text: 'Output structure', link: '/guide/output-structure' },
    { text: 'CLI', link: '/guide/cli' },
    { text: 'Dev server (run)', link: '/guide/dev-server' },
    { text: 'Building with Gokit', link: '/guide/gokit' },
    { text: 'Releasing', link: '/guide/releasing' },
    { text: 'Contributing to docs', link: '/guide/docs-development' },
  ],
}

const concepts: DefaultTheme.SidebarItem = {
  text: 'Concepts',
  collapsed: false,
  items: [
    { text: 'Design principles', link: '/concepts/' },
    { text: 'Serialization strategy', link: '/concepts/serialization' },
    { text: 'Sync and async', link: '/concepts/sync-async' },
    { text: 'The stable ABI', link: '/concepts/stable-abi' },
    { text: 'Generated support package', link: '/concepts/support-package' },
  ],
}

const reference: DefaultTheme.SidebarItem = {
  text: 'Reference',
  collapsed: false,
  items: [
    { text: 'Type mapping', link: '/reference/type-mapping' },
    { text: 'Directives and tags', link: '/reference/directives' },
    { text: 'Structs and interfaces', link: '/reference/structs-interfaces' },
    { text: 'Returns and errors', link: '/reference/returns-errors' },
    { text: 'Stream', link: '/reference/stream' },
    { text: 'Dart closure callbacks', link: '/reference/callbacks' },
  ],
}

export const en: LocaleSpecificConfig<DefaultTheme.Config> & { label: string; link?: string } = {
  label: 'English',
  lang: 'en-US',
  description: 'Go to Dart/Flutter code generator for Gokit. No Flutter SDK dependency.',

  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/', activeMatch: '^/guide/' },
      { text: 'Concepts', link: '/concepts/', activeMatch: '^/concepts/' },
      { text: 'Reference', link: '/reference/type-mapping', activeMatch: '^/reference/' },
      { text: 'Gokit', link: '/guide/gokit' },
      { text: 'Docs development', link: '/guide/docs-development' },
    ],

    sidebar: {
      '/guide/': [guide, concepts, reference],
      '/concepts/': [guide, concepts, reference],
      '/reference/': [guide, concepts, reference],
    },

    editLink: {
      pattern: `${repo}/edit/main/docs/:path`,
      text: 'Edit this page on GitHub',
    },

    docFooter: { prev: 'Previous', next: 'Next' },
    outline: { label: 'On this page' },
    lastUpdated: { text: 'Last updated' },
    darkModeSwitchLabel: 'Appearance',
    lightModeSwitchTitle: 'Switch to light theme',
    darkModeSwitchTitle: 'Switch to dark theme',
    sidebarMenuLabel: 'Menu',
    returnToTopLabel: 'Return to top',
    langMenuLabel: 'Change language',

    footer: {
      message: 'Released under the MIT License.',
      copyright: `Copyright © ${new Date().getFullYear()} flutter_go_bridge contributors`,
    },
  },
}
