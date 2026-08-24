<script lang="ts">
  import Icon from '../Icon.svelte'

  interface Props {
    value: string
    eyebrow?: string
    title?: string
    description?: string
    warning?: string
    onCopy: () => void
    onClose: () => void
  }

  let { value, eyebrow = 'KEY CREATED', title = 'Save your API key', description = 'This is the only time the full key will be shown.', warning = 'Store this key somewhere secure before closing.', onCopy, onClose }: Props = $props()
</script>

<div class="dialog-header">
  <div><p class="eyebrow">{eyebrow}</p><h2 id="dialog-title">{title}</h2><p>{description}</p></div>
</div>
<div class="dialog-body">
  <div class="secret-box"><code>{value}</code><button class="icon-button" onclick={onCopy} aria-label="Copy API key"><Icon name="copy" size={20} /></button></div>
  <div class="inline-message warning"><Icon name="warning" size={20} /><span>{warning}</span></div>
</div>
<div class="dialog-actions"><button class="filled" onclick={onClose}>I’ve saved it</button></div>
