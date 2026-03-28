import type { ComponentChildren } from 'preact'

interface InlineMsgProps {
  type: 'success' | 'error'
  children: ComponentChildren
}

const styles: Record<string, object> = {
  success: {
    color: 'var(--green-text)',
    background: 'var(--green-bg)',
    border: '1px solid var(--green-border)',
  },
  error: {
    color: 'var(--red-text)',
    background: 'var(--red-bg)',
    border: '1px solid var(--red-border)',
  },
}

export default function InlineMsg({ type, children }: InlineMsgProps) {
  return (
    <div
      style={{
        borderRadius: 6,
        padding: '0.5rem 0.875rem',
        fontSize: '0.8125rem',
        marginTop: '0.75rem',
        ...styles[type],
      }}
    >
      {children}
    </div>
  )
}
