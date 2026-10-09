import { useEditor } from '@tiptap/react'
import { forwardRef, useId, useImperativeHandle, useState } from 'react'
import composerCSS from './editor/composer.css?inline'
import { createCommentExtensions } from './editor/extensions'
import { EditorToolbar } from './editor/Toolbar'
import { RichTextEditor } from './editorcn'

// The editor's styles ship with its chunk, injected the first time it loads.
let stylesInjected = false
function injectComposerStyles(): void {
  if (stylesInjected || typeof document === 'undefined') return
  stylesInjected = true
  const style = document.createElement('style')
  style.textContent = composerCSS
  document.head.appendChild(style)
}

export interface EditorRef {
  getHTML: () => string
  isEmpty: () => boolean
  clear: () => void
}

export interface EditorProps {
  readonly placeholder?: string
  readonly initialContent?: string
  readonly onChange?: (html: string, isEmpty: boolean) => void
  readonly disabled?: boolean
  /** Focus the editor once it mounts (the reader already clicked into it). */
  readonly autoFocus?: boolean
}

export const Editor = forwardRef<EditorRef, EditorProps>(
  (
    {
      placeholder = 'Write a comment…',
      initialContent = '',
      onChange,
      disabled = false,
      autoFocus = false,
    },
    ref,
  ) => {
    injectComposerStyles()
    const labelId = useId()
    const [linkOpen, setLinkOpen] = useState(false)
    const editor = useEditor({
      immediatelyRender: false,
      autofocus: autoFocus ? 'end' : false,
      shouldRerenderOnTransaction: false,
      extensions: createCommentExtensions({ placeholder, openLink: () => setLinkOpen(true) }),
      content: initialContent,
      editorProps: {
        attributes: { role: 'textbox', 'aria-multiline': 'true', 'aria-labelledby': labelId },
      },
      onUpdate: ({ editor }) => onChange?.(editor.getHTML(), editor.isEmpty),
    })

    useImperativeHandle(ref, () => ({
      getHTML: () => editor?.getHTML() ?? '',
      isEmpty: () => editor?.isEmpty ?? true,
      clear: () => editor?.commands.clearContent(true),
    }))

    if (!editor) return null

    return (
      <RichTextEditor editor={editor} editable={!disabled} className="qt-composer">
        <span id={labelId} className="qt-editor-label">
          {placeholder}
        </span>
        <EditorToolbar linkOpen={linkOpen} onLinkOpenChange={setLinkOpen} />
        <RichTextEditor.Content className="qt-rich-text" />
      </RichTextEditor>
    )
  },
)

Editor.displayName = 'Editor'
