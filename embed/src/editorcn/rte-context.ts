import type { Editor } from '@tiptap/react'
import { createContext, useContext } from 'react'
import type { RichTextEditorIcons } from './icons'
import type { RichTextEditorLabels } from './labels'

type RichTextEditorContextValue = {
  readonly editor: Editor | null
  readonly labels: RichTextEditorLabels
  readonly icons: RichTextEditorIcons
  readonly editable: boolean
}

export const RichTextEditorContext = createContext<RichTextEditorContextValue | null>(null)

class MissingRichTextEditorProviderError extends Error {
  constructor() {
    super('useRichTextEditorContext must be used within RichTextEditor')
    this.name = 'MissingRichTextEditorProviderError'
  }
}

export const useRichTextEditorContext = (): RichTextEditorContextValue => {
  const context = useContext(RichTextEditorContext)
  if (!context) throw new MissingRichTextEditorProviderError()
  return context
}
