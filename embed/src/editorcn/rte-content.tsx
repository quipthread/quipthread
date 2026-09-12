import { EditorContent } from '@tiptap/react'
import { useRichTextEditorContext } from './rte-context'
import type { RichTextEditorContentProps } from './types'

export const Content = ({ children, className = '' }: RichTextEditorContentProps) => {
  const { editor } = useRichTextEditorContext()
  return (
    <div className="rte-content-wrapper">
      <EditorContent editor={editor} className={`rte-content ${className}`} />
      {children}
    </div>
  )
}
