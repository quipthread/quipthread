import { Popover } from '@base-ui/react/popover'
import { useEditorState } from '@tiptap/react'
import { Ellipsis, Link } from 'lucide-react'
import { useRef, useState } from 'react'
import { RichTextEditor as RTE, useRichTextEditorContext } from '../editorcn'
import { LinkPopover } from './LinkPopover'
import { useToolbarOverflow } from './useToolbarOverflow'

const groupSizes = [6, 6, 4, 4, 2, 3, 1, 2] as const

export function EditorToolbar({
  linkOpen,
  onLinkOpenChange,
}: {
  readonly linkOpen: boolean
  readonly onLinkOpenChange: (open: boolean) => void
}) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const linkTriggerRef = useRef<HTMLButtonElement>(null)
  const overflowTriggerRef = useRef<HTMLButtonElement>(null)
  const { editor, editable } = useRichTextEditorContext()
  const { toolbarRef, visibleGroups } = useToolbarOverflow(groupSizes)
  const [overflowOpen, setOverflowOpen] = useState(false)
  const linkState = useEditorState({
    editor,
    selector: ({ editor }) => editor?.isActive('link') ?? false,
  })
  const groups = [
    {
      label: 'Text',
      content: (
        <>
          <RTE.Bold />
          <RTE.Italic />
          <RTE.Underline />
          <RTE.Strikethrough />
          <RTE.Code />
          <RTE.ClearFormatting />
        </>
      ),
    },
    {
      label: 'Headings',
      content: (
        <>
          <RTE.H1 />
          <RTE.H2 />
          <RTE.H3 />
          <RTE.H4 />
          <RTE.H5 />
          <RTE.H6 />
        </>
      ),
    },
    {
      label: 'Lists and quotes',
      content: (
        <>
          <RTE.BulletList />
          <RTE.OrderedList />
          <RTE.Blockquote />
          <RTE.Hr />
        </>
      ),
    },
    {
      label: 'Alignment',
      content: (
        <>
          <RTE.AlignLeft />
          <RTE.AlignCenter />
          <RTE.AlignRight />
          <RTE.AlignJustify />
        </>
      ),
    },
    {
      label: 'Links',
      content: (
        <>
          <RTE.Control
            ref={linkTriggerRef}
            aria-label="Add or edit link"
            title="Add or edit link (Ctrl/Cmd+K)"
            active={linkState ?? false}
            disabled={!editable}
            aria-expanded={linkOpen}
            aria-haspopup="dialog"
            onClick={() => {
              setOverflowOpen(false)
              onLinkOpenChange(true)
            }}
          >
            <Link aria-hidden="true" className="rte-editor-icon" />
          </RTE.Control>
          <RTE.Unlink />
        </>
      ),
    },
    {
      label: 'More text styles',
      content: (
        <>
          <RTE.Highlight />
          <RTE.Subscript />
          <RTE.Superscript />
        </>
      ),
    },
    {
      label: 'Code block',
      content: (
        <>
          <RTE.CodeBlock />
        </>
      ),
    },
    {
      label: 'History',
      content: (
        <>
          <RTE.Undo />
          <RTE.Redo />
        </>
      ),
    },
  ]
  const hasOverflow = visibleGroups < groups.length
  return (
    <>
      <div ref={setContainer} />
      <LinkPopover
        open={linkOpen}
        onOpenChange={onLinkOpenChange}
        container={container}
        anchor={() => linkTriggerRef.current ?? overflowTriggerRef.current}
      />
      <RTE.Toolbar
        ref={toolbarRef}
        className={`rte-toolbar--compact ${hasOverflow ? 'qt-toolbar-overflowing' : ''}`}
      >
        {groups.slice(0, visibleGroups).map((group) => (
          <RTE.ControlsGroup key={group.label}>{group.content}</RTE.ControlsGroup>
        ))}
        {hasOverflow && (
          <Popover.Root open={overflowOpen} onOpenChange={setOverflowOpen}>
            <Popover.Trigger
              ref={overflowTriggerRef}
              type="button"
              className="rte-control-button qt-overflow-trigger"
              aria-label="More formatting"
              title="More formatting"
              disabled={!editable}
            >
              <Ellipsis aria-hidden="true" className="rte-editor-icon" />
            </Popover.Trigger>
            <Popover.Portal container={container}>
              <Popover.Positioner sideOffset={6} align="end" className="qt-editor-positioner">
                <Popover.Popup className="qt-editor-popover qt-overflow-popover">
                  <Popover.Title className="qt-popover-title">Formatting</Popover.Title>
                  {groups.slice(visibleGroups).map((group) => (
                    <div key={group.label} className="qt-overflow-section">
                      <div className="qt-overflow-label">{group.label}</div>
                      <RTE.ControlsGroup>{group.content}</RTE.ControlsGroup>
                    </div>
                  ))}
                </Popover.Popup>
              </Popover.Positioner>
            </Popover.Portal>
          </Popover.Root>
        )}
      </RTE.Toolbar>
    </>
  )
}
