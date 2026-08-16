<script lang="ts">
  import { providerLabels } from '../app'
  import { quotaLabel, quotaReset } from '../format'
  import Icon from '../Icon.svelte'
  import type { Provider } from '../models/providers'

  interface Props {
    providers: Provider[]
    search: string
    onClearSearch: () => void
    onCreate: () => void
    onDelete: (provider: Provider) => void
  }

  let { providers, search, onClearSearch, onCreate, onDelete }: Props = $props()
</script>

{#if providers.length}
  <div class="data-list" aria-label="Providers">
    {#each providers as provider (provider.id)}
      <article class="data-row provider-row">
        <div class="entity-icon provider-icon"><span>{provider.name.slice(0, 1).toUpperCase()}</span></div>
        <div class="entity-main"><h2>{provider.name}</h2><p>{provider.customUrl || 'Default endpoint'}</p></div>
        <span class="type-label">{providerLabels[provider.type]}</span>
        {#if provider.quota}
          <div class="quota" title={quotaReset(provider)}>
            <div><span style={`width:${Math.max(0, 100 - (provider.quota.primary?.usedPercent ?? 100))}%`}></span></div>
            <small>{quotaLabel(provider)}</small>
          </div>
        {/if}
        <span class="status-dot"><i></i> Connected</span>
        <button class="icon-button danger" onclick={() => onDelete(provider)} aria-label={`Delete ${provider.name}`}>
          <Icon name="trash" size={20} />
        </button>
      </article>
    {/each}
  </div>
{:else}
  <div class="state-panel empty-state">
    <div class="state-icon"><Icon name="provider" /></div>
    <div>
      <h2>{search ? 'No matching providers' : 'No providers connected'}</h2>
      <p>{search ? 'Try a different name or clear your search.' : 'Add a provider to start routing model requests.'}</p>
    </div>
    <button class="tonal" onclick={search ? onClearSearch : onCreate}>{search ? 'Clear search' : 'Add provider'}</button>
  </div>
{/if}
