<script lang="ts">
  import Icon from '../Icon.svelte'

  interface Props {
    search: string
    entity: 'API keys' | 'providers'
    total: number
    onRefresh: () => void
  }

  let { search = $bindable(), entity, total, onRefresh }: Props = $props()
</script>

<div class="collection-toolbar">
  <label class="search-field">
    <Icon name="search" size={20} />
    <span class="sr-only">Search {entity}</span>
    <input bind:value={search} placeholder={`Search ${entity}`} />
  </label>
  <span class="count">{total} total</span>
  <button class="icon-button" onclick={onRefresh} aria-label="Refresh"><Icon name="refresh" size={20} /></button>
</div>
