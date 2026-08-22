<script lang="ts">
  import { untrack } from 'svelte'
  import { providerLabels } from '../app'
  import Icon from '../Icon.svelte'
  import { quotaResetOptions, type ApiKey, type UpdateApiKeyInput } from '../models/apikey'
  import type { Provider, ProviderModel } from '../models/providers'

  interface Props {
    apiKey: ApiKey
    pending: boolean
    error: string
    providers: Provider[]
    models: ProviderModel[]
    onSubmit: (input: UpdateApiKeyInput) => Promise<void>
    onClose: () => void
  }

  let { apiKey, pending, error, providers, models, onSubmit, onClose }: Props = $props()
  const initial = untrack(() => apiKey)
  let name = $state(initial.name)
  let isActive = $state(initial.isActive)
  let quotaSet = $state(initial.quotaSet)
  let reservedMillions = $state(initial.reservedTokens ? initial.reservedTokens / 1_000_000 : 1)
  let quotaResetStrategy = $state(initial.quotaResetStrategy)
  let restrictedProviders = $state([...initial.restrictedProviders])
  let restrictedModels = $state([...initial.restrictedModels])
  let modelSearch = $state('')

  const availableModels = $derived.by(() => {
    const unique = new Map<string, ProviderModel>()
    for (const model of models) {
      const id = model.id.trim()
      if (id && !unique.has(id)) unique.set(id, model)
    }
    return [...unique.values()].sort((left, right) => left.id.localeCompare(right.id))
  })
  const visibleModels = $derived(
    availableModels.filter((model) => `${model.id} ${model.owned_by}`.toLowerCase().includes(modelSearch.trim().toLowerCase())),
  )
  const missingProviders = $derived(restrictedProviders.filter((id) => !providers.some((provider) => provider.id === id)))
  const missingModels = $derived(restrictedModels.filter((id) => !availableModels.some((model) => model.id === id)))

  function toggleProvider(id: string) {
    restrictedProviders = toggleValue(restrictedProviders, id)
  }

  function toggleModel(id: string) {
    restrictedModels = toggleValue(restrictedModels, id)
  }

  function toggleValue(values: string[], value: string): string[] {
    return values.includes(value) ? values.filter((item) => item !== value) : [...values, value]
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    void onSubmit({
      id: apiKey.id,
      name,
      isAdmin: apiKey.isAdmin,
      isActive,
      quotaSet,
      reservedTokens: quotaSet ? Math.round(Number(reservedMillions) * 1_000_000) : 0,
      quotaResetAt: quotaResetStrategy === initial.quotaResetStrategy ? apiKey.quotaResetAt : '',
      quotaResetStrategy: quotaSet ? quotaResetStrategy : 'QUOTA_RESET_STRATEGY_FOREVER',
      restrictedProviders,
      restrictedModels,
    })
  }
</script>

<form onsubmit={submit}>
  <div class="dialog-header">
    <div><p class="eyebrow">API KEY SETTINGS</p><h2 id="dialog-title">Edit API key</h2><p>Change access status and token quota.</p></div>
    <button type="button" class="close-button" onclick={onClose} aria-label="Close">×</button>
  </div>
  <div class="dialog-body key-edit-form">
    <div class="field-group"><label for="edit-key-name">Key name</label><div class="text-field"><input id="edit-key-name" bind:value={name} disabled={pending} /></div></div>

    <label class="switch-row" for="edit-key-active">
      <span><strong>Active</strong><small>Inactive keys cannot authenticate requests.</small></span>
      <input id="edit-key-active" type="checkbox" bind:checked={isActive} disabled={pending} /><i></i>
    </label>
    <label class="switch-row" for="edit-key-quota">
      <span><strong>Token quota</strong><small>Limit tokens available during each reset period.</small></span>
      <input id="edit-key-quota" type="checkbox" bind:checked={quotaSet} disabled={pending} /><i></i>
    </label>

    {#if quotaSet}
      <div class="form-grid quota-fields">
        <div class="field-group">
          <label for="reserved-millions">Quota <span>Millions of tokens</span></label>
          <div class="text-field"><input id="reserved-millions" type="number" min="0.001" step="0.001" bind:value={reservedMillions} disabled={pending} /></div>
          <p class="supporting">Sent as {Number.isFinite(Number(reservedMillions)) ? Math.round(Number(reservedMillions) * 1_000_000).toLocaleString() : 0} tokens.</p>
        </div>
        <div class="field-group">
          <label for="quota-reset-strategy">Reset frequency</label>
          <div class="select-field"><select id="quota-reset-strategy" bind:value={quotaResetStrategy} disabled={pending}>{#each quotaResetOptions as option}<option value={option.value}>{option.label}</option>{/each}</select></div>
        </div>
      </div>
    {/if}

    <div class="form-grid access-restrictions">
      <div class="field-group">
        <div class="restriction-label" id="restricted-providers-label">Blocked providers <span>{restrictedProviders.length} selected</span></div>
        <div class="restriction-list" role="group" aria-labelledby="restricted-providers-label">
          {#each providers as provider (provider.id)}
            <label class="restriction-option">
              <input type="checkbox" checked={restrictedProviders.includes(provider.id)} disabled={pending} onchange={() => toggleProvider(provider.id)} />
              <span><strong>{provider.name}</strong><small>{providerLabels[provider.type]}{provider.disabled ? ' · Disabled' : ''}</small></span>
            </label>
          {/each}
          {#each missingProviders as providerId (providerId)}
            <label class="restriction-option unavailable">
              <input type="checkbox" checked disabled={pending} onchange={() => toggleProvider(providerId)} />
              <span><strong>{providerId}</strong><small>Unavailable provider</small></span>
            </label>
          {/each}
          {#if !providers.length && !missingProviders.length}<p class="restriction-empty">No providers available.</p>{/if}
        </div>
        <p class="supporting">Selected providers are removed before round-robin selection.</p>
      </div>
      <div class="field-group">
        <label for="restricted-model-search">Blocked models <span>{restrictedModels.length} selected</span></label>
        <div class="text-field restriction-search"><Icon name="search" size={18} /><input id="restricted-model-search" bind:value={modelSearch} disabled={pending} placeholder="Search /v1/models" /></div>
        <div class="restriction-list models" role="group" aria-label="Blocked models">
          {#each visibleModels as model (model.id)}
            <label class="restriction-option">
              <input type="checkbox" checked={restrictedModels.includes(model.id)} disabled={pending} onchange={() => toggleModel(model.id)} />
              <span><strong>{model.id}</strong><small>{model.owned_by || 'Unknown provider'}</small></span>
            </label>
          {/each}
          {#each missingModels as modelId (modelId)}
            <label class="restriction-option unavailable">
              <input type="checkbox" checked disabled={pending} onchange={() => toggleModel(modelId)} />
              <span><strong>{modelId}</strong><small>Unavailable model</small></span>
            </label>
          {/each}
          {#if !visibleModels.length && !missingModels.length}<p class="restriction-empty">No matching models.</p>{/if}
        </div>
        <p class="supporting">Models are loaded from <code>/v1/models</code>.</p>
      </div>
    </div>
    {#if error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{error}</p>{/if}
  </div>
  <div class="dialog-actions"><button type="button" class="text-button" onclick={onClose}>Cancel</button><button class="filled" disabled={pending}>{pending ? 'Saving…' : 'Save changes'}</button></div>
</form>
