<script lang="ts">
  import { providerLabels } from '../app'
  import { quotaLabel, quotaReset } from '../format'
  import Icon from '../Icon.svelte'
  import { providerTypeAsCLIPROXY, providerTypeAsPrefix, type Provider, type ProviderModel } from '../models/providers'

  interface Props {
    providers: Provider[]
    search: string
    models: ProviderModel[]
    onClearSearch: () => void
    onCreate: () => void
    onEdit: (provider: Provider) => void
    onDelete: (provider: Provider) => void
    onToggleDisabled: (provider: Provider) => void
    togglePendingId: string
  }

  let { providers, search, models, onClearSearch, onCreate, onEdit, onDelete, onToggleDisabled, togglePendingId }: Props = $props()
  let expandedId = $state('')

  function toggle(provider: Provider) {
    expandedId = expandedId === provider.id ? '' : provider.id
  }
  function onKeydown(event: KeyboardEvent, provider: Provider) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    toggle(provider)
  }

  function filteredModels(provider: Provider): ProviderModel[] {
    return models.filter(
      (model) => (model.owned_by === providerTypeAsCLIPROXY(provider.type) || model.owned_by === provider.id) && model.id.startsWith(providerTypeAsPrefix(provider.type) + '/')
    ).sort((a, b) => a.id.localeCompare(b.id, undefined, { sensitivity: 'base' }));
  }
</script>

{#if providers.length}
  <div class="data-list" aria-label="Providers">
    {#each providers as provider (provider.id)}
      <article class:provider-expanded={expandedId === provider.id} class:provider-disabled={provider.disabled} class="provider-item">
        <div class="data-row provider-row" role="button" tabindex="0" aria-expanded={expandedId === provider.id} onclick={() => toggle(provider)} onkeydown={(event) => onKeydown(event, provider)}>
          <div class="entity-icon provider-icon"><span>{provider.name.slice(0, 1).toUpperCase()}</span></div>
          <div class="entity-main"><h2>{provider.name}</h2><p>{provider.customUrl || 'Default endpoint'}</p></div>
          <span class="type-label">{providerLabels[provider.type]}</span>
          {#if provider.quota}
            <div class="quota" title={quotaReset(provider)}>
              <div><span style={`width:${Math.max(0, 100 - (provider.quota.quotas?.[0]?.usedPercent ?? 100))}%`}></span></div>
              <small>{quotaLabel(provider)}</small>
            </div>
          {/if}
          <span class:disabled={provider.disabled} class="status-dot"><i></i> {provider.disabled ? 'Disabled' : 'Connected'}</span>
          <span class:expanded={expandedId === provider.id} class="expand-icon" aria-hidden="true"><Icon name="chevron" size={19} /></span>
          <button
            class:enable={provider.disabled}
            class="icon-button provider-toggle"
            disabled={togglePendingId === provider.id}
            onclick={(event) => { event.stopPropagation(); onToggleDisabled(provider) }}
            aria-label={`${provider.disabled ? 'Enable' : 'Disable'} ${provider.name}`}
            title={`${provider.disabled ? 'Enable' : 'Disable'} provider`}
          >
            <Icon name="power" size={20} />
          </button>
          <button class="icon-button danger" onclick={(event) => { event.stopPropagation(); onDelete(provider) }} aria-label={`Delete ${provider.name}`}>
            <Icon name="trash" size={20} />
          </button>
        </div>
        {#if expandedId === provider.id}
          <section class="provider-details" aria-label={`${provider.name} settings`}>
            <dl>
              <div><dt>Provider ID</dt><dd><code>{provider.id}</code></dd></div>
              <div><dt>Type</dt><dd>{providerLabels[provider.type]}</dd></div>
              <div><dt>Endpoint</dt><dd>{provider.customUrl || 'Default endpoint'}</dd></div>
              <div><dt>Proxy</dt><dd>{provider.useProxy ? (provider.isCloudflare ? 'Cloudflare WARP' : provider.proxy || "Enabled") : 'Disabled'}</dd></div>
              <div><dt>Priority</dt><dd>{provider.priority}</dd></div>
              <div><dt>Status</dt><dd>{provider.disabled ? 'Disabled' : 'Enabled'}</dd></div>
            </dl>
            <button class="tonal" onclick={() => onEdit(provider)}>Edit</button>
          </section>
          {#if provider.quota}
            <section class="quota-details" aria-label={`${provider.name} quotas`}>
                {#each provider.quota?.quotas as quota}
                <div class="quota-card">
                    <div class="quota-header">
                    <span class="quota-name" title={quota.name}>{quota.name}</span>
                    <small>{quotaLabel(provider)}</small>
                    </div>

                    <div class="quota-progress-bar">
                    <span
                        class="quota-progress-fill"
                        style={`width: ${Math.max(0, 100 - (quota.usedPercent ?? 100))}%`}
                    ></span>
                    </div>
                </div>
                {/each}
            </section>
          {/if}
          {#if filteredModels(provider).length}
            <div class="usage-summary">
                <div class="summary-card">
                <span class="summary-label">Available models</span>
                <strong class="summary-value">{filteredModels(provider).length}</strong>
                </div>
            </div>

            <div class="models-list">
            {#each filteredModels(provider) as model}
                <div class="model-row">
                <code>{model.id}</code>
                </div>
            {/each}
            </div>
           {/if}
        {/if}
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
