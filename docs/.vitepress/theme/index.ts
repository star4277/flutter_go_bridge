import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import './style.css'

/**
 * The Bridge theme is a re-skin of the default theme rather than a
 * replacement: all customisation is expressed as CSS custom properties and a
 * handful of component overrides, so upstream layout fixes keep flowing in.
 */
export default {
  extends: DefaultTheme,
} satisfies Theme
