import { createRoot, type Root } from 'react-dom/client'
import { CommentWidget } from './CommentWidget'
import type { MountOptions, ThemeVars, WidgetHandle } from './public-api'

/**
 * Containers mounted automatically. id="comments" is the original markup; it
 * is only claimed when it carries data-site-id, so a theme's own #comments
 * (WordPress uses that id for native comments) is left alone.
 */
export const AUTO_MOUNT_SELECTOR = '[data-quipthread], [id="comments"][data-site-id]'

// data-qt-* attribute (as a dataset key) and ThemeVars key for each CSS variable.
const THEME_VARS: ReadonlyArray<readonly [cssVar: string, dataKey: string, key: keyof ThemeVars]> =
  [
    ['--qt-accent', 'qtAccent', 'accent'],
    ['--qt-accent-hover', 'qtAccentHover', 'accentHover'],
    ['--qt-bg', 'qtBg', 'bg'],
    ['--qt-bg-alt', 'qtBgAlt', 'bgAlt'],
    ['--qt-text', 'qtText', 'text'],
    ['--qt-text-muted', 'qtTextMuted', 'textMuted'],
    ['--qt-border', 'qtBorder', 'border'],
    ['--qt-radius', 'qtRadius', 'radius'],
    ['--qt-radius-sm', 'qtRadiusSm', 'radiusSm'],
    ['--qt-radius-lg', 'qtRadiusLg', 'radiusLg'],
    ['--qt-font', 'qtFont', 'font'],
  ]

interface ResolvedOptions {
  siteId: string
  pageId: string
  pageUrl?: string
  pageTitle?: string
  lang: string
  theme?: string
  sso?: string
  customVars: Record<string, string>
}

/** Container data-* attributes, overridden by options passed to mount. */
function resolveOptions(el: HTMLElement, options: MountOptions): ResolvedOptions {
  const data = el.dataset
  const customVars: Record<string, string> = {}
  for (const [cssVar, dataKey, key] of THEME_VARS) {
    const value = options.vars?.[key] ?? data[dataKey]
    if (value) customVars[cssVar] = value
  }
  return {
    siteId: options.siteId ?? data.siteId ?? '',
    pageId: options.pageId ?? data.pageId ?? '',
    pageUrl: options.pageUrl ?? data.pageUrl,
    pageTitle: options.pageTitle ?? data.pageTitle,
    lang: options.lang ?? data.lang ?? 'en',
    theme: options.theme ?? data.theme,
    sso: options.sso ?? data.sso,
    customVars,
  }
}

interface Instance {
  readonly root: Root
  readonly handle: WidgetHandle
  options: MountOptions
}

const instances = new WeakMap<Element, Instance>()

const noopHandle: WidgetHandle = { update() {}, unmount() {} }

function render(el: HTMLElement, instance: Instance): boolean {
  const o = resolveOptions(el, instance.options)
  if (!o.siteId || !o.pageId) {
    console.error('[quipthread] siteId and pageId are required (data-site-id and data-page-id)', el)
    return false
  }
  instance.root.render(
    <CommentWidget
      // A different site or page is a different thread: start fresh.
      key={`${o.siteId}\n${o.pageId}`}
      siteId={o.siteId}
      pageId={o.pageId}
      pageUrl={o.pageUrl}
      pageTitle={o.pageTitle}
      lang={o.lang}
      theme={o.theme}
      customVars={o.customVars}
      ssoToken={o.sso}
    />,
  )
  return true
}

export function mount(target: Element | string, options: MountOptions = {}): WidgetHandle {
  const el = typeof target === 'string' ? document.querySelector(target) : target
  if (!(el instanceof HTMLElement)) {
    console.error('[quipthread] mount target not found:', target)
    return noopHandle
  }

  const existing = instances.get(el)
  if (existing) {
    existing.handle.update(options)
    return existing.handle
  }

  const instance: Instance = {
    root: createRoot(el),
    options: { ...options },
    handle: {
      update(next) {
        if (instances.get(el) !== instance) return
        instance.options = { ...instance.options, ...next }
        render(el, instance)
      },
      unmount() {
        if (instances.get(el) !== instance) return
        instances.delete(el)
        instance.root.unmount()
      },
    },
  }
  instances.set(el, instance)
  if (!render(el, instance)) {
    instances.delete(el)
    instance.root.unmount()
    return noopHandle
  }
  return instance.handle
}

/** Mounts every auto-mount container that has no widget yet. */
export function scan(): void {
  for (const el of document.querySelectorAll(AUTO_MOUNT_SELECTOR)) {
    if (!instances.has(el)) mount(el)
  }
}
