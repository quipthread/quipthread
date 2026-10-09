import { initApi } from './api'
import { mount, scan } from './mount'
import type { StartContext } from './public-api'
import { injectStyles } from './styles'

declare const __QT_VERSION__: string

/**
 * Entry point of the widget module, called by embed.js once it has loaded.
 * Installs the real API, mounts what the page queued before the module
 * arrived, then mounts the page's auto-mount containers.
 */
export function start({ apiBase, queued }: StartContext): void {
  injectStyles()
  initApi(apiBase)
  window.Quipthread = { version: __QT_VERSION__, mount, scan }

  for (const { target, options, attach } of queued) {
    attach(mount(target, options))
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', scan, { once: true })
  } else {
    scan()
  }
}
