<script lang="ts">
  import Icon from '../Icon.svelte'
  import type { Theme } from '../theme.svelte'

  interface Props {
    theme: Theme
    onChange: (theme: Theme) => void
  }

  let { theme, onChange }: Props = $props()

  const items: { value: Theme; label: string; icon: 'sun' | 'moon' | 'monitor' }[] = [
    { value: 'light', label: 'Light', icon: 'sun' },
    { value: 'dark', label: 'Dark', icon: 'moon' },
    { value: 'system', label: 'System', icon: 'monitor' },
  ]

  let open = $state(false)

  function select(value: Theme) {
    onChange(value)
    open = false
  }
</script>

<div class="theme-toggle">
  <button
    class="icon-button"
    type="button"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label="Theme"
    onclick={() => (open = !open)}
  >
    <Icon name={theme === 'dark' ? 'moon' : theme === 'light' ? 'sun' : 'monitor'} size={20} />
  </button>
  {#if open}
    <div class="theme-menu" role="menu">
      {#each items as item}
        <button
          type="button"
          role="menuitem"
          class:active={theme === item.value}
          onclick={() => select(item.value)}
        >
          <Icon name={item.icon} size={18} />
          <span>{item.label}</span>
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .theme-toggle {
    position: relative;
  }
  .theme-menu {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 30;
    min-width: 140px;
    display: grid;
    padding: 8px;
    border-radius: 12px;
    background: var(--surface-container);
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
    border: 1px solid var(--outline-variant);
  }
  .theme-menu button {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--on-surface);
    font: inherit;
    font-size: 0.88rem;
    text-align: left;
    cursor: pointer;
  }
  .theme-menu button:hover,
  .theme-menu button.active {
    background: var(--primary-container);
    color: var(--on-primary-container);
  }
</style>
