export interface User {
  id: string
  display_name: string
  provider: string
  role: string
}

export interface Comment {
  id: string
  site_id: string
  page_id: string
  page_url: string
  page_title: string
  parent_id: string
  user_id: string
  content: string
  status: string
  author_name: string
  author_avatar: string
  created_at: string
  updated_at: string
}

export interface CommentsResponse {
  comments: Comment[]
  total: number
  page: number
  limit: number
}

export interface CreateCommentInput {
  site_id: string
  page_id: string
  page_url?: string
  page_title?: string
  parent_id?: string
  content: string
  turnstile_token?: string
}

export interface WidgetConfig {
  turnstileSiteKey: string
  theme?: string
}
