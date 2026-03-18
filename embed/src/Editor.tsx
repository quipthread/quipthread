import { useEditor, EditorContent } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Placeholder from '@tiptap/extension-placeholder'
import { useEffect, useImperativeHandle, forwardRef } from 'react'

export interface EditorRef {
  getHTML: () => string
  isEmpty: () => boolean
  clear: () => void
}

interface EditorProps {
  placeholder?: string
  initialContent?: string
  onChange?: (html: string, isEmpty: boolean) => void
}

export const Editor = forwardRef<EditorRef, EditorProps>(
  ({ placeholder = 'Write a comment…', initialContent = '', onChange }, ref) => {
    const editor = useEditor({
      immediatelyRender: false,
      shouldRerenderOnTransaction: true,
      extensions: [
        StarterKit.configure({ link: false }),
        Link.configure({
          openOnClick: false,
          validate: (href) => /^https?:\/\//.test(href),
        }),
        Placeholder.configure({ placeholder }),
      ],
      content: initialContent,
      onUpdate: ({ editor }) => {
        onChange?.(editor.getHTML(), editor.isEmpty)
      },
    })

    useImperativeHandle(ref, () => ({
      getHTML: () => editor?.getHTML() ?? '',
      isEmpty: () => editor?.isEmpty ?? true,
      clear: () => editor?.commands.clearContent(true),
    }))

    useEffect(() => {
      return () => {
        editor?.destroy()
      }
    }, [editor])

    if (!editor) return null

    const ToolbarBtn = ({
      onClick,
      active,
      title,
      children,
    }: {
      onClick: () => void
      active?: boolean
      title: string
      children: React.ReactNode
    }) => (
      <button
        type="button"
        className={`qt-toolbar-btn${active ? ' is-active' : ''}`}
        title={title}
        onClick={onClick}
        onMouseDown={(e) => e.preventDefault()}
      >
        {children}
      </button>
    )

    const handleAddLink = () => {
      const prev = editor.isActive('link') ? editor.getAttributes('link').href as string : ''
      const url = window.prompt('Link URL:', prev)
      if (url === null) return
      if (url === '') {
        editor.chain().focus().unsetLink().run()
      } else {
        editor.chain().focus().setLink({ href: url }).run()
      }
    }

    return (
      <div className="qt-editor-wrapper">
        <div className="qt-toolbar">
          <ToolbarBtn
            onClick={() => editor.chain().focus().toggleBold().run()}
            active={editor.isActive('bold')}
            title="Bold"
          >
            <strong>B</strong>
          </ToolbarBtn>
          <ToolbarBtn
            onClick={() => editor.chain().focus().toggleItalic().run()}
            active={editor.isActive('italic')}
            title="Italic"
          >
            <em>I</em>
          </ToolbarBtn>
          <ToolbarBtn
            onClick={() => editor.chain().focus().toggleStrike().run()}
            active={editor.isActive('strike')}
            title="Strikethrough"
          >
            <s>S</s>
          </ToolbarBtn>
          <div className="qt-toolbar-separator" />
          <ToolbarBtn
            onClick={() => editor.chain().focus().toggleBulletList().run()}
            active={editor.isActive('bulletList')}
            title="Bullet list"
          >
            ≡
          </ToolbarBtn>
          <ToolbarBtn
            onClick={() => editor.chain().focus().toggleOrderedList().run()}
            active={editor.isActive('orderedList')}
            title="Numbered list"
          >
            №
          </ToolbarBtn>
          <div className="qt-toolbar-separator" />
          <ToolbarBtn
            onClick={() => editor.chain().focus().toggleCode().run()}
            active={editor.isActive('code')}
            title="Inline code"
          >
            {'<>'}
          </ToolbarBtn>
          <ToolbarBtn
            onClick={handleAddLink}
            active={editor.isActive('link')}
            title="Add link"
          >
            ↗
          </ToolbarBtn>
        </div>
        <div className="qt-editor-content">
          <EditorContent editor={editor} />
        </div>
      </div>
    )
  },
)

Editor.displayName = 'Editor'
