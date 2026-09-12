import type {} from '@tiptap/extension-highlight'
import type {} from '@tiptap/extension-subscript'
import type {} from '@tiptap/extension-superscript'
import type {} from '@tiptap/extension-text-align'
import { createControl } from './rte-control'

export const BoldControl = createControl({
  iconKey: 'boldControlIcon',
  isActive: (editor) => editor.isActive('bold'),
  label: 'boldControlLabel',
  operation: (chain) => chain.toggleBold(),
})

export const ItalicControl = createControl({
  iconKey: 'italicControlIcon',
  isActive: (editor) => editor.isActive('italic'),
  label: 'italicControlLabel',
  operation: (chain) => chain.toggleItalic(),
})

export const UnderlineControl = createControl({
  iconKey: 'underlineControlIcon',
  isActive: (editor) => editor.isActive('underline'),
  label: 'underlineControlLabel',
  operation: (chain) => chain.toggleUnderline(),
})

export const StrikethroughControl = createControl({
  iconKey: 'strikeControlIcon',
  isActive: (editor) => editor.isActive('strike'),
  label: 'strikeControlLabel',
  operation: (chain) => chain.toggleStrike(),
})

export const ClearFormattingControl = createControl({
  iconKey: 'clearFormattingControlIcon',
  label: 'clearFormattingControlLabel',
  operation: (chain) => chain.unsetAllMarks().clearNodes().unsetTextAlign(),
})

export const CodeControl = createControl({
  iconKey: 'codeControlIcon',
  isActive: (editor) => editor.isActive('code'),
  label: 'codeControlLabel',
  operation: (chain) => chain.toggleCode(),
})

export const CodeBlockControl = createControl({
  iconKey: 'codeBlockControlIcon',
  isActive: (editor) => editor.isActive('codeBlock'),
  label: 'codeBlockControlLabel',
  operation: (chain) => chain.toggleCodeBlock(),
})

export const H1Control = createControl({
  iconKey: 'h1ControlIcon',
  isActive: (editor) => editor.isActive('heading', { level: 1 }),
  label: 'h1ControlLabel',
  operation: (chain) => chain.toggleHeading({ level: 1 }),
})

export const H2Control = createControl({
  iconKey: 'h2ControlIcon',
  isActive: (editor) => editor.isActive('heading', { level: 2 }),
  label: 'h2ControlLabel',
  operation: (chain) => chain.toggleHeading({ level: 2 }),
})

export const H3Control = createControl({
  iconKey: 'h3ControlIcon',
  isActive: (editor) => editor.isActive('heading', { level: 3 }),
  label: 'h3ControlLabel',
  operation: (chain) => chain.toggleHeading({ level: 3 }),
})

export const H4Control = createControl({
  iconKey: 'h4ControlIcon',
  isActive: (editor) => editor.isActive('heading', { level: 4 }),
  label: 'h4ControlLabel',
  operation: (chain) => chain.toggleHeading({ level: 4 }),
})

export const BulletListControl = createControl({
  iconKey: 'bulletListControlIcon',
  isActive: (editor) => editor.isActive('bulletList'),
  label: 'bulletListControlLabel',
  operation: (chain) => chain.toggleBulletList(),
})

export const OrderedListControl = createControl({
  iconKey: 'orderedListControlIcon',
  isActive: (editor) => editor.isActive('orderedList'),
  label: 'orderedListControlLabel',
  operation: (chain) => chain.toggleOrderedList(),
})

export const BlockquoteControl = createControl({
  iconKey: 'blockquoteControlIcon',
  isActive: (editor) => editor.isActive('blockquote'),
  label: 'blockquoteControlLabel',
  operation: (chain) => chain.toggleBlockquote(),
})

export const HrControl = createControl({
  iconKey: 'hrControlIcon',
  label: 'hrControlLabel',
  operation: (chain) => chain.setHorizontalRule(),
})

export const UnlinkControl = createControl({
  iconKey: 'unlinkControlIcon',
  label: 'unlinkControlLabel',
  isActive: (editor) => editor.isActive('link'),
  operation: (chain) => chain.extendMarkRange('link').unsetLink(),
})

export const UndoControl = createControl({
  iconKey: 'undoControlIcon',
  isDisabled: (editor) => !editor.can().undo(),
  label: 'undoControlLabel',
  operation: (chain) => chain.undo(),
})

export const RedoControl = createControl({
  iconKey: 'redoControlIcon',
  isDisabled: (editor) => !editor.can().redo(),
  label: 'redoControlLabel',
  operation: (chain) => chain.redo(),
})

export const H5Control = createControl({
  iconKey: 'h5ControlIcon',
  isActive: (editor) => editor.isActive('heading', { level: 5 }),
  label: 'h5ControlLabel',
  operation: (chain) => chain.toggleHeading({ level: 5 }),
})

export const H6Control = createControl({
  iconKey: 'h6ControlIcon',
  isActive: (editor) => editor.isActive('heading', { level: 6 }),
  label: 'h6ControlLabel',
  operation: (chain) => chain.toggleHeading({ level: 6 }),
})

export const AlignLeftControl = createControl({
  iconKey: 'alignLeftControlIcon',
  isActive: (editor) => editor.isActive({ textAlign: 'left' }),
  label: 'alignLeftControlLabel',
  operation: (chain) => chain.setTextAlign('left'),
})

export const AlignCenterControl = createControl({
  iconKey: 'alignCenterControlIcon',
  isActive: (editor) => editor.isActive({ textAlign: 'center' }),
  label: 'alignCenterControlLabel',
  operation: (chain) => chain.setTextAlign('center'),
})

export const AlignRightControl = createControl({
  iconKey: 'alignRightControlIcon',
  isActive: (editor) => editor.isActive({ textAlign: 'right' }),
  label: 'alignRightControlLabel',
  operation: (chain) => chain.setTextAlign('right'),
})

export const AlignJustifyControl = createControl({
  iconKey: 'alignJustifyControlIcon',
  isActive: (editor) => editor.isActive({ textAlign: 'justify' }),
  label: 'alignJustifyControlLabel',
  operation: (chain) => chain.setTextAlign('justify'),
})

export const HighlightControl = createControl({
  iconKey: 'highlightControlIcon',
  isActive: (editor) => editor.isActive('highlight'),
  label: 'highlightControlLabel',
  operation: (chain) => chain.toggleHighlight(),
})

export const SubscriptControl = createControl({
  iconKey: 'subscriptControlIcon',
  isActive: (editor) => editor.isActive('subscript'),
  label: 'subscriptControlLabel',
  operation: (chain) => chain.toggleSubscript(),
})

export const SuperscriptControl = createControl({
  iconKey: 'superscriptControlIcon',
  isActive: (editor) => editor.isActive('superscript'),
  label: 'superscriptControlLabel',
  operation: (chain) => chain.toggleSuperscript(),
})
