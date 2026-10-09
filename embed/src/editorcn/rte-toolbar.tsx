import { forwardRef } from 'react'
import { useRichTextEditorContext } from './rte-context'
import type { RichTextEditorToolbarProps } from './types'

// forwardRef rather than a ref prop: the widget runs on Preact, which only
// passes refs to function components through forwardRef.
export const Toolbar = forwardRef<HTMLFieldSetElement, RichTextEditorToolbarProps>(
  ({ children, className = '', sticky = false, stickyOffset = 0 }, ref) => {
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
  },
)
