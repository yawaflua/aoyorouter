<script lang="ts">
  import { providerLabels } from '../app'
  import { quotaLabel, quotaReset, quotaResetLabel } from '../format'
  import { createTicker } from '../now.svelte'
  import Icon from '../Icon.svelte'
  import ProviderIcon from './ProviderIcon.svelte'
  import { providerTypeAsCLIPROXY, providerTypeAsPrefix, type Provider, type ProviderModel, type ProviderType } from '../models/providers'
    import type { QuotaWindow } from '../models/quota';

  interface Props {
    providers: Provider[]
    search: string
    models: ProviderModel[]
    onClearSearch: () => void
    onCreate: () => void
    onEdit: (provider: Provider) => void
    onDelete: (provider: Provider) => void
    onToggleDisabled: (provider: Provider) => void
    onReload: () => void
    onCopy: (value: string, success: string) => void
    togglePendingId: string
    reloadPending: boolean
  }

  let { providers, search, models, onClearSearch, onCreate, onEdit, onDelete, onToggleDisabled, onReload, onCopy, togglePendingId, reloadPending }: Props = $props()
  let expandedType = $state<ProviderType | null>(null)
  let expandedProviderId = $state<string | null>(null)
  const ticker = createTicker()

  const grouped = $derived(() => {
    const map = new Map<ProviderType, Provider[]>()
    for (const provider of providers) {
      const list = map.get(provider.type) ?? []
      list.push(provider)
      map.set(provider.type, list)
    }
    return Array.from(map.entries()).sort((a, b) => providerLabels[a[0]].localeCompare(providerLabels[b[0]]))
  })

  function toggleType(type: ProviderType) {
    expandedType = expandedType === type ? null : type
  }

  function toggleProvider(provider: Provider) {
    expandedProviderId = expandedProviderId === provider.id ? null : provider.id
  }

  function onProviderKeydown(event: KeyboardEvent, provider: Provider) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    toggleProvider(provider)
  }

  function typeKey(type: string): string {
    return type.toLowerCase().replace(/_/g, '-')
  }

  function modelsForType(type: ProviderType, typeProviders: Provider[]): ProviderModel[] {
    const ownerIds = new Set<string>([providerTypeAsCLIPROXY(type), ...typeProviders.map((p) => p.id)])
    const prefix = providerTypeAsPrefix(type)
    const seen = new Set<string>()
    const result: ProviderModel[] = []
    for (const model of models) {
      if (!ownerIds.has(model.owned_by)) continue
      if (!model.id.startsWith(prefix + '/')) continue
      if (seen.has(model.id)) continue
      seen.add(model.id)
      result.push(model)
    }
    return result.sort((a, b) => a.id.localeCompare(b.id, undefined, { sensitivity: 'base' }))
  }

  function minimalQuota(quotas: QuotaWindow[] | null | undefined): QuotaWindow | null {
    if (!quotas || quotas.length === 0) return null
    return quotas.reduce((min, q) => (q.usedPercent > min.usedPercent ? q : min))
  }

  function copyModelName(model: ProviderModel) {
    void onCopy(model.id, `Model ${model.id} copied.`)
  }
</script>

{#if providers.length}
  <div class="provider-toolbar">
    <button class="text-button" onclick={onReload} disabled={reloadPending}>
      <Icon name="refresh" size={17} />
      {reloadPending ? 'Reloading…' : 'Reload providers'}
    </button>
  </div>
  <div class="data-list provider-type-list" aria-label="Providers">
    {#each grouped() as [type, typeProviders] (type)}
      {@const expanded = expandedType === type}
      {@const typeModels = expanded ? modelsForType(type, typeProviders) : []}
      <article class:expanded class="provider-type-item">
        <button
          class="data-row provider-type-row"
          onclick={() => toggleType(type)}
          aria-expanded={expanded}
        >
          <ProviderIcon {type} size={44} />
          <div class="entity-main"><h2>{providerLabels[type]}</h2><p>{typeProviders.length} account{typeProviders.length === 1 ? '' : 's'}</p></div>
          <span class:expanded class="expand-icon" aria-hidden="true"><Icon name="chevron" size={19} /></span>
        </button>
        {#if expanded}
          <div class="provider-accounts">
            {#each typeProviders as provider (provider.id)}
              {@const providerExpanded = expandedProviderId === provider.id}
              <article class:provider-expanded={providerExpanded} class:provider-disabled={provider.disabled} class="provider-item">
                <div
                  class="data-row provider-row"
                  role="button"
                  tabindex="0"
                  aria-expanded={providerExpanded}
                  onclick={() => toggleProvider(provider)}
                  onkeydown={(event) => onProviderKeydown(event, provider)}
                >
                  <div class="entity-main"><h2>{provider.name}</h2><p><code>{provider.id}</code></p></div>
                  {#if provider.quota}
                    {@const lowestQuota = minimalQuota(provider.quota?.quotas)}
                    <div class="quota" title={quotaReset(lowestQuota)}>
                      <div><span style={`width:${Math.max(0, 100 - (lowestQuota?.usedPercent ?? 100))}%`}></span></div>
                      <small>{quotaLabel(lowestQuota)}</small>
                    </div>
                  {/if}
                  <span class:disabled={provider.disabled} class="status-dot"><i></i> {provider.disabled ? 'Disabled' : 'Connected'}</span>
                  <span class:expanded={providerExpanded} class="expand-icon" aria-hidden="true"><Icon name="chevron" size={19} /></span>
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
                {#if providerExpanded}
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
                        {@const resetLabel = quotaResetLabel(quota, ticker.current)}
                        <div class="quota-card">
                          <div class="quota-header">
                            <span class="quota-name" title={resetLabel}>{#if quota.name === ""}{resetLabel}{:else}{quota.name}{/if}</span>
                            <small>{quotaLabel(quota)}</small>
                          </div>
                          <div class="quota-progress-bar">
                            <span class="quota-progress-fill" style={`width: ${Math.max(0, 100 - (quota.usedPercent ?? 100))}%`}></span>
                          </div>
                        </div>
                      {/each}
                    </section>
                  {/if}
                {/if}
              </article>
            {/each}
            {#if typeModels.length}
              <div class="type-models">
                <h3>Available models</h3>
                <div class="models-list">
                  {#each typeModels as model (model.id)}
                    <button class="model-row" onclick={() => copyModelName(model)} title="Copy model name">
                      <code>{model.id}</code>
                      <Icon name="copy" size={16} />
                    </button>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
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
