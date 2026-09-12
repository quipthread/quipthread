import { useRichTextEditorContext } from './rte-context'
import type { RichTextEditorToolbarProps } from './types'

export const Toolbar = ({
  children,
  ref,
  className = '',
  sticky = false,
  stickyOffset = 0,
}: RichTextEditorToolbarProps) => {
  const { editable } = useRichTextEditorContext()
  return (
    <fieldset
      ref={ref}
      disabled={!editable}
      className={`rte-toolbar ${className}`}
      aria-label="Text formatting"
      data-sticky={sticky ? '' : undefined}
      style={sticky ? { position: 'sticky', top: stickyOffset, zIndex: 1 } : undefined}
    >
      {children}
    </fieldset>
  )
}
