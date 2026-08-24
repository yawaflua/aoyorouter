export type Theme = 'light' | 'dark' | 'system'

const THEME_KEY = 'aoyo.theme'

function resolveTheme(theme: Theme): 'light' | 'dark' {
  if (theme !== 'system') return theme
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyThemeAttribute(theme: 'light' | 'dark') {
  document.documentElement.setAttribute('data-theme', theme)
  document.documentElement.style.colorScheme = theme
}

function applyMeta(theme: 'light' | 'dark') {
  const meta = document.querySelector('meta[name="theme-color"]') as HTMLMetaElement | null
  if (meta) {
    meta.content = theme === 'dark' ? '#111318' : '#f8f9ff'
  }
}

function apply(theme: 'light' | 'dark') {
  applyThemeAttribute(theme)
  applyMeta(theme)
}

export function createThemeStore() {
  const saved = (localStorage.getItem(THEME_KEY) as Theme | null) ?? 'system'
  let theme = $state<Theme>(saved)
  let resolved = $state<'light' | 'dark'>(resolveTheme(saved))

  function set(value: Theme) {
    theme = value
    resolved = resolveTheme(value)
    localStorage.setItem(THEME_KEY, value)
    apply(resolved)
  }

  function init() {
    const value = (localStorage.getItem(THEME_KEY) as Theme | null) ?? 'system'
    set(value)
  }

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (event) => {
    if (theme === 'system') {
      resolved = event.matches ? 'dark' : 'light'
      apply(resolved)
    }
  })

  return {
    get theme() {
      return theme
    },
    get resolved() {
      return resolved
    },
    set,
    init,
    apply,
  }
}
