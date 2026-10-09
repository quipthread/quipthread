// The public JavaScript API, shared by the loader (embed.js) and the widget
// module. Types only: the loader must stay free of runtime imports.

/** CSS variable overrides, the JS form of the data-qt-* attributes. */
export interface ThemeVars {
  accent?: string
  accentHover?: string
  bg?: string
  bgAlt?: string
  text?: string
  textMuted?: string
  border?: string
  radius?: string
  radiusSm?: string
  radiusLg?: string
  font?: string
}

/** Options for Quipthread.mount. They override the container's data-* attributes. */
export interface MountOptions {
  siteId?: string
  pageId?: string
  pageUrl?: string
  pageTitle?: string
  lang?: string
  theme?: string
  sso?: string
  vars?: ThemeVars
}

export interface WidgetHandle {
  /** Re-renders with changed options; a new siteId or pageId starts a fresh thread view. */
  update(options: MountOptions): void
  /** Removes the widget and frees the container. */
  unmount(): void
}

export interface QuipthreadAPI {
  readonly version: string
  /**
   * Mounts the widget into target (an element or a CSS selector). Mounting
   * an element that already has a widget updates it and returns its handle.
   */
  mount(target: Element | string, options?: MountOptions): WidgetHandle
  /** Mounts every auto-mount container on the page that has no widget yet. */
  scan(): void
}

/** A mount requested before the widget module finished loading. */
export interface QueuedMount {
  readonly target: Element | string
  readonly options: MountOptions | undefined
  readonly attach: (handle: WidgetHandle) => void
}

/** What the loader passes to the widget module's start(). */
export interface StartContext {
  readonly apiBase: string
  readonly queued: readonly QueuedMount[]
}
