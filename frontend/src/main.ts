import { mount } from 'svelte'
import './app.css'
import { createThemeStore } from './lib/theme.svelte'
import App from './App.svelte'

const theme = createThemeStore()
theme.apply(theme.resolved)

const app = mount(App, {
  target: document.getElementById('app')!,
})



export default app
