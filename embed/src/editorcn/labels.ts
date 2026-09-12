export interface RichTextEditorLabels {
  readonly unlinkControlLabel: string
  readonly boldControlLabel: string
  readonly italicControlLabel: string
  readonly underlineControlLabel: string
  readonly strikeControlLabel: string
  readonly clearFormattingControlLabel: string
  readonly codeControlLabel: string
  readonly codeBlockControlLabel: string
  readonly h1ControlLabel: string
  readonly h2ControlLabel: string
  readonly h3ControlLabel: string
  readonly h4ControlLabel: string
  readonly h5ControlLabel: string
  readonly h6ControlLabel: string
  readonly bulletListControlLabel: string
  readonly orderedListControlLabel: string
  readonly blockquoteControlLabel: string
  readonly hrControlLabel: string
  readonly undoControlLabel: string
  readonly redoControlLabel: string
  readonly alignLeftControlLabel: string
  readonly alignCenterControlLabel: string
  readonly alignRightControlLabel: string
  readonly alignJustifyControlLabel: string
  readonly highlightControlLabel: string
  readonly subscriptControlLabel: string
  readonly superscriptControlLabel: string
}

export const DEFAULT_LABELS: RichTextEditorLabels = {
  unlinkControlLabel: 'Remove link',
  alignCenterControlLabel: 'Align center',
  alignJustifyControlLabel: 'Align justify',
  alignLeftControlLabel: 'Align left',
  alignRightControlLabel: 'Align right',
  blockquoteControlLabel: 'Blockquote',
  boldControlLabel: 'Bold',
  bulletListControlLabel: 'Bullet list',
  clearFormattingControlLabel: 'Clear formatting',
  codeControlLabel: 'Code',
  codeBlockControlLabel: 'Code block',
  h1ControlLabel: 'Heading 1',
  h2ControlLabel: 'Heading 2',
  h3ControlLabel: 'Heading 3',
  h4ControlLabel: 'Heading 4',
  h5ControlLabel: 'Heading 5',
  h6ControlLabel: 'Heading 6',
  highlightControlLabel: 'Highlight',
  hrControlLabel: 'Horizontal rule',
  italicControlLabel: 'Italic',
  orderedListControlLabel: 'Ordered list',
  redoControlLabel: 'Redo',
  strikeControlLabel: 'Strikethrough',
  subscriptControlLabel: 'Subscript',
  superscriptControlLabel: 'Superscript',
  underlineControlLabel: 'Underline',
  undoControlLabel: 'Undo',
}
