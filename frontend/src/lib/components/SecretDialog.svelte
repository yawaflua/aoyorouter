<script lang="ts">
  import Icon from '../Icon.svelte'

  interface Props {
    value: string
    onCopy: () => void
    onClose: () => void
  }

  let { value, onCopy, onClose }: Props = $props()
</script>

<div class="dialog-header">
  <div><p class="eyebrow">KEY CREATED</p><h2 id="dialog-title">Save your API key</h2><p>This is the only time the full key will be shown.</p></div>
</div>
<div class="dialog-body">
  <div class="secret-box"><code>{value}</code><button class="icon-button" onclick={onCopy} aria-label="Copy API key"><Icon name="copy" size={20} /></button></div>
  <div class="inline-message warning"><Icon name="warning" size={20} /><span>Store this key somewhere secure before closing.</span></div>
</div>
<div class="dialog-actions"><button class="filled" onclick={onClose}>I’ve saved it</button></div>
