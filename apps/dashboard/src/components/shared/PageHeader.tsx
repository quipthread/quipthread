import type { ComponentChildren } from 'preact'

interface PageHeaderProps {
  title: string
  action?: ComponentChildren
}

export default function PageHeader({ title, action }: PageHeaderProps) {
  return (
    <div className="page-header">
      <h1>{title}</h1>
      {action}
    </div>
  )
}
