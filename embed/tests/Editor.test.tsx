import { afterEach, expect, test } from 'bun:test'
import { Editor as TiptapEditor } from '@tiptap/core'
import { AllSelection } from '@tiptap/pm/state'
import { act, createRef, type ReactElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { CommentForm } from '../src/CommentForm'
import { Editor, type EditorRef } from '../src/Editor'
import { createCommentExtensions } from '../src/editor/extensions'
import { RichTextEditor as RTE } from '../src/editorcn'
import { useTranslations } from '../src/i18n'

const roots: Root[] = []
const editors: TiptapEditor[] = []

async function mount(component: ReactElement) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  roots.push(root)
  await act(async () => root.render(component))
  return container
}

function requireElement<T extends Element>(element: T | null): T {
  if (!element) throw new TypeError('Expected rendered editor element')
  return element
}

function requireRef(ref: { readonly current: EditorRef | null }): EditorRef {
  if (!ref.current) throw new TypeError('Expected mounted editor ref')
  return ref.current
}

afterEach(async () => {
  await act(async () => {
    for (const root of roots.splice(0)) root.unmount()
    for (const editor of editors.splice(0)) editor.destroy()
  })
  document.body.replaceChildren()
  localStorage.clear()
})

const blockControls = [
  { label: 'Bullet list', node: 'ul', Control: RTE.BulletList },
  { label: 'Ordered list', node: 'ol', Control: RTE.OrderedList },
  { label: 'Blockquote', node: 'blockquote', Control: RTE.Blockquote },
  { label: 'Heading 1', node: 'h1', Control: RTE.H1 },
  { label: 'Heading 2', node: 'h2', Control: RTE.H2 },
  { label: 'Heading 3', node: 'h3', Control: RTE.H3 },
  { label: 'Heading 4', node: 'h4', Control: RTE.H4 },
  { label: 'Heading 5', node: 'h5', Control: RTE.H5 },
  { label: 'Heading 6', node: 'h6', Control: RTE.H6 },
] as const

test.each([...blockControls])(
  'restores paragraphs when $label is toggled twice after Select All',
  async ({ label, node, Control }) => {
    // Given: the browser Select All command selects the entire ProseMirror document.
    const editor = new TiptapEditor({
      extensions: createCommentExtensions({ placeholder: 'Comment', openLink: () => {} }),
      content: '<p>Selected text</p>',
    })
    editors.push(editor)
    const container = await mount(
      <RTE editor={editor}>
        <Control />
      </RTE>,
    )
    const button = requireElement(
      container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`),
    )
    editor.commands.selectAll()
    expect(editor.state.selection).toBeInstanceOf(AllSelection)

    // When: the user toggles a block format on and then off through its real control.
    await act(async () => button.click())
    expect(editor.getHTML()).toContain(`<${node}>`)
    await act(async () => {
      editor.commands.selectAll()
      button.click()
    })

    // Then: the original paragraph returns without extra empty blocks.
    const html = document.createElement('div')
    html.innerHTML = editor.getHTML()
    expect(html.querySelector(node)).toBeNull()
    expect(html.children.length).toBe(1)
    expect(html.firstElementChild?.tagName).toBe('P')
    expect(html.textContent).toBe('Selected text')
  },
)

test('restores edited reply HTML when a comment form closes and reopens', async () => {
  // Given: a saved reply is opened for further editing.
  localStorage.setItem('qt-draft:site:page:parent', '<p>Reply draft</p>')
  const form = (
    <CommentForm
      siteId="site"
      pageId="page"
      parentId="parent"
      t={useTranslations('en')}
      onSuccess={() => {}}
    />
  )
  const container = await mount(form)
  const textbox = requireElement(container.querySelector<HTMLElement>('[role="textbox"]'))
  const bold = requireElement(
    container.querySelector<HTMLButtonElement>('button[aria-label="Bold"]'),
  )

  // When: the user formats the draft, closes the form, and opens it again.
  await act(async () => {
    textbox.focus()
    textbox.dispatchEvent(
      new window.KeyboardEvent('keydown', {
        key: 'a',
        ctrlKey: !navigator.platform.includes('Mac'),
        metaKey: navigator.platform.includes('Mac'),
        bubbles: true,
        cancelable: true,
      }),
    )
    bold.click()
  })
  await act(async () => {
    for (const root of roots.splice(0)) root.unmount()
  })
  const reopened = await mount(form)

  // Then: the remounted editor preserves the latest formatting, not the original draft.
  expect(reopened.querySelector('[role="textbox"] strong')?.textContent).toBe('Reply draft')
})

test('renders an enabled toolbar when mounted before the first focus', async () => {
  // Given / When: a fresh editor mounts without focus or input.
  const container = await mount(<Editor />)

  // Then: initialization renders usable controls and an empty textbox.
  const textbox = requireElement(container.querySelector<HTMLElement>('[role="textbox"]'))
  expect(document.activeElement).not.toBe(textbox)
  expect(textbox.getAttribute('contenteditable')).toBe('true')
  for (const label of ['Bold', 'Italic', 'Underline', 'Add or edit link']) {
    const button = requireElement(
      container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`),
    )
    expect(button.disabled).toBe(false)
  }
  expect(container.querySelector('select')).toBeNull()
  expect(container.querySelectorAll('fieldset button').length).toBe(28)
})

test('exposes initial HTML through the mounted editor ref', async () => {
  // Given: initial content includes formatting and multiple blocks.
  const ref = createRef<EditorRef>()

  // When: the editor initializes with saved comment HTML.
  await mount(
    <Editor ref={ref} initialContent="<h2>Title</h2><p><strong>Saved</strong> text</p>" />,
  )

  // Then: consumers can read the persisted formatting and populated state.
  const html = document.createElement('div')
  html.innerHTML = requireRef(ref).getHTML()
  expect(html.querySelector('h2')?.textContent).toBe('Title')
  expect(html.querySelector('strong')?.textContent).toBe('Saved')
  expect(requireRef(ref).isEmpty()).toBe(false)
})

test('restores all primary controls without focus after clearing while disabled', async () => {
  // Given: a populated editor is mounted and then disabled for submission.
  const ref = createRef<EditorRef>()
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  roots.push(root)
  await act(async () =>
    root.render(<Editor ref={ref} initialContent="<p>Submitted draft</p>" disabled={false} />),
  )
  await act(async () => root.render(<Editor ref={ref} disabled />))
  expect(container.querySelector('[role="textbox"]')?.getAttribute('contenteditable')).toBe('false')

  // When: submission clears the disabled editor and then restores editing.
  await act(async () => requireRef(ref).clear())
  await act(async () => root.render(<Editor ref={ref} disabled={false} />))

  // Then: every primary control is immediately available without focusing the editor.
  const textbox = requireElement(container.querySelector<HTMLElement>('[role="textbox"]'))
  expect(document.activeElement).not.toBe(textbox)
  expect(textbox.getAttribute('contenteditable')).toBe('true')
  expect(requireRef(ref).isEmpty()).toBe(true)
  const controls = container.querySelectorAll<HTMLButtonElement>('fieldset button')
  expect(controls.length).toBe(28)
  for (const control of controls) {
    const label = control.getAttribute('aria-label')
    if (label === 'Undo' || label === 'Redo') continue
    expect({ label, disabled: control.disabled }).toEqual({ label, disabled: false })
  }
})

test('clears content and notifies consumers when the ref clear method runs', async () => {
  // Given: a populated editor with its public change callback.
  const ref = createRef<EditorRef>()
  const changes: { readonly html: string; readonly isEmpty: boolean }[] = []
  await mount(
    <Editor
      ref={ref}
      initialContent="<p>Draft</p>"
      onChange={(html, isEmpty) => changes.push({ html, isEmpty })}
    />,
  )

  // When: the comment submission flow clears the editor.
  await act(async () => requireRef(ref).clear())

  // Then: both the ref and callback report an empty draft.
  expect(requireRef(ref).isEmpty()).toBe(true)
  expect(changes.at(-1)?.isEmpty).toBe(true)
  expect(changes.at(-1)?.html).toBe(requireRef(ref).getHTML())
})

test.each([0, 1])(
  'opens a link popover only inside editor %i when its shortcut fires',
  async (owner) => {
    // Given: two independently mounted editors.
    const container = await mount(
      <>
        <Editor placeholder="First comment" />
        <Editor placeholder="Second comment" />
      </>,
    )
    const composers = container.querySelectorAll<HTMLElement>('.qt-composer')
    const owningComposer = requireElement(composers.item(owner))
    const otherComposer = requireElement(composers.item(1 - owner))
    const textbox = requireElement(owningComposer.querySelector<HTMLElement>('[role="textbox"]'))

    // When: the focused editor receives the platform's Mod-K shortcut.
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

    // Then: only that editor owns the link form.
    expect(owningComposer.querySelector('input[type="url"]')).not.toBeNull()
    expect(otherComposer.querySelector('input[type="url"]')).toBeNull()
  },
)
