import type { RichTextEditorContentProps } from './types'

export const ControlsGroup = ({ children, className = '' }: RichTextEditorContentProps) => (
  <div className={`rte-controls-group ${className}`}>{children}</div>
)
