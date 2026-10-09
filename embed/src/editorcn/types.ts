import type { Editor } from '@tiptap/react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import type { RichTextEditorIcons } from './icons'
import type { RichTextEditorLabels } from './labels'

export type RichTextEditorProps = {
  readonly editor: Editor | null
  readonly children: ReactNode
  readonly className?: string
  readonly labels?: Partial<RichTextEditorLabels>
  readonly icons?: Partial<RichTextEditorIcons>
  readonly editable?: boolean
}

export type RichTextEditorContentProps = {
  readonly children?: ReactNode
  readonly className?: string
}

export type RichTextEditorToolbarProps = RichTextEditorContentProps & {
  readonly sticky?: boolean
  readonly stickyOffset?: number | string
}

export type RichTextEditorControlProps = Readonly<ButtonHTMLAttributes<HTMLButtonElement>> & {
  readonly active?: boolean
  readonly interactive?: boolean
}
