<script lang="ts">
  import { untrack } from 'svelte'
  import Icon from '../Icon.svelte'
  import { quotaResetOptions, type ApiKey, type UpdateApiKeyInput } from '../models/apikey'

  interface Props {
    apiKey: ApiKey
    pending: boolean
    error: string
    onSubmit: (input: UpdateApiKeyInput) => Promise<void>
    onClose: () => void
  }

  let { apiKey, pending, error, onSubmit, onClose }: Props = $props()
  const initial = untrack(() => apiKey)
  let name = $state(initial.name)
  let isActive = $state(initial.isActive)
  let quotaSet = $state(initial.quotaSet)
  let reservedMillions = $state(initial.reservedTokens ? initial.reservedTokens / 1_000_000 : 1)
  let quotaResetStrategy = $state(initial.quotaResetStrategy)

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
    {#if error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{error}</p>{/if}
  </div>
  <div class="dialog-actions"><button type="button" class="text-button" onclick={onClose}>Cancel</button><button class="filled" disabled={pending}>{pending ? 'Saving…' : 'Save changes'}</button></div>
</form>
