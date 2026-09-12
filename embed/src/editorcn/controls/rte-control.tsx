import { AllSelection, TextSelection } from '@tiptap/pm/state'
import type { ChainedCommands, Editor } from '@tiptap/react'
import { useEditorState } from '@tiptap/react'
import type { RichTextEditorIcons } from '../icons'
import type { RichTextEditorLabels } from '../labels'
import { useRichTextEditorContext } from '../rte-context'
import type { RichTextEditorControlProps } from '../types'

type CreateControlProps = {
  readonly label: keyof RichTextEditorLabels
  readonly iconKey: keyof RichTextEditorIcons
  readonly isActive?: (editor: Editor) => boolean
  readonly isDisabled?: (editor: Editor) => boolean
  readonly operation: (chain: ChainedCommands) => ChainedCommands
}

export const RichTextEditorControl = ({
  active,
  interactive = true,
  className = '',
  children,
  onMouseDown,
  disabled,
  ...props
}: RichTextEditorControlProps) => (
  <button
    {...props}
    type="button"
    aria-pressed={active}
    data-state={active ? 'on' : 'off'}
    disabled={disabled || !interactive}
    className={`rte-control-button ${className}`}
    onMouseDown={(event) => {
      event.preventDefault()
      onMouseDown?.(event)
    }}
  >
    {children}
  </button>
)

export const createControl = ({
  label,
  iconKey,
  isActive,
  isDisabled,
  operation,
}: CreateControlProps) => {
  const Control = ({ className }: { readonly className?: string }) => {
    const { editor, labels, icons, editable } = useRichTextEditorContext()
    const ariaLabel = labels[label]
    const editorState = useEditorState({
      editor,
      selector: ({ editor: currentEditor }) => {
        if (!currentEditor || currentEditor.isDestroyed) {
          return { active: false, disabled: true }
        }
        return {
          active: isActive?.(currentEditor) ?? false,
          disabled: isDisabled?.(currentEditor) ?? false,
        }
      },
    })
    const disabled = !editable || (editorState?.disabled ?? true)

    return (
      <RichTextEditorControl
        active={editorState?.active ?? false}
        disabled={disabled}
        aria-label={ariaLabel}
        title={ariaLabel}
        className={className}
        onClick={() => {
          if (!editor || editor.isDestroyed || !editor.isEditable || disabled) return
          const chain = editor.chain().focus()
          if (editor.state.selection instanceof AllSelection) {
            chain.setTextSelection({
              from: TextSelection.atStart(editor.state.doc).from,
              to: TextSelection.atEnd(editor.state.doc).to,
            })
          }
          operation(chain).run()
        }}
      >
        {icons[iconKey]}
      </RichTextEditorControl>
    )
  }
  Control.displayName = `RichTextEditor.${label}`
  return Control
}
