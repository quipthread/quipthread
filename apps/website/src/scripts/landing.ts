import '@knadh/oat'

const html = document.documentElement

function applyThemeToggle() {
  const isLight = html.getAttribute('data-theme') === 'light'
  if (isLight) {
    html.removeAttribute('data-theme')
    localStorage.setItem('qt-theme', 'dark')
  } else {
    html.setAttribute('data-theme', 'light')
    localStorage.setItem('qt-theme', 'light')
  }
}

document.getElementById('theme-toggle')?.addEventListener('click', applyThemeToggle)
document.getElementById('menu-theme-toggle')?.addEventListener('click', applyThemeToggle)

// Hamburger menu
const menuToggle = document.getElementById('menu-toggle')
const mobileMenu = document.getElementById('mobile-menu')
const nav = menuToggle?.closest('nav')

function closeMenu() {
  nav?.removeAttribute('data-open')
  menuToggle?.setAttribute('aria-expanded', 'false')
  mobileMenu?.setAttribute('aria-hidden', 'true')
}

menuToggle?.addEventListener('click', (e) => {
  e.stopPropagation()
  const isOpen = nav?.hasAttribute('data-open')
  if (isOpen) {
    closeMenu()
  } else {
    nav?.setAttribute('data-open', '')
    menuToggle.setAttribute('aria-expanded', 'true')
    mobileMenu?.setAttribute('aria-hidden', 'false')
  }
})

mobileMenu?.querySelectorAll('a').forEach((link) => {
  link.addEventListener('click', closeMenu)
})

document.addEventListener('click', (e) => {
  if (nav && !nav.contains(e.target as Node)) closeMenu()
})

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeMenu()
})

// Login state detection via indicator cookie
const appUrl =
  (import.meta.env.PUBLIC_BACKEND_URL as string | undefined) || 'https://app.quipthread.com'

function getCookie(name: string): string | null {
  const m = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`))
  return m ? decodeURIComponent(m[1]) : null
}

if (getCookie('qt_logged_in') === '1') {
  const navLogin = document.querySelector<HTMLAnchorElement>('.nav-login')
  if (navLogin) {
    navLogin.textContent = 'Dashboard'
    navLogin.href = `${appUrl}/dashboard/comments`
  }
  const mobileLogin = document.querySelector<HTMLAnchorElement>('.mobile-login')
  if (mobileLogin) {
    mobileLogin.textContent = 'Dashboard'
    mobileLogin.href = `${appUrl}/dashboard/comments`
  }

  document.querySelectorAll<HTMLAnchorElement>('[data-plan]').forEach((el) => {
    if (el.dataset.plan === 'hobby') {
      el.href = `${appUrl}/dashboard/comments`
      el.textContent = 'Open dashboard'
    } else {
      el.href = `${appUrl}/dashboard/billing`
      el.textContent = 'Go to billing'
    }
  })
}
