<script lang="ts">
  import { formatDateTime } from '../format'
  import Icon from '../Icon.svelte'
  import type { ErrorLog } from '../models/errorlog'

  interface Props {
    errors: ErrorLog[]
  }

  let { errors }: Props = $props()
  let expandedId = $state<string | null>(null)

  const sortedErrors = $derived(
    [...errors].sort((a, b) => b.timestamp.localeCompare(a.timestamp)),
  )

  function toggle(id: string) {
    expandedId = expandedId === id ? null : id
  }

  function formatJson(value: string): string {
    if (!value) return ''
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }
</script>

{#if sortedErrors.length}
  <div class="error-list">
    {#each sortedErrors as entry (entry.id)}
      <article class="error-item" class:error-expanded={expandedId === entry.id}>
        <button class="error-row" onclick={() => toggle(entry.id)}>
          <div class="expand-icon" class:expanded={expandedId === entry.id}>
            <Icon name="chevron" size={18} />
          </div>
          <div class="error-main">
            <h2>{entry.method} {entry.url}</h2>
            <p>{entry.id}</p>
          </div>
          <time>{formatDateTime(entry.timestamp)}</time>
          <span class="error-status" class:client-error={entry.statusCode >= 400 && entry.statusCode < 500} class:server-error={entry.statusCode >= 500}>
            {entry.statusCode || '—'}
          </span>
        </button>
        {#if expandedId === entry.id}
          <div class="error-details">
            <div class="error-block">
              <h3>Request body</h3>
              <pre class="code-block">{formatJson(entry.body) || 'null'}</pre>
            </div>
            <div class="error-block">
              <h3>Response body</h3>
              <pre class="code-block">{formatJson(entry.responseBody) || 'null'}</pre>
            </div>
            <div class="error-block">
              <h3>Headers</h3>
              <ul class="header-list">
                {#each entry.headers as header}
                  <li><code>{header}</code></li>
                {:else}
                  <li class="empty">No headers</li>
                {/each}
              </ul>
            </div>
            <div class="error-block compact">
              <h3>Request time</h3>
              <p>{formatDateTime(entry.timestamp)}</p>
            </div>
            <div class="error-block compact">
              <h3>Status</h3>
              <p>{entry.statusCode || '—'}</p>
            </div>
          </div>
        {/if}
      </article>
    {/each}
  </div>
{:else}
  <div class="state-panel empty-state compact">
    <div class="state-icon"><Icon name="warning" /></div>
    <div><h2>No errors to show</h2><p>Failed request records will appear here.</p></div>
  </div>
{/if}
