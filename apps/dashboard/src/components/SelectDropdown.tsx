import { useEffect, useRef, useState } from 'preact/hooks'

interface Option {
  value: string
  label: string
}

interface Props {
  value: string
  options: Option[]
  onChange: (value: string) => void
  disabled?: boolean
}

export default function SelectDropdown({ value, options, onChange, disabled }: Props) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const selected = options.find((o) => o.value === value)

  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  return (
    <div
      ref={ref}
      style={{ position: 'relative', display: 'inline-block', alignSelf: 'flex-start' }}
    >
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((o) => !o)}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.5rem',
          fontFamily: 'var(--f-ui)',
          fontSize: '0.875rem',
          color: 'var(--text)',
          background: 'var(--card-bg)',
          border: '1px solid var(--border)',
          borderRadius: 6,
          padding: '0.5rem 0.625rem 0.5rem 0.75rem',
          cursor: disabled ? 'not-allowed' : 'pointer',
          opacity: disabled ? 0.5 : 1,
          outline: 'none',
          transition: 'border-color 0.15s, box-shadow 0.15s',
          whiteSpace: 'nowrap' as const,
          ...(open
            ? {
                borderColor: 'var(--amber)',
                boxShadow: '0 0 0 3px var(--amber-bg)',
              }
            : {}),
        }}
      >
        <span>{selected?.label ?? value}</span>
        <svg
          width="12"
          height="12"
          viewBox="0 0 12 12"
          fill="none"
          aria-hidden="true"
          style={{
            flexShrink: 0,
            transition: 'transform 150ms',
            transform: open ? 'rotate(180deg)' : 'rotate(0deg)',
          }}
        >
          <path
            d="M2 4.5l4 4 4-4"
            stroke="var(--muted)"
            stroke-width="1.5"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </button>

      {open && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 4px)',
            left: 0,
            zIndex: 200,
            minWidth: '100%',
            background: 'var(--card-bg)',
            border: '1px solid var(--border)',
            borderRadius: 8,
            boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
            overflow: 'hidden',
          }}
        >
          {options.map((o) => (
            <button
              key={o.value}
              type="button"
              onClick={() => {
                onChange(o.value)
                setOpen(false)
              }}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '0.5rem',
                width: '100%',
                padding: '0.5rem 0.75rem',
                fontFamily: 'var(--f-ui)',
                fontSize: '0.875rem',
                background: o.value === value ? 'var(--amber-bg)' : 'none',
                color: o.value === value ? 'var(--amber)' : 'var(--text)',
                border: 'none',
                cursor: 'pointer',
                textAlign: 'left' as const,
                whiteSpace: 'nowrap' as const,
                transition: 'background 0.1s',
              }}
            >
              <svg
                width="12"
                height="12"
                viewBox="0 0 12 12"
                fill="none"
                aria-hidden="true"
                style={{ flexShrink: 0, opacity: o.value === value ? 1 : 0 }}
              >
                <path
                  d="M2 6.5l3 3 5-6"
                  stroke="currentColor"
                  stroke-width="1.5"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              </svg>
              {o.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
