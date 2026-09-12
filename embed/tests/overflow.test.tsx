import { afterEach, beforeEach, expect, mock, test } from 'bun:test'
import { act, type ReactElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { Editor } from '../src/Editor'

const roots: Root[] = []
const containers: HTMLDivElement[] = []
const observers = new Set<ToolbarResizeObserver>()
const originalObserver = Object.getOwnPropertyDescriptor(globalThis, 'ResizeObserver')
const originalWindowObserver = Object.getOwnPropertyDescriptor(window, 'ResizeObserver')
const originalWidth = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth')
const originalComputedStyle = globalThis.getComputedStyle
let toolbarWidth = 758
let style: HTMLStyleElement

class ToolbarResizeObserver implements ResizeObserver {
  readonly targets = new Set<Element>()

  constructor(readonly callback: ResizeObserverCallback) {
    observers.add(this)
  }

  observe(target: Element) {
    this.targets.add(target)
  }

  unobserve(target: Element) {
    this.targets.delete(target)
  }

  disconnect() {
    this.targets.clear()
    observers.delete(this)
  }
}

function restoreProperty(target: object, key: string, descriptor: PropertyDescriptor | undefined) {
  if (descriptor) Object.defineProperty(target, key, descriptor)
  else Reflect.deleteProperty(target, key)
}

function requireElement<T extends Element>(element: T | null): T {
  if (!element) throw new TypeError('Expected rendered toolbar element')
  return element
}

async function mount(component: ReactElement) {
  const container = document.createElement('div')
  container.className = 'qt-root'
  document.body.append(container)
  containers.push(container)
  const root = createRoot(container)
  roots.push(root)
  await act(async () => root.render(component))
  return container
}

async function resize(width: number) {
  await act(async () => {
    toolbarWidth = width
    for (const observer of observers) {
      if ([...observer.targets].some((target) => target.matches('.rte-toolbar'))) {
        observer.callback([], observer)
      }
    }
  })
}

function formattingLabels(container: Element) {
  return [...container.querySelectorAll<HTMLButtonElement>('.rte-control-button')]
    .map((button) => button.getAttribute('aria-label'))
    .filter((label) => label !== 'More formatting')
    .sort()
}

beforeEach(async () => {
  toolbarWidth = 758
  style = document.createElement('style')
  style.textContent = await Bun.file(new URL('../src/editor/composer.css', import.meta.url)).text()
  document.head.append(style)
  globalThis.getComputedStyle = (element, pseudoElement) => {
    const computed = originalComputedStyle(element, pseudoElement)
    if (!element.matches('.rte-toolbar')) return computed
    const resolved = document.createElement('span').style
    resolved.paddingLeft = computed.paddingLeft
    resolved.paddingRight = computed.paddingRight
    resolved.columnGap = computed.columnGap || computed.gap
    const composer = requireElement(element.closest('.qt-composer'))
    const inherited = originalComputedStyle(composer)
    for (const name of ['--qt-editor-control', '--qt-editor-divider']) {
      resolved.setProperty(
        name,
        computed.getPropertyValue(name) || inherited.getPropertyValue(name),
      )
    }
    return resolved
  }
  for (const target of [globalThis, window]) {
    Object.defineProperty(target, 'ResizeObserver', {
      configurable: true,
      writable: true,
      value: ToolbarResizeObserver,
    })
  }
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.matches('.rte-toolbar') ? toolbarWidth : 0
    },
  })
})

afterEach(async () => {
  await act(async () => {
    for (const root of roots.splice(0)) root.unmount()
  })
  for (const container of containers.splice(0)) container.remove()
  style.remove()
  observers.clear()
  restoreProperty(globalThis, 'ResizeObserver', originalObserver)
  restoreProperty(window, 'ResizeObserver', originalWindowObserver)
  restoreProperty(HTMLElement.prototype, 'clientWidth', originalWidth)
  globalThis.getComputedStyle = originalComputedStyle
})

test('keeps every format reachable while the mounted toolbar shrinks and expands', async () => {
  // Given: the desktop toolbar exposes all 28 controls inline.
  const container = await mount(<Editor />)
  const toolbar = requireElement(container.querySelector('fieldset'))
  const labels = formattingLabels(toolbar)
  expect(labels).toHaveLength(28)
  expect(toolbar.querySelector('.qt-overflow-trigger')).toBeNull()

  // When: the host contracts through tablet and mobile widths, then expands again.
  for (const { width, inlineControls } of [
    { width: 726, inlineControls: 26 },
    { width: 333, inlineControls: 6 },
  ]) {
    await resize(width)
    const more = requireElement(
      toolbar.querySelector<HTMLButtonElement>('[aria-label="More formatting"]'),
    )
    expect(formattingLabels(toolbar)).toHaveLength(inlineControls)
    expect(toolbar.lastElementChild).toBe(more)
    await act(async () => more.click())

    // Then: every original format remains reachable exactly once in the toolbar or menu.
    expect(more.getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelector('.qt-overflow-popover')).not.toBeNull()
    expect(formattingLabels(container)).toEqual(labels)
    await act(async () => more.click())
  }
  await resize(758)
  expect(toolbar.querySelector('.qt-overflow-trigger')).toBeNull()
  expect(container.querySelector('.qt-overflow-popover')).toBeNull()
  expect(formattingLabels(toolbar)).toEqual(labels)
})

test.each([0, 1])(
  'anchors the hidden-link shortcut to the overflow button in editor %i',
  async (owner) => {
    // Given: two narrow editors keep their link controls inside the closed overflow menu.
    toolbarWidth = 333
    const container = await mount(
      <>
        <Editor />
        <Editor />
      </>,
    )
    const composers = container.querySelectorAll<HTMLElement>('.qt-composer')
    const owningComposer = requireElement(composers.item(owner))
    const otherComposer = requireElement(composers.item(1 - owner))
    expect(container.querySelector('[aria-label="Add or edit link"]') === null).toBe(true)
    const more = requireElement(
      owningComposer.querySelector<HTMLButtonElement>('.qt-overflow-trigger'),
    )
    const bounds = mock(() => new DOMRect(280, 20, 24, 24))
    more.getBoundingClientRect = bounds
    const textbox = requireElement(owningComposer.querySelector<HTMLElement>('[role="textbox"]'))

    // When: the owning editor receives its platform's Mod-K shortcut.
    await act(async () => {
      textbox.focus()
      textbox.dispatchEvent(
        new window.KeyboardEvent('keydown', {
          key: 'k',
          ctrlKey: !navigator.platform.includes('Mac'),
          metaKey: navigator.platform.includes('Mac'),
          bubbles: true,
          cancelable: true,
        }),
      )
    })

    // Then: only that editor opens a link form, positioned using its overflow trigger.
    expect(owningComposer.querySelector('input[type="url"]')).not.toBeNull()
    expect(otherComposer.querySelector('input[type="url"]')).toBeNull()
    expect(bounds).toHaveBeenCalled()
    expect(owningComposer.querySelector('.qt-overflow-popover')).toBeNull()
  },
)
