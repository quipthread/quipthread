const THEMES: { value: string; label: string; bg: string; accent: string }[] = [
  { value: 'auto', label: 'Auto', bg: '#888888', accent: '#e07f32' },
  { value: 'light', label: 'Light', bg: '#f7f4ef', accent: '#c06020' },
  { value: 'dark', label: 'Dark', bg: '#0f0f0f', accent: '#e07f32' },
  { value: 'catppuccin-latte', label: 'Latte', bg: '#eff1f5', accent: '#1e66f5' },
  { value: 'catppuccin-frappe', label: 'Frappé', bg: '#303446', accent: '#8caaee' },
  { value: 'catppuccin-macchiato', label: 'Macchiato', bg: '#24273a', accent: '#8aadf4' },
  { value: 'catppuccin-mocha', label: 'Mocha', bg: '#1e1e2e', accent: '#89b4fa' },
  { value: 'dracula', label: 'Dracula', bg: '#282a36', accent: '#bd93f9' },
  { value: 'nord', label: 'Nord', bg: '#2e3440', accent: '#88c0d0' },
  { value: 'gruvbox-light', label: 'Gruvbox Light', bg: '#fbf1c7', accent: '#d65d0e' },
  { value: 'gruvbox-dark', label: 'Gruvbox Dark', bg: '#282828', accent: '#d79921' },
  { value: 'tokyo-night', label: 'Tokyo Night', bg: '#1a1b26', accent: '#7aa2f7' },
  { value: 'rose-pine', label: 'Rosé Pine', bg: '#191724', accent: '#c4a7e7' },
  { value: 'rose-pine-dawn', label: 'Rosé Pine Dawn', bg: '#faf4ed', accent: '#907aa9' },
  { value: 'solarized-light', label: 'Solarized Light', bg: '#fdf6e3', accent: '#268bd2' },
  { value: 'solarized-dark', label: 'Solarized Dark', bg: '#002b36', accent: '#268bd2' },
  { value: 'one-dark', label: 'One Dark', bg: '#282c34', accent: '#61afef' },
]

interface Props {
  value: string
  onChange: (theme: string) => void
  disabled?: boolean
}

export default function ThemeSwatches({ value, onChange, disabled }: Props) {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem' }}>
      {THEMES.map((t) => {
        const active = value === t.value
        return (
          <button
            type="button"
            key={t.value}
            title={t.label}
            disabled={disabled}
            onClick={() => onChange(t.value)}
            style={{
              width: 36,
              height: 36,
              borderRadius: 7,
              border: active ? '2px solid var(--amber-hi)' : '2px solid var(--border)',
              outline: active ? '2px solid var(--amber)' : 'none',
              outlineOffset: '1px',
              background: `linear-gradient(135deg, ${t.bg} 40%, ${t.accent})`,
              cursor: disabled ? 'not-allowed' : 'pointer',
              padding: 0,
              flexShrink: 0,
              opacity: disabled ? 0.5 : 1,
              transition: 'outline 0.1s, border-color 0.1s, opacity 0.1s',
            }}
          />
        )
      })}
    </div>
  )
}
