import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from 'react'
// Type-only: the editor and its dependencies stay out of the widget chunk.
import type { EditorProps, EditorRef } from './Editor'

type EditorModule = typeof import('./Editor')

let editorModule: EditorModule | null = null
let loading: Promise<EditorModule> | null = null

/** Loads the editor chunk once per page; a failed load can be retried. */
function loadEditor(): Promise<EditorModule> {
  if (!loading) {
    loading = import('./Editor').then((mod) => {
      editorModule = mod
      return mod
    })
    loading.catch(() => {
      loading = null
    })
  }
  return loading
}

interface LazyEditorProps extends EditorProps {
  /** Load right away instead of on intent, e.g. a reply form the reader just opened. */
  readonly loadNow?: boolean
  readonly loadErrorText: string
}

/**
 * The comment editor, loaded when the reader shows intent to write. Hovering
 * or touching the stand-in starts the download; clicking or focusing it shows
 * the editor and focuses it as soon as it is ready. A saved draft or loadNow
 * shows it immediately.
 */
export const LazyEditor = forwardRef<EditorRef, LazyEditorProps>(
  ({ loadNow = false, loadErrorText, ...props }, ref) => {
    const [mod, setMod] = useState<EditorModule | null>(editorModule)
    const [failed, setFailed] = useState(false)
    const [focusOnLoad, setFocusOnLoad] = useState(false)
    const editorRef = useRef<EditorRef>(null)
    const active = useRef(true)

    const show = useCallback(() => {
      if (editorModule) {
        setMod(editorModule)
        return
      }
      setFailed(false)
      loadEditor().then(
        (loaded) => active.current && setMod(loaded),
        () => active.current && setFailed(true),
      )
    }, [])

    const prefetch = () => {
      loadEditor().catch(() => {
        // Reported if the reader goes on to open the editor.
      })
    }

    const activate = () => {
      setFocusOnLoad(true)
      show()
    }

    const eager = loadNow || Boolean(props.initialContent)
    useEffect(() => {
      active.current = true
      if (eager) show()
      return () => {
        active.current = false
      }
    }, [eager, show])

    // Until the editor exists the form sees it as empty.
    useImperativeHandle(ref, () => ({
      getHTML: () => editorRef.current?.getHTML() ?? '',
      isEmpty: () => editorRef.current?.isEmpty() ?? true,
      clear: () => editorRef.current?.clear(),
    }))

    if (mod) return <mod.Editor ref={editorRef} {...props} autoFocus={focusOnLoad} />

    if (failed) {
      return (
        <p className="qt-error" role="alert">
          {loadErrorText}
        </p>
      )
    }

    return (
      <button
        type="button"
        className="qt-composer-standin"
        disabled={props.disabled}
        onPointerEnter={prefetch}
        onTouchStart={prefetch}
        onFocus={activate}
        onClick={activate}
      >
        <span>{props.placeholder}</span>
      </button>
    )
  },
)

LazyEditor.displayName = 'LazyEditor'
