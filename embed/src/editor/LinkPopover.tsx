import { Popover } from '@base-ui/react/popover'
import { useEditorState } from '@tiptap/react'
import { useEffect, useId, useRef, useState } from 'react'
import { useRichTextEditorContext } from '../editorcn'
import { isCommentLink } from './extensions'

interface LinkPopoverProps {
  readonly anchor: () => HTMLButtonElement | null
  readonly container: HTMLDivElement | null
  readonly open: boolean
  readonly onOpenChange: (open: boolean) => void
}

export function LinkPopover({ open, onOpenChange, container, anchor }: LinkPopoverProps) {
  const { editor, editable } = useRichTextEditorContext()
  const [url, setUrl] = useState('')
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const id = useId()
  const state = useEditorState({
    editor,
    selector: ({ editor }) => ({
      active: editor?.isActive('link') ?? false,
    }),
  })

  useEffect(() => {
    if (!open) return
    const href: unknown = editor?.getAttributes('link').href
    setUrl(typeof href === 'string' ? href : '')
    setError('')
  }, [editor, open])

  const applyLink = () => {
    if (!editor?.isEditable) return
    const href = url.trim()
    if (!isCommentLink(href)) {
      setError('Enter a complete http:// or https:// URL.')
      inputRef.current?.focus()
      return
    }
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run()
    onOpenChange(false)
  }

  return (
    <Popover.Root open={open && editable} onOpenChange={onOpenChange} modal="trap-focus">
      <Popover.Portal container={container}>
        <Popover.Positioner
          anchor={anchor}
          sideOffset={8}
          align="start"
          className="qt-editor-positioner"
        >
          <Popover.Popup
            className="qt-editor-popover qt-link-popover"
            initialFocus={inputRef}
            finalFocus={anchor}
          >
            <Popover.Title className="qt-popover-title">Edit link</Popover.Title>
            <label htmlFor={id}>URL</label>
            <input
              id={id}
              ref={inputRef}
              type="url"
              value={url}
              placeholder="https://example.com"
              autoComplete="off"
              aria-invalid={Boolean(error)}
              aria-describedby={error ? `${id}-error` : undefined}
              onChange={(event) => {
                setUrl(event.target.value)
                setError('')
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  applyLink()
                }
              }}
            />
            <p id={`${id}-error`} role="status" className="qt-link-error">
              {error}
            </p>
            <div className="qt-link-actions">
              <button
                type="button"
                className="qt-editor-action"
                disabled={!state?.active}
                onClick={() => {
                  editor?.chain().focus().extendMarkRange('link').unsetLink().run()
                  onOpenChange(false)
                }}
              >
                Remove
              </button>
              <button type="button" className="qt-editor-action" onClick={applyLink}>
                Apply link
              </button>
            </div>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  )
}
