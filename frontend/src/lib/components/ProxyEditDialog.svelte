<script lang="ts">
  import { untrack } from 'svelte'
  import { providerOptions } from '../app'
  import Icon from '../Icon.svelte'
  import type { Provider, UpdateProviderInput } from '../models/providers'
  import ProxySettings from './ProxySettings.svelte'
    import type { Endpoint, LiveProxy } from '../models/liveproxy';

  interface Props {
    proxy: LiveProxy
    availableEndpoints: Endpoint[]
    pending: boolean
    error: string
    onSubmit: (input: { id: string, endpoint: string, newEndpoint: string }) => Promise<void>
    onClose: () => void
  }

  let { proxy, availableEndpoints, pending, error, onSubmit, onClose }: Props = $props()
  const initial = untrack(() => proxy)
  let name = $state(initial.name)
  let cloudflareAddress = $state(initial.cloudflareAddress)
  let customUrl = $state(initial.url)

  function submit(event: SubmitEvent) {
    event.preventDefault()
    void onSubmit({ id: initial.id, endpoint: initial.cloudflareAddress, newEndpoint: cloudflareAddress })
  }
</script>

<form onsubmit={submit}>
  <div class="dialog-header">
    <div><p class="eyebrow">PROXY SETTINGS</p><h2 id="dialog-title">Edit proxy</h2><p>Change cloudflare address.</p></div>
    <button type="button" class="close-button" onclick={onClose} aria-label="Close">×</button>
  </div>
  <div class="dialog-body provider-form">
    <div class="form-grid">
      <div class="field-group"><label for="edit-proxy-name">{name}</label><div class="text-field"></div></div>
      <div class="field-group"><label for="edit-proxy-type">Proxy type</label><div class="select-field"><select id="edit-proxy-type" bind:value={cloudflareAddress} disabled={pending}>{#each availableEndpoints  as option}<option value={option.addr}>{option.addr} - {option.rtt}S</option>{/each}</select></div></div>
    </div>
    
    {#if error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{error}</p>{/if}
  </div>
  <div class="dialog-actions"><button type="button" class="text-button" onclick={onClose}>Cancel</button><button class="filled" disabled={pending}>{pending ? 'Saving…' : 'Save changes'}</button></div>
</form>
