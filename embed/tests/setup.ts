import { plugin } from 'bun'
import { GlobalWindow } from 'happy-dom'
import { preactAliases } from '../preact-aliases'

// Run the tests on Preact, as the bundle does (see preact-aliases.ts). Bun's
// runtime plugins cannot redirect resolution, so each React specifier is a
// virtual module that re-exports its Preact counterpart; that also covers
// imports from inside dependencies.
const aliasModules = await Promise.all(
  preactAliases.map(async ([specifier, target]) => [specifier, await import(target)] as const),
)
plugin({
  name: 'preact-aliases',
  setup(build) {
    for (const [specifier, mod] of aliasModules) {
      build.module(specifier, () => ({ exports: { ...mod }, loader: 'object' }))
    }
  },
})

const window = new GlobalWindow({ url: 'https://comments.example.test' })
Object.defineProperty(window.navigator, 'platform', {
  value: process.env['EDITOR_TEST_PLATFORM'] ?? 'Win32',
})

for (const key of Object.getOwnPropertyNames(window)) {
  if (!(key in globalThis)) {
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value: Reflect.get(window, key),
    })
  }
}

for (const [key, value] of Object.entries({
  window,
  document: window.document,
  navigator: window.navigator,
  getComputedStyle: window.getComputedStyle.bind(window),
  requestAnimationFrame: window.requestAnimationFrame.bind(window),
  cancelAnimationFrame: window.cancelAnimationFrame.bind(window),
  IS_REACT_ACT_ENVIRONMENT: true,
})) {
  Object.defineProperty(globalThis, key, { configurable: true, writable: true, value })
}
