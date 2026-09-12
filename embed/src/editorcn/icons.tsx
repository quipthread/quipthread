import {
  AlignCenter,
  AlignJustify,
  AlignLeft,
  AlignRight,
  Bold,
  Code,
  Heading1,
  Heading2,
  Heading3,
  Heading4,
  Heading5,
  Heading6,
  Highlighter,
  Italic,
  Link2Off,
  List,
  ListOrdered,
  Minus,
  Redo2,
  RemoveFormatting,
  SquareCode,
  Strikethrough,
  Subscript,
  Superscript,
  TextQuote,
  Underline,
  Undo2,
} from 'lucide-react'
import type { ReactNode } from 'react'

export interface RichTextEditorIcons {
  readonly boldControlIcon: ReactNode
  readonly italicControlIcon: ReactNode
  readonly underlineControlIcon: ReactNode
  readonly strikeControlIcon: ReactNode
  readonly clearFormattingControlIcon: ReactNode
  readonly codeControlIcon: ReactNode
  readonly codeBlockControlIcon: ReactNode
  readonly h1ControlIcon: ReactNode
  readonly h2ControlIcon: ReactNode
  readonly h3ControlIcon: ReactNode
  readonly h4ControlIcon: ReactNode
  readonly h5ControlIcon: ReactNode
  readonly h6ControlIcon: ReactNode
  readonly bulletListControlIcon: ReactNode
  readonly orderedListControlIcon: ReactNode
  readonly blockquoteControlIcon: ReactNode
  readonly hrControlIcon: ReactNode
  readonly unlinkControlIcon: ReactNode
  readonly undoControlIcon: ReactNode
  readonly redoControlIcon: ReactNode
  readonly alignLeftControlIcon: ReactNode
  readonly alignCenterControlIcon: ReactNode
  readonly alignRightControlIcon: ReactNode
  readonly alignJustifyControlIcon: ReactNode
  readonly highlightControlIcon: ReactNode
  readonly subscriptControlIcon: ReactNode
  readonly superscriptControlIcon: ReactNode
}

const iconProps = { className: 'rte-editor-icon', 'aria-hidden': true } as const

export const DEFAULT_ICONS: RichTextEditorIcons = {
  alignCenterControlIcon: <AlignCenter {...iconProps} />,
  alignJustifyControlIcon: <AlignJustify {...iconProps} />,
  alignLeftControlIcon: <AlignLeft {...iconProps} />,
  alignRightControlIcon: <AlignRight {...iconProps} />,
  blockquoteControlIcon: <TextQuote {...iconProps} />,
  boldControlIcon: <Bold {...iconProps} />,
  bulletListControlIcon: <List {...iconProps} />,
  clearFormattingControlIcon: <RemoveFormatting {...iconProps} />,
  codeControlIcon: <Code {...iconProps} />,
  codeBlockControlIcon: <SquareCode {...iconProps} />,
  h1ControlIcon: <Heading1 {...iconProps} />,
  h2ControlIcon: <Heading2 {...iconProps} />,
  h3ControlIcon: <Heading3 {...iconProps} />,
  h4ControlIcon: <Heading4 {...iconProps} />,
  h5ControlIcon: <Heading5 {...iconProps} />,
  h6ControlIcon: <Heading6 {...iconProps} />,
  highlightControlIcon: <Highlighter {...iconProps} />,
  hrControlIcon: <Minus {...iconProps} />,
  italicControlIcon: <Italic {...iconProps} />,
  orderedListControlIcon: <ListOrdered {...iconProps} />,
  redoControlIcon: <Redo2 {...iconProps} />,
  strikeControlIcon: <Strikethrough {...iconProps} />,
  subscriptControlIcon: <Subscript {...iconProps} />,
  superscriptControlIcon: <Superscript {...iconProps} />,
  underlineControlIcon: <Underline {...iconProps} />,
  undoControlIcon: <Undo2 {...iconProps} />,
  unlinkControlIcon: <Link2Off {...iconProps} />,
}
