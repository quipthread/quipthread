import type { ComponentChildren } from 'preact'

type BadgeVariant = 'pending' | 'approved' | 'rejected' | 'admin' | 'user' | 'banned' | 'shadow'

interface BadgeProps {
  variant: BadgeVariant
  children: ComponentChildren
}

export default function Badge({ variant, children }: BadgeProps) {
  return <span className={`badge badge-${variant}`}>{children}</span>
}
