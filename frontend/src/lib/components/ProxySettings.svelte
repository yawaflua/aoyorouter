<script lang="ts">
  interface Props {
    useProxy: boolean
    proxy: string
    idPrefix: string
    disabled?: boolean
  }

  let { useProxy = $bindable(), proxy = $bindable(), idPrefix, disabled = false }: Props = $props()
</script>

<div class="proxy-settings">
  <label class="switch-row" for={`${idPrefix}-use-proxy`}>
    <span><strong>Use proxy</strong><small>Route provider traffic through a proxy.</small></span>
    <input id={`${idPrefix}-use-proxy`} type="checkbox" bind:checked={useProxy} {disabled} /><i></i>
  </label>

  {#if useProxy}
    <div class="field-group">
      <label for={`${idPrefix}-proxy-url`}>Proxy URL <span>Optional</span></label>
      <div class="text-field">
        <input id={`${idPrefix}-proxy-url`} bind:value={proxy} disabled={disabled} placeholder="http://127.0.0.1:5555" spellcheck="false" />
      </div>
      <p class="supporting">Leave empty to let backend create Cloudflare WARP. Supports http://, https://, socks://, socks5://, and socks5h://.</p>
    </div>
  {/if}
</div>
