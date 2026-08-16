<script lang="ts">
  import Icon from '../Icon.svelte'

  interface Props {
    pending: boolean
    error: string
    onSubmit: (name: string, admin: boolean) => Promise<void>
    onClose: () => void
  }

  let { pending, error, onSubmit, onClose }: Props = $props()
  let name = $state('')
  let admin = $state(false)

  function submit(event: SubmitEvent) {
    event.preventDefault()
    void onSubmit(name, admin)
  }
</script>

<form onsubmit={submit}>
  <div class="dialog-header">
    <div><p class="eyebrow">NEW CREDENTIAL</p><h2 id="dialog-title">Create API key</h2><p>The key is shown once after creation.</p></div>
    <button type="button" class="close-button" onclick={onClose} aria-label="Close">×</button>
  </div>
  <div class="dialog-body">
    <div class="field-group">
      <label for="key-name">Key name</label>
      <div class="text-field"><input id="key-name" bind:value={name} placeholder="Production app" /></div>
      <p class="supporting">Use a name that identifies the application.</p>
    </div>
    <label class="switch-row">
      <span><strong>Administrator access</strong><small>Allows this key to manage router resources.</small></span>
      <input type="checkbox" bind:checked={admin} /><i></i>
    </label>
    {#if error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{error}</p>{/if}
  </div>
  <div class="dialog-actions">
    <button type="button" class="text-button" onclick={onClose}>Cancel</button>
    <button class="filled" disabled={pending}>{pending ? 'Creating…' : 'Create key'}</button>
  </div>
</form>
