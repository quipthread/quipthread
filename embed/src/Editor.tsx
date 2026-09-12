import { useEditor } from '@tiptap/react'
import { forwardRef, useId, useImperativeHandle, useState } from 'react'
import { createCommentExtensions } from './editor/extensions'
import { EditorToolbar } from './editor/Toolbar'
import { RichTextEditor } from './editorcn'

export interface EditorRef {
  getHTML: () => string
  isEmpty: () => boolean
  clear: () => void
}

interface EditorProps {
  readonly placeholder?: string
  readonly initialContent?: string
  readonly onChange?: (html: string, isEmpty: boolean) => void
  readonly disabled?: boolean
}

export const Editor = forwardRef<EditorRef, EditorProps>(
  ({ placeholder = 'Write a comment…', initialContent = '', onChange, disabled = false }, ref) => {
    const labelId = useId()
    const [linkOpen, setLinkOpen] = useState(false)
    const editor = useEditor({
      immediatelyRender: false,
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
