import { afterEach, expect, test } from 'bun:test'
import { type ChainedCommands, Editor } from '@tiptap/core'
import { createCommentExtensions, isCommentLink } from '../src/editor/extensions'

const editors: Editor[] = []

function createEditor(content = '<p>Sample text</p>') {
  const editor = new Editor({
    extensions: createCommentExtensions({ placeholder: 'Comment', openLink: () => {} }),
    content,
  })
  editors.push(editor)
  return editor
}

function renderedHTML(editor: Editor) {
  const container = document.createElement('div')
  container.innerHTML = editor.getHTML()
  return container
}

afterEach(() => {
  for (const editor of editors.splice(0)) editor.destroy()
})

const formats = [
  { name: 'bold', selector: 'strong', apply: (chain: ChainedCommands) => chain.toggleBold() },
  { name: 'italic', selector: 'em', apply: (chain: ChainedCommands) => chain.toggleItalic() },
  { name: 'underline', selector: 'u', apply: (chain: ChainedCommands) => chain.toggleUnderline() },
  { name: 'strikethrough', selector: 's', apply: (chain: ChainedCommands) => chain.toggleStrike() },
  {
    name: 'highlight',
    selector: 'mark',
    apply: (chain: ChainedCommands) => chain.toggleHighlight(),
  },
  {
    name: 'subscript',
    selector: 'sub',
    apply: (chain: ChainedCommands) => chain.toggleSubscript(),
  },
  {
    name: 'superscript',
    selector: 'sup',
    apply: (chain: ChainedCommands) => chain.toggleSuperscript(),
  },
  { name: 'inline code', selector: 'code', apply: (chain: ChainedCommands) => chain.toggleCode() },
  {
    name: 'bullet list',
    selector: 'ul > li > p',
    apply: (chain: ChainedCommands) => chain.toggleBulletList(),
  },
  {
    name: 'ordered list',
    selector: 'ol > li > p',
    apply: (chain: ChainedCommands) => chain.toggleOrderedList(),
  },
  {
    name: 'blockquote',
    selector: 'blockquote > p',
    apply: (chain: ChainedCommands) => chain.toggleBlockquote(),
  },
  {
    name: 'code block',
    selector: 'pre > code',
    apply: (chain: ChainedCommands) => chain.toggleCodeBlock(),
  },
] as const

test.each([...formats])(
  'serializes $name when the formatting command runs',
  ({ selector, apply }) => {
    // Given: selected comment text using the production extension configuration.
    const editor = createEditor()
    editor.commands.selectAll()

    // When: the formatting control's command is applied.
    expect(apply(editor.chain()).run()).toBe(true)

    // Then: persisted HTML retains the requested semantic formatting.
    expect(renderedHTML(editor).querySelector(selector)?.textContent).toBe('Sample text')
  },
)

test.each([1, 2, 3, 4, 5, 6] as const)('serializes heading level %i when selected', (level) => {
  // Given: a paragraph in a comment.
  const editor = createEditor()

  // When: a heading level is applied.
  editor.commands.setHeading({ level })

  // Then: saved HTML uses the chosen heading element.
  expect(renderedHTML(editor).querySelector(`h${level}`)?.textContent).toBe('Sample text')
})

test.each(['left', 'center', 'right', 'justify'])(
  'serializes %s alignment when applied',
  (alignment) => {
    // Given: a comment paragraph.
    const editor = createEditor()

    // When: paragraph alignment is changed.
    editor.commands.setTextAlign(alignment)

    // Then: the HTML retains paragraph alignment.
    expect(renderedHTML(editor).querySelector('p')?.style.textAlign).toBe(alignment)
  },
)

test('serializes a horizontal rule when inserted', () => {
  // Given: a comment with text.
  const editor = createEditor()

  // When: a rule is inserted.
  editor.commands.setHorizontalRule()

  // Then: saved HTML includes the rule regardless of trailing paragraphs.
  expect(renderedHTML(editor).querySelectorAll('hr').length).toBe(1)
})

const rejectedLinks = [
  'javascript:alert(1)',
  'data:text/html,hello',
  'mailto:test@example.com',
  'ftp://example.com',
  'tel:123',
  '//example.com',
  '/relative',
  'example.com',
  '',
] as const

test.each([...rejectedLinks])('rejects unsupported link %s when applying a link', (href) => {
  // Given: selected text and an unsupported or incomplete URL.
  const editor = createEditor()
  editor.commands.selectAll()

  // When: a caller tries to apply the link.
  const applied = editor.commands.setLink({ href })

  // Then: both the shared UI rule and Tiptap reject it without serializing an anchor.
  expect(isCommentLink(href)).toBe(false)
  expect(applied).toBe(false)
  expect(renderedHTML(editor).querySelector('a')).toBeNull()
})

test.each(['https://example.com/path?q=1#section', 'http://example.com'])(
  'retains supported link %s when applied',
  (href) => {
    // Given: selected text and a complete HTTP(S) URL.
    const editor = createEditor()
    editor.commands.selectAll()

    // When: the link is applied.
    const applied = editor.commands.setLink({ href })

    // Then: the UI rule permits it and saved HTML contains the intended target.
    expect(isCommentLink(href)).toBe(true)
    expect(applied).toBe(true)
    expect(renderedHTML(editor).querySelector('a')?.getAttribute('href')).toBe(href)
  },
)
