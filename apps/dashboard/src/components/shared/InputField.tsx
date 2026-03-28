import type { ComponentChildren } from 'preact'

interface InputFieldProps {
  label: string
  htmlFor: string
  children: ComponentChildren
}

const labelStyle = {
  display: 'block',
  fontSize: '0.8125rem',
  fontWeight: 500,
  color: 'var(--muted)',
  marginBottom: '0.375rem',
} as const

export default function InputField({ label, htmlFor, children }: InputFieldProps) {
  return (
    <div>
      <label htmlFor={htmlFor} style={labelStyle}>
        {label}
      </label>
      {children}
    </div>
  )
}
