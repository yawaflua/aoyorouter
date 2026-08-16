<script lang="ts">
  import type { SectionDefinition } from '../app'
  import Icon from '../Icon.svelte'

  interface Props {
    section: SectionDefinition
    onAction: () => void
  }

  let { section, onAction }: Props = $props()
</script>

<header class="topbar">
  <div>
    <p class="eyebrow">ADMIN CONSOLE</p>
    <h1>{section.title}</h1>
    <p>{section.description}</p>
  </div>
  {#if section.id !== 'logs'}
    <button class="filled top-action" onclick={onAction}>
      <Icon name="plus" size={20} /> {section.id === 'keys' ? 'Create key' : 'Add provider'}
    </button>
  {/if}
</header>
