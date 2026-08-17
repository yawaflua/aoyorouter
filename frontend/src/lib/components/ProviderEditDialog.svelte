<script lang="ts">
  import { untrack } from 'svelte'
  import { providerOptions } from '../app'
  import Icon from '../Icon.svelte'
  import type { Provider, UpdateProviderInput } from '../models/providers'
  import ProxySettings from './ProxySettings.svelte'

  interface Props {
    provider: Provider
    pending: boolean
    error: string
    onSubmit: (input: UpdateProviderInput) => Promise<void>
    onClose: () => void
  }

  let { provider, pending, error, onSubmit, onClose }: Props = $props()
  const initial = untrack(() => provider)
  let name = $state(initial.name)
  let type = $state(initial.type)
  let customUrl = $state(initial.customUrl)
  let authorizationData = $state(initial.clientSecret)
  let useProxy = $state(initial.useProxy)
  let proxy = $state(initial.proxy)

  function submit(event: SubmitEvent) {
    event.preventDefault()
    void onSubmit({ id: provider.id, name, type, customUrl, authorizationData, useProxy, proxy })
  }
</script>

<form onsubmit={submit}>
  <div class="dialog-header">
    <div><p class="eyebrow">PROVIDER SETTINGS</p><h2 id="dialog-title">Edit provider</h2><p>Change connection and proxy settings.</p></div>
    <button type="button" class="close-button" onclick={onClose} aria-label="Close">×</button>
  </div>
  <div class="dialog-body provider-form">
    <div class="form-grid">
      <div class="field-group"><label for="edit-provider-name">Provider name</label><div class="text-field"><input id="edit-provider-name" bind:value={name} disabled={pending} /></div></div>
      <div class="field-group"><label for="edit-provider-type">Provider type</label><div class="select-field"><select id="edit-provider-type" bind:value={type} disabled={pending}>{#each providerOptions as option}<option value={option.value}>{option.label}</option>{/each}</select></div></div>
    </div>
    <div class="field-group">
      <label for="edit-custom-url">Custom URL <span>Optional</span></label>
      <div class="text-field"><input id="edit-custom-url" bind:value={customUrl} disabled={pending} placeholder="https://api.example.com/v1" /></div>
    </div>
    <div class="field-group">
      <label for="edit-credentials">Authorization data</label>
      <textarea id="edit-credentials" bind:value={authorizationData} disabled={pending} spellcheck="false"></textarea>
      <p class="supporting">Current backend value. Required by update API.</p>
    </div>
    <ProxySettings bind:useProxy bind:proxy idPrefix="edit-provider" disabled={pending} />
    {#if error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{error}</p>{/if}
  </div>
  <div class="dialog-actions"><button type="button" class="text-button" onclick={onClose}>Cancel</button><button class="filled" disabled={pending}>{pending ? 'Saving…' : 'Save changes'}</button></div>
</form>
