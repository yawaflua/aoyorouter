<script lang="ts">
  let { name, size = 24 }: { name: string; size?: number } = $props()
</script>

<svg
  class="icon"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === 'key'}
    <circle cx="8" cy="15" r="4"></circle><path d="m11 12 8-8m-2 2 2 2m-5 1 2 2"></path>
  {:else if name === 'provider'}
    <path d="M8 3v4m8-4v4M6 7h12v3a6 6 0 0 1-12 0V7Zm6 9v5"></path>
  {:else if name === 'proxy'}
    <circle cx="6" cy="12" r="2.5"></circle><circle cx="18" cy="6" r="2.5"></circle><circle cx="18" cy="18" r="2.5"></circle><path d="m8.2 10.8 7.6-3.6m-7.6 6 7.6 3.6"></path>
  {:else if name === 'logs'}
    <path d="M5 4h14v16H5zM8 8h8m-8 4h8m-8 4h5"></path>
  {:else if name === 'plus'}
    <path d="M12 5v14M5 12h14"></path>
  {:else if name === 'trash'}
    <path d="M4 7h16m-10 4v5m4-5v5M9 7l1-3h4l1 3m3 0-1 13H7L6 7"></path>
  {:else if name === 'search'}
    <circle cx="11" cy="11" r="6"></circle><path d="m16 16 4 4"></path>
  {:else if name === 'copy'}
    <rect x="8" y="8" width="11" height="11" rx="2"></rect><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"></path>
  {:else if name === 'logout'}
    <path d="M10 5H5v14h5m4-3 4-4-4-4m4 4H9"></path>
  {:else if name === 'refresh'}
    <path d="M20 7v5h-5M4 17v-5h5m9.5-4A8 8 0 0 0 5 9m.5 7A8 8 0 0 0 19 15"></path>
  {:else if name === 'check'}
    <path d="m5 12 4 4L19 6"></path>
  {:else if name === 'eye'}
    <path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12Z"></path><circle cx="12" cy="12" r="2.5"></circle>
  {:else if name === 'eye-off'}
    <path d="m3 3 18 18M10.5 6.2A10.8 10.8 0 0 1 12 6c6.5 0 10 6 10 6a15 15 0 0 1-2 2.7M6.3 6.3C3.5 8.1 2 12 2 12s3.5 6 10 6a10 10 0 0 0 3.7-.7"></path>
  {:else if name === 'external'}
    <path d="M14 4h6v6m0-6-9 9M9 5H5v14h14v-4"></path>
  {:else if name === 'chevron'}
    <path d="m9 18 6-6-6-6"></path>
  {:else if name === 'shield'}
    <path d="M12 3 5 6v5c0 4.8 3 8.2 7 10 4-1.8 7-5.2 7-10V6l-7-3Z"></path><path d="m9 12 2 2 4-4"></path>
  {:else if name === 'warning'}
    <path d="M12 4 3 20h18L12 4Zm0 6v4m0 3h.01"></path>
  {:else}
    <circle cx="12" cy="12" r="9"></circle><path d="M12 8v5m0 3h.01"></path>
  {/if}
</svg>
