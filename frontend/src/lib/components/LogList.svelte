<script lang="ts">
  import { formatLogTime } from '../format'
  import Icon from '../Icon.svelte'
  import type { LogEntry } from '../models/logentry'

  interface Props {
    logs: LogEntry[]
  }

  let { logs }: Props = $props()
</script>

{#if logs.length}
  <div class="log-list">
    {#each logs as entry}
      <article class:failed={entry.failed} class="log-row">
        <div><strong>{entry.model || 'Unknown model'}</strong><time>{formatLogTime(entry)}</time></div>
        <div class="log-provider"><strong>{entry.provider || 'Unknown provider'}</strong><code>{entry.apiKeyId || 'No key ID'}</code></div>
        <div class="token-breakdown">
          <span>{entry.inputTokens.toLocaleString()} input</span>
          <span>{entry.outputTokens.toLocaleString()} output</span>
          <strong>{entry.totalTokens.toLocaleString()} total</strong>
        </div>
        <span>{entry.failed ? 'Failed' : 'Completed'}</span>
      </article>
    {/each}
  </div>
{:else}
  <div class="state-panel empty-state compact">
    <div class="state-icon"><Icon name="logs" /></div>
    <div><h2>No logs to show</h2><p>New request records will appear here after traffic reaches the router.</p></div>
  </div>
{/if}
