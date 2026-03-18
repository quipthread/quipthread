const CSS = `
.qt-root {
  --qt-bg: #ffffff;
  --qt-bg-alt: #f5f5f5;
  --qt-text: #1A1714;
  --qt-text-muted: #7A7570;
  --qt-border: #D9D4CB;
  --qt-accent: #C06020;
  --qt-accent-hover: #9A4A15;
  --qt-danger: #c0392b;
  --qt-radius-sm: 4px;
  --qt-radius: 8px;
  --qt-radius-lg: 12px;
  --qt-font: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  font-family: var(--qt-font);
  color: var(--qt-text);
  background: var(--qt-bg);
  line-height: 1.5;
  box-sizing: border-box;
}
.qt-root *, .qt-root *::before, .qt-root *::after {
  box-sizing: inherit;
}
@media (prefers-color-scheme: dark) {
  .qt-root[data-theme="auto"] {
    --qt-bg: #0F0F0F;
    --qt-bg-alt: #1A1A1A;
    --qt-text: #E8E3DC;
    --qt-text-muted: #8A8480;
    --qt-border: #2E2E2E;
    --qt-accent: #E07F32;
    --qt-accent-hover: #F0A06A;
  }
}
.qt-root[data-theme="dark"] {
  --qt-bg: #0F0F0F;
  --qt-bg-alt: #1A1A1A;
  --qt-text: #E8E3DC;
  --qt-text-muted: #8A8480;
  --qt-border: #2E2E2E;
  --qt-accent: #E07F32;
  --qt-accent-hover: #F0A06A;
}

/* --- Theme presets -------------------------------------------------------- */

.qt-root[data-theme="catppuccin-latte"] {
  --qt-bg: #eff1f5;
  --qt-bg-alt: #e6e9ef;
  --qt-text: #4c4f69;
  --qt-text-muted: #6c6f85;
  --qt-border: #ccd0da;
  --qt-accent: #1e66f5;
  --qt-accent-hover: #1657d6;
  --qt-danger: #d20f39;
}
.qt-root[data-theme="catppuccin-frappe"] {
  --qt-bg: #303446;
  --qt-bg-alt: #292c3c;
  --qt-text: #c6d0f5;
  --qt-text-muted: #a5adce;
  --qt-border: #414559;
  --qt-accent: #8caaee;
  --qt-accent-hover: #709ae2;
  --qt-danger: #e78284;
}
.qt-root[data-theme="catppuccin-macchiato"] {
  --qt-bg: #24273a;
  --qt-bg-alt: #1e2030;
  --qt-text: #cad3f5;
  --qt-text-muted: #a5adcb;
  --qt-border: #363a4f;
  --qt-accent: #8aadf4;
  --qt-accent-hover: #6e9bee;
  --qt-danger: #ed8796;
}
.qt-root[data-theme="catppuccin-mocha"] {
  --qt-bg: #1e1e2e;
  --qt-bg-alt: #181825;
  --qt-text: #cdd6f4;
  --qt-text-muted: #a6adc8;
  --qt-border: #313244;
  --qt-accent: #89b4fa;
  --qt-accent-hover: #6da4f8;
  --qt-danger: #f38ba8;
}
.qt-root[data-theme="dracula"] {
  --qt-bg: #282a36;
  --qt-bg-alt: #1e1f29;
  --qt-text: #f8f8f2;
  --qt-text-muted: #6272a4;
  --qt-border: #44475a;
  --qt-accent: #bd93f9;
  --qt-accent-hover: #a779f7;
  --qt-danger: #ff5555;
}
.qt-root[data-theme="nord"] {
  --qt-bg: #2e3440;
  --qt-bg-alt: #242932;
  --qt-text: #eceff4;
  --qt-text-muted: #9099a8;
  --qt-border: #3b4252;
  --qt-accent: #88c0d0;
  --qt-accent-hover: #6aabb9;
  --qt-danger: #bf616a;
}
.qt-root[data-theme="gruvbox-light"] {
  --qt-bg: #fbf1c7;
  --qt-bg-alt: #f2e5bc;
  --qt-text: #3c3836;
  --qt-text-muted: #7c6f64;
  --qt-border: #d5c4a1;
  --qt-accent: #d65d0e;
  --qt-accent-hover: #af4a04;
  --qt-danger: #cc241d;
}
.qt-root[data-theme="gruvbox-dark"] {
  --qt-bg: #282828;
  --qt-bg-alt: #1d2021;
  --qt-text: #ebdbb2;
  --qt-text-muted: #a89984;
  --qt-border: #3c3836;
  --qt-accent: #d79921;
  --qt-accent-hover: #b57614;
  --qt-danger: #cc241d;
}
.qt-root[data-theme="tokyo-night"] {
  --qt-bg: #1a1b26;
  --qt-bg-alt: #16161e;
  --qt-text: #c0caf5;
  --qt-text-muted: #565f89;
  --qt-border: #292e42;
  --qt-accent: #7aa2f7;
  --qt-accent-hover: #5d8ef5;
  --qt-danger: #f7768e;
}
.qt-root[data-theme="rose-pine"] {
  --qt-bg: #191724;
  --qt-bg-alt: #1f1d2e;
  --qt-text: #e0def4;
  --qt-text-muted: #6e6a86;
  --qt-border: #26233a;
  --qt-accent: #c4a7e7;
  --qt-accent-hover: #aa8fd0;
  --qt-danger: #eb6f92;
}
.qt-root[data-theme="rose-pine-dawn"] {
  --qt-bg: #faf4ed;
  --qt-bg-alt: #fffaf3;
  --qt-text: #575279;
  --qt-text-muted: #9893a5;
  --qt-border: #dfdad9;
  --qt-accent: #907aa9;
  --qt-accent-hover: #7b6491;
  --qt-danger: #b4637a;
}
.qt-root[data-theme="solarized-light"] {
  --qt-bg: #fdf6e3;
  --qt-bg-alt: #eee8d5;
  --qt-text: #657b83;
  --qt-text-muted: #93a1a1;
  --qt-border: #d4cdb5;
  --qt-accent: #268bd2;
  --qt-accent-hover: #1a6faa;
  --qt-danger: #dc322f;
}
.qt-root[data-theme="solarized-dark"] {
  --qt-bg: #002b36;
  --qt-bg-alt: #073642;
  --qt-text: #839496;
  --qt-text-muted: #586e75;
  --qt-border: #094952;
  --qt-accent: #268bd2;
  --qt-accent-hover: #1a6faa;
  --qt-danger: #dc322f;
}
.qt-root[data-theme="one-dark"] {
  --qt-bg: #282c34;
  --qt-bg-alt: #21252b;
  --qt-text: #abb2bf;
  --qt-text-muted: #636d83;
  --qt-border: #3e4451;
  --qt-accent: #61afef;
  --qt-accent-hover: #4e9fe0;
  --qt-danger: #e06c75;
}

/* Header */
.qt-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.5rem;
}
.qt-title {
  font-size: 1rem;
  font-weight: 600;
  margin: 0;
  color: var(--qt-text);
}

/* Buttons */
.qt-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.5rem 1rem;
  border-radius: var(--qt-radius);
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid transparent;
  font-family: var(--qt-font);
  transition: background-color 0.15s, border-color 0.15s;
  text-decoration: none;
  line-height: 1.25;
}
.qt-btn-primary {
  background: var(--qt-accent);
  color: #fff;
  border-color: var(--qt-accent);
}
.qt-btn-primary:hover {
  background: var(--qt-accent-hover);
  border-color: var(--qt-accent-hover);
}
.qt-btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
.qt-btn-secondary {
  background: transparent;
  color: var(--qt-text);
  border-color: var(--qt-border);
}
.qt-btn-secondary:hover {
  border-color: var(--qt-text-muted);
}
.qt-btn-ghost {
  background: transparent;
  color: var(--qt-text-muted);
  border-color: transparent;
  padding: 0.25rem 0.5rem;
  font-size: 0.8125rem;
}
.qt-btn-ghost:hover {
  color: var(--qt-text);
  background: var(--qt-bg-alt);
}
.qt-btn-danger-ghost {
  background: transparent;
  color: var(--qt-text-muted);
  border-color: transparent;
  padding: 0.25rem 0.5rem;
  font-size: 0.8125rem;
}
.qt-btn-danger-ghost:hover {
  color: var(--qt-danger);
  background: var(--qt-bg-alt);
}

/* User bar */
.qt-user-bar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8125rem;
  color: var(--qt-text-muted);
}
.qt-user-bar-avatar {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  object-fit: cover;
}
.qt-user-bar-name {
  color: var(--qt-text);
  font-weight: 500;
}

/* Comment list */
.qt-comment-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

/* Comment item */
.qt-comment {
  display: flex;
  gap: 0.75rem;
}
.qt-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  object-fit: cover;
  flex-shrink: 0;
}
.qt-avatar-placeholder {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: var(--qt-bg-alt);
  border: 1px solid var(--qt-border);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--qt-text-muted);
  flex-shrink: 0;
  text-transform: uppercase;
}
.qt-comment-body {
  flex: 1;
  min-width: 0;
}
.qt-comment-meta {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
  flex-wrap: wrap;
}
.qt-author {
  font-weight: 600;
  font-size: 0.875rem;
  color: var(--qt-text);
}
.qt-timestamp {
  font-size: 0.75rem;
  color: var(--qt-text-muted);
}
.qt-comment-content {
  font-size: 0.9375rem;
  color: var(--qt-text);
  line-height: 1.625;
}
.qt-comment-content p {
  margin: 0 0 0.5em;
}
.qt-comment-content p:last-child {
  margin-bottom: 0;
}
.qt-comment-content a {
  color: var(--qt-accent);
  text-decoration: none;
}
.qt-comment-content a:hover {
  text-decoration: underline;
}
.qt-comment-content code {
  background: var(--qt-bg-alt);
  border: 1px solid var(--qt-border);
  padding: 0.1em 0.35em;
  border-radius: var(--qt-radius-sm);
  font-size: 0.875em;
}
.qt-comment-content pre {
  background: var(--qt-bg-alt);
  border: 1px solid var(--qt-border);
  border-radius: var(--qt-radius);
  padding: 0.75rem 1rem;
  overflow-x: auto;
  margin: 0.5em 0;
}
.qt-comment-content pre code {
  background: none;
  border: none;
  padding: 0;
  font-size: 0.875rem;
}
.qt-comment-content ul,
.qt-comment-content ol {
  margin: 0.375em 0;
  padding-left: 1.5rem;
}
.qt-comment-content li + li {
  margin-top: 0.125em;
}
.qt-comment-actions {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  margin-top: 0.375rem;
}

/* Replies */
.qt-replies {
  margin-top: 1rem;
  padding-left: 1rem;
  border-left: 2px solid var(--qt-border);
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.qt-reply-comment {
  display: flex;
  gap: 0.625rem;
}
.qt-reply-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  object-fit: cover;
  flex-shrink: 0;
}
.qt-reply-avatar-placeholder {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--qt-bg-alt);
  border: 1px solid var(--qt-border);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--qt-text-muted);
  flex-shrink: 0;
  text-transform: uppercase;
}

/* Inline reply form */
.qt-reply-form {
  margin-top: 0.75rem;
}

/* Pagination */
.qt-pagination {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 1.5rem;
  justify-content: center;
}

/* Editor */
.qt-editor-wrapper {
  border: 1px solid var(--qt-border);
  border-radius: var(--qt-radius);
  overflow: hidden;
  background: var(--qt-bg);
  transition: border-color 0.15s, box-shadow 0.15s;
}
.qt-editor-wrapper:focus-within {
  border-color: var(--qt-accent);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--qt-accent) 20%, transparent);
}
.qt-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 0.375rem 0.5rem;
  border-bottom: 1px solid var(--qt-border);
  background: var(--qt-bg-alt);
}
.qt-toolbar-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: var(--qt-radius-sm);
  cursor: pointer;
  color: var(--qt-text-muted);
  font-family: var(--qt-font);
  font-size: 0.875rem;
  transition: background-color 0.1s, color 0.1s;
}
.qt-toolbar-btn:hover {
  background: var(--qt-border);
  color: var(--qt-text);
}
.qt-toolbar-btn.is-active {
  background: var(--qt-accent);
  color: #fff;
}
.qt-toolbar-separator {
  width: 1px;
  background: var(--qt-border);
  margin: 4px 2px;
  align-self: stretch;
}
.qt-editor-content {
  padding: 0.625rem 0.75rem;
  min-height: 90px;
  cursor: text;
}
.qt-editor-content .ProseMirror {
  outline: none;
  min-height: 70px;
  font-size: 0.9375rem;
  color: var(--qt-text);
  caret-color: var(--qt-text);
}
.qt-editor-content .ProseMirror p {
  margin: 0 0 0.5em;
}
.qt-editor-content .ProseMirror p:last-child {
  margin-bottom: 0;
}
.qt-editor-content .ProseMirror p.is-editor-empty:first-child::before {
  content: attr(data-placeholder);
  float: left;
  color: var(--qt-text-muted);
  pointer-events: none;
  height: 0;
}
.qt-editor-content .ProseMirror a {
  color: var(--qt-accent);
}
.qt-editor-content .ProseMirror code {
  background: var(--qt-bg-alt);
  border: 1px solid var(--qt-border);
  padding: 0.1em 0.35em;
  border-radius: var(--qt-radius-sm);
  font-size: 0.875em;
}
.qt-editor-content .ProseMirror ul,
.qt-editor-content .ProseMirror ol {
  margin: 0.375em 0;
  padding-left: 1.5rem;
}
.qt-form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.625rem;
}

/* New comment section */
.qt-new-comment {
  margin-top: 2rem;
  padding-top: 1.5rem;
  border-top: 1px solid var(--qt-border);
}
.qt-new-comment-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.75rem;
}
.qt-new-comment-label {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--qt-text-muted);
}

/* Login prompt */
.qt-login-prompt {
  text-align: center;
  padding: 1.5rem;
  background: var(--qt-bg-alt);
  border-radius: var(--qt-radius);
  border: 1px solid var(--qt-border);
}
.qt-login-prompt p {
  margin: 0 0 1rem;
  color: var(--qt-text-muted);
  font-size: 0.9375rem;
}

/* Pending notice */
.qt-pending-notice {
  padding: 0.625rem 0.875rem;
  background: var(--qt-bg-alt);
  border: 1px solid var(--qt-border);
  border-radius: var(--qt-radius);
  font-size: 0.8125rem;
  color: var(--qt-text-muted);
  margin-top: 0.75rem;
}

/* Auth modal */
.qt-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  padding: 1rem;
  backdrop-filter: blur(2px);
}
.qt-modal {
  background: var(--qt-bg);
  border-radius: var(--qt-radius-lg);
  padding: 1.75rem;
  width: 100%;
  max-width: 380px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.25);
  border: 1px solid var(--qt-border);
}
.qt-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.25rem;
}
.qt-modal-title {
  font-size: 1.125rem;
  font-weight: 700;
  margin: 0;
  color: var(--qt-text);
  line-height: 1.3;
}
.qt-modal-subtitle {
  font-size: 0.8125rem;
  color: var(--qt-text-muted);
  margin: 0.25rem 0 0;
}
.qt-modal-close {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--qt-text-muted);
  padding: 0.125rem;
  border-radius: var(--qt-radius-sm);
  font-size: 1.25rem;
  line-height: 1;
  flex-shrink: 0;
  margin-left: 0.5rem;
}
.qt-modal-close:hover {
  color: var(--qt-text);
}
.qt-oauth-buttons {
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
}
.qt-oauth-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.625rem;
  padding: 0.6875rem 1rem;
  border-radius: var(--qt-radius);
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  border: 1px solid var(--qt-border);
  background: var(--qt-bg);
  color: var(--qt-text);
  font-family: var(--qt-font);
  text-decoration: none;
  transition: background-color 0.15s;
  width: 100%;
}
.qt-oauth-btn:hover {
  background: var(--qt-bg-alt);
}
.qt-oauth-btn svg {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

/* Loading / empty states */
.qt-loading {
  text-align: center;
  padding: 2rem;
  color: var(--qt-text-muted);
  font-size: 0.9375rem;
}
.qt-empty {
  text-align: center;
  padding: 2rem;
  color: var(--qt-text-muted);
  font-size: 0.9375rem;
}
.qt-error {
  text-align: center;
  padding: 1rem;
  color: var(--qt-danger);
  font-size: 0.875rem;
}
`

let injected = false

export function injectStyles(): void {
  if (injected) return
  injected = true
  const style = document.createElement('style')
  style.textContent = CSS
  document.head.appendChild(style)
}
