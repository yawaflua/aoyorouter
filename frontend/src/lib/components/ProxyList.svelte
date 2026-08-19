<script lang="ts">
  import Icon from '../Icon.svelte'
  import type { LiveProxy } from '../models/liveproxy'

  interface Props {
    proxies: LiveProxy[]
    search: string
    onClearSearch: () => void
    onEdit: (proxy: LiveProxy) => void
    onCopy: (value: string, message: string) => void
  }

  let { proxies, search, onClearSearch, onEdit, onCopy }: Props = $props()
  let expandedId = $state('')

  function toggle(provider: LiveProxy) {
    expandedId = expandedId === provider.id ? '' : provider.id
  }
  $effect(() => {

  })

  function onKeydown(event: KeyboardEvent, provider: LiveProxy) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    toggle(provider)
  }
</script>

{#if proxies.length}
  <div class="data-list" aria-label="Live proxies">
    {#each proxies as proxy (proxy.id)}
      <article class:proxy-expanded={expandedId === proxy.id} class="provider-item">
        <div class="data-row provider-row" role="button" tabindex="0" aria-expanded={expandedId === proxy.id} onclick={() => toggle(proxy)} onkeydown={(event) => onKeydown(event, proxy)}>
            <div class="entity-icon proxy-icon"><Icon name="proxy" /></div>
            <div class="entity-main"><h2>{proxy.name || proxy.id}</h2><p><code>{proxy.id}</code></p></div>
            <div class="proxy-addresses">
            <span><small>Local</small><code>{proxy.url}</code></span>
            {#if proxy.cloudflareAddress}<span><small>Cloudflare</small><code>{proxy.cloudflareAddress}</code></span>{/if}
            </div>
            <span class="status-dot"><i></i> Live</span>
            <button class="icon-button" onclick={() => onCopy(proxy.url, 'Proxy address copied.')} aria-label={`Copy ${proxy.name || proxy.id} address`}>
            <Icon name="copy" size={20} />
            </button>
        </div>
        {#if expandedId === proxy.id}
        <div class="provider-details" aria-label={`Proxy settings`}>
            <dl>
                <div><dt>Details</dt><dd><code>{proxy.id}</code></dd></div>
                <div><dt>Local</dt><dd><code>{proxy.url}</code></dd></div>
                {#if proxy.cloudflareAddress}<div><dt>Cloudflare IP</dt><dd><code>{proxy.cloudflareAddress}</code></dd></div>{/if}
                {#if proxy.warpInfo}<div><dt>Warp</dt><dd><code>City: {proxy.warpInfo.serverCity}<br>Location: {proxy.warpInfo.serverLocation}<br>IP: {proxy.warpInfo.ip}<br>HTTP: {proxy.warpInfo.httpType}<br>TLS: {proxy.warpInfo.tls}</code></dd></div>{/if}
            </dl>
        </div>
        {/if}

      </article>

    {/each}
  </div>
{:else}
  <div class="state-panel empty-state">
    <div class="state-icon"><Icon name="proxy" /></div>
    <div>
      <h2>{search ? 'No matching proxies' : 'No live proxies'}</h2>
      <p>{search ? 'Try another name, ID, or address.' : 'Managed proxy instances will appear here while they are running.'}</p>
    </div>
    {#if search}<button class="tonal" onclick={onClearSearch}>Clear search</button>{/if}
  </div>
{/if}
