<script lang="ts">
  import { formatDateTime, formatLogTime, formatTokens } from '../format'
  import Icon from '../Icon.svelte'
  import { quotaResetLabels, type ApiKey, type ApiKeyUsage } from '../models/apikey'

  interface Props {
    keys: ApiKey[]
    search: string
    usage: Record<string, ApiKeyUsage>
    usageLoading: string
    usageErrors: Record<string, string>
    onClearSearch: () => void
    onCreate: () => void
    onEdit: (key: ApiKey) => void
    onDelete: (key: ApiKey) => void
    onLoadUsage: (key: ApiKey) => Promise<void>
  }

  let { keys, search, usage, usageLoading, usageErrors, onClearSearch, onCreate, onEdit, onDelete, onLoadUsage }: Props = $props()
  let expandedId = $state('')

  async function toggle(key: ApiKey) {
    if (expandedId === key.id) {
      expandedId = ''
      return
    }

    expandedId = key.id
    if (!usage[key.id]) await onLoadUsage(key)
  }

  function onKeydown(event: KeyboardEvent, key: ApiKey) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    void toggle(key)
  }
</script>

{#if keys.length}
  <div class="data-list" aria-label="API keys">
    {#each keys as key (key.id)}
      <article class:key-expanded={expandedId === key.id} class="key-item">
        <div
          class="data-row key-row"
          role="button"
          tabindex="0"
          aria-expanded={expandedId === key.id}
          onclick={() => toggle(key)}
          onkeydown={(event) => onKeydown(event, key)}
        >
          <div class="entity-icon key-icon"><Icon name="key" /></div>
          <div class="entity-main"><h2>{key.name}</h2><p><code>{key.id}</code></p></div>
          {#if key.isAdmin === 'true'}<span class="status-chip">Admin</span>{/if}
          <span class:inactive={!key.isActive} class="status-chip key-status">{key.isActive ? 'Active' : 'Inactive'}</span>
          <span class:expanded={expandedId === key.id} class="expand-icon" aria-hidden="true"><Icon name="chevron" size={19} /></span>
          <button
            class="icon-button danger"
            onclick={(event) => { event.stopPropagation(); onDelete(key) }}
            aria-label={`Delete ${key.name}`}
          ><Icon name="trash" size={20} /></button>
        </div>

        {#if expandedId === key.id}
          <section class="key-details" aria-label={`${key.name} usage details`}>
            <div class="key-quota-summary">
              <div><span>Quota</span><strong>{key.quotaSet ? formatTokens(key.reservedTokens) : 'Unlimited'}</strong></div>
              <div><span>Used</span><strong>{formatTokens(key.quotaUsed)}</strong></div>
              <div><span>Resets</span><strong>{key.quotaSet && key.quotaResetStrategy !== 'QUOTA_RESET_STRATEGY_FOREVER' ? formatDateTime(key.quotaResetAt) : 'Never'}</strong></div>
              <div><span>Frequency</span><strong>{key.quotaSet ? quotaResetLabels[key.quotaResetStrategy] : 'No limit'}</strong></div>
              <button class="tonal" onclick={() => onEdit(key)}>Edit</button>
            </div>
            {#if usageLoading === key.id}
              <div class="details-loading"><span class="dark-spinner"></span> Loading usage…</div>
            {:else if usageErrors[key.id]}
              <div class="details-error" role="alert">
                <Icon name="warning" size={19} /><span>{usageErrors[key.id]}</span>
                <button class="text-button" onclick={() => onLoadUsage(key)}>Retry</button>
              </div>
            {:else if usage[key.id]}
              <div class="usage-summary">
                <div><span>Recent requests</span><strong>{usage[key.id].logs.length} of 10</strong></div>
              </div>
              {#if usage[key.id].logs.length}
                <div class="key-log-list">
                  {#each usage[key.id].logs as entry}
                    <div class:failed={entry.failed} class="key-log-row">
                      <div><strong>{entry.model || 'Unknown model'}</strong><time>{formatLogTime(entry)}</time></div>
                      <span>{entry.provider || 'Unknown provider'}</span>
                      <span>{formatTokens(entry.totalTokens)}</span>
                      <span class="request-status">{entry.failed ? 'Failed' : 'Completed'}</span>
                    </div>
                  {/each}
                </div>
              {:else}
                <p class="details-empty">No requests recorded for this key.</p>
              {/if}
            {/if}
          </section>
        {/if}
      </article>
    {/each}
  </div>
{:else}
  <div class="state-panel empty-state">
    <div class="state-icon"><Icon name="key" /></div>
    <div>
      <h2>{search ? 'No matching keys' : 'No API keys yet'}</h2>
      <p>{search ? 'Try a different name or clear your search.' : 'Create a key to let an application use this router.'}</p>
    </div>
    <button class="tonal" onclick={search ? onClearSearch : onCreate}>{search ? 'Clear search' : 'Create key'}</button>
  </div>
{/if}
