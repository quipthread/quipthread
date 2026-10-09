// embed.js: the classic script sites include. It stays tiny and loads the
// widget as an ES module from the same origin, so readers only download the
// code they need. Kept free of imports so the build emits it as plain script.
import type {
  MountOptions,
  QueuedMount,
  QuipthreadAPI,
  StartContext,
  WidgetHandle,
} from './public-api'

declare global {
  interface Window {
    Quipthread?: QuipthreadAPI
  }
}

// Replaced at build time with the widget module's hashed path.
declare const __QT_WIDGET_PATH__: string

;(() => {
  // A second copy of embed.js (some SPA setups inject it on every route)
  // only looks for new containers.
  if (window.Quipthread) {
    window.Quipthread.scan()
    return
  }

  // document.currentScript is only set while this script first runs.
  const script = document.currentScript as HTMLScriptElement | null
  if (!script?.src) {
    console.error('[quipthread] embed.js must be loaded with a <script src> tag')
    return
  }
  const scriptUrl = new URL(script.src)

  const queued: QueuedMount[] = []
  window.Quipthread = {
    version: 'loading',
    mount(target: Element | string, options?: MountOptions): WidgetHandle {
      // Calls on the handle wait until the widget has mounted.
      let real: WidgetHandle | null = null
      const pending: Array<(handle: WidgetHandle) => void> = []
      queued.push({
        target,
        options,
        attach(handle) {
          real = handle
          for (const call of pending.splice(0)) call(handle)
        },
      })
      return {
        update: (next) => (real ? real.update(next) : pending.push((h) => h.update(next))),
        unmount: () => (real ? real.unmount() : pending.push((h) => h.unmount())),
      }
    },
    scan() {
      // The widget scans the page when it starts.
    },
  }

  import(/* @vite-ignore */ new URL(__QT_WIDGET_PATH__, scriptUrl).href)
    .then((widget: { start(context: StartContext): void }) =>
      widget.start({ apiBase: scriptUrl.origin, queued }),
    )
    .catch((error: unknown) =>
      console.error('[quipthread] failed to load the comment widget', error),
    )
})()
