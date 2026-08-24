<script lang="ts">
  import type { ProviderType } from '../models/providers'

  interface Props {
    type: ProviderType
    size?: number
  }

  let { type, size = 40 }: Props = $props()

  interface IconDef {
    src: string
    label: string
    invertInDark?: boolean
  }

  const icons: Record<ProviderType, IconDef> = {
    PROVIDER_TYPE_ANTHROPIC: { src: '/icons/anthropic.svg', label: 'A' },
    PROVIDER_TYPE_OPENAI: { src: '/icons/openai.svg', label: 'O' },
    PROVIDER_TYPE_GROK: { src: '/icons/grok.svg', label: 'G', invertInDark: true },
    PROVIDER_TYPE_ANTIGRAVITY: { src: '/icons/google.svg', label: 'G' },
    PROVIDER_TYPE_KIMI: { src: '/icons/kimi-ai.svg', label: 'K', invertInDark: true },
    PROVIDER_TYPE_OPENCODE_ZEN: { src: '/icons/opencode.svg', label: 'Z', invertInDark: true },
    PROVIDER_TYPE_OPENCODE_GO: { src: '/icons/opencode.svg', label: 'G', invertInDark: true },
    PROVIDER_TYPE_CLINE: { src: '/icons/cline.svg', label: 'C', invertInDark: true },
    PROVIDER_TYPE_CURSOR: { src: '/icons/cursor.svg', label: 'C', invertInDark: true },
    PROVIDER_TYPE_CUSTOM: { src: '', label: 'C' },
  }

  const styles: Record<ProviderType, { bg: string; fg: string }> = {
    PROVIDER_TYPE_CUSTOM: { bg: '#74777f', fg: '#ffffff' },
    PROVIDER_TYPE_OPENAI: { bg: '#10a37f', fg: '#ffffff' },
    PROVIDER_TYPE_ANTHROPIC: { bg: '#d4a27f', fg: '#1f1f1f' },
    PROVIDER_TYPE_KIMI: { bg: '#e8e8e8', fg: '#2b2a29' },
    PROVIDER_TYPE_GROK: { bg: '#e8e8e8', fg: '#000000' },
    PROVIDER_TYPE_ANTIGRAVITY: { bg: '#ffffff', fg: '#4285f4' },
    PROVIDER_TYPE_OPENCODE_ZEN: { bg: '#211e1e', fg: '#cfcecd' },
    PROVIDER_TYPE_OPENCODE_GO: { bg: '#211e1e', fg: '#cfcecd' },
    PROVIDER_TYPE_CLINE: { bg: '#6366f1', fg: '#ffffff' },
    PROVIDER_TYPE_CURSOR: { bg: '#18181b', fg: '#ffffff' },
  }

  const icon = $derived(icons[type] ?? icons.PROVIDER_TYPE_CUSTOM)
  const style = $derived(styles[type] ?? styles.PROVIDER_TYPE_CUSTOM)
</script>

<span
  class="provider-icon-m3"
  class:invert-dark={icon.invertInDark}
  style:width={`${size}px`}
  style:height={`${size}px`}
  style:background={style.bg}
  style:color={style.fg}
  aria-hidden="true"
>
  {#if icon.src}
    <img src={icon.src} alt="" width={size} height={size} />
  {:else}
    <span class="fallback">{icon.label}</span>
  {/if}
</span>

<style>
  .provider-icon-m3 {
    display: grid;
    place-items: center;
    flex: 0 0 auto;
    overflow: hidden;
    border-radius: 28%;
    box-shadow:
      0 1px 2px rgba(0, 0, 0, 0.08),
      inset 0 1px 0 rgba(255, 255, 255, 0.15);
  }
  .provider-icon-m3 img {
    width: 70%;
    height: 70%;
    object-fit: contain;
  }
  :global([data-theme="dark"]) .provider-icon-m3.invert-dark img {
    filter: invert(1);
  }
  .fallback {
    font-family: "Manrope", sans-serif;
    font-size: 45%;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
</style>
