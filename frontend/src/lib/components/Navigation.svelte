<script lang="ts">
  import { sections, type Section } from '../app'
  import Icon from '../Icon.svelte'

  interface Props {
    current: Section
    onNavigate: (section: Section) => void
    onSignOut: () => void
  }

  let { current, onNavigate, onSignOut }: Props = $props()
</script>

<aside class="rail" aria-label="Primary navigation">
  <div class="rail-brand"><div class="brand-mark small"><span></span><span></span><span></span></div><strong>Aoyo</strong></div>
  <nav>
    {#each sections as item}
      <button class:active={current === item.id} onclick={() => onNavigate(item.id)} aria-current={current === item.id ? 'page' : undefined}>
        <Icon name={item.icon} /><span>{item.label}</span>
      </button>
    {/each}
  </nav>
  <button class="rail-signout" onclick={onSignOut}><Icon name="logout" /><span>Sign out</span></button>
</aside>

<nav class="bottom-nav" aria-label="Primary navigation">
  {#each sections as item}
    <button class:active={current === item.id} onclick={() => onNavigate(item.id)}>
      <Icon name={item.icon} /><span>{item.label}</span>
    </button>
  {/each}
</nav>
