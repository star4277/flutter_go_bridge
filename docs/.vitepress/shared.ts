import { defineConfig } from 'vitepress'

/** Canonical repository, reused by nav links, edit links and social links. */
export const repo = 'https://github.com/star4277/flutter_go_bridge'

/**
 * Locale-independent configuration. Anything that reads as prose lives in
 * `locales/en.ts` and `locales/zh.ts` instead, so a new language only has to
 * add one file.
 */
export const shared = defineConfig({
  title: 'flutter_go_bridge',
  lastUpdated: true,
  cleanUrls: true,
  metaChunk: true,

  // A broken cross-locale link is the most common regression in a translated
  // site, so fail the build instead of shipping it.
  ignoreDeadLinks: false,

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }],
    ['meta', { name: 'theme-color', content: '#0175C2' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'flutter_go_bridge' }],
  ],

  markdown: {
    // Line numbers pay off here: most snippets are Go or Dart source that the
    // surrounding prose refers to by position.
    lineNumbers: false,
    theme: { light: 'github-light', dark: 'github-dark' },
  },

  themeConfig: {
    logo: '/logo.svg',
    socialLinks: [{ icon: 'github', link: repo }],
    externalLinkIcon: true,
    outline: [2, 3],
  },
})
