<script lang="ts">
  import Icon from '../Icon.svelte'

  interface Props {
    entity: 'API key' | 'provider'
    name: string
    pending: boolean
    error: string
    onDelete: () => Promise<void>
    onClose: () => void
  }

  let { entity, name, pending, error, onDelete, onClose }: Props = $props()
</script>

<div class="dialog-header destructive">
  <div class="state-icon"><Icon name="trash" /></div>
  <div><h2 id="dialog-title">Delete {entity}?</h2><p>This action cannot be undone.</p></div>
</div>
<div class="dialog-body">
  <p class="confirmation-copy">
    {entity === 'API key' ? `Applications using “${name}” will immediately lose access.` : `Requests can no longer be routed through “${name}”.`}
  </p>
  {#if error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{error}</p>{/if}
</div>
<div class="dialog-actions">
  <button class="text-button" onclick={onClose}>Cancel</button>
  <button class="danger-button" onclick={onDelete} disabled={pending}>{pending ? 'Deleting…' : 'Delete'}</button>
</div>
