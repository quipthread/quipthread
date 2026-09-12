import { GlobalWindow } from 'happy-dom'

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
