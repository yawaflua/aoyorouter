<script lang="ts">
  import { providerLabels, providerOptions } from '../app'
  import Icon from '../Icon.svelte'
  import ProxySettings from './ProxySettings.svelte'
  import type { CodexAuthorization, ProviderAuthorization } from '../models/authorization'
  import type { ProviderType } from '../models/providers'

  export interface ProviderDraft {
    name: string
    type: ProviderType
    customUrl: string
    authorizationData: string
    useProxy: boolean
    proxy: string
    codexSession: CodexAuthorization | null
    providerSession: ProviderAuthorization | null
  }

  interface Props {
    pending: boolean
    error: string
    onGenerateCodex: (draft: ProviderDraft) => Promise<CodexAuthorization>
    onGenerateOAuth: (draft: ProviderDraft) => Promise<ProviderAuthorization>
    onCheckOAuth: (draft: ProviderDraft, callbackSubmitted: boolean) => Promise<boolean>
    onSubmit: (draft: ProviderDraft) => Promise<void>
    onCopy: (value: string, message: string) => void
    onClose: () => void
  }

  let { pending, error, onGenerateCodex, onGenerateOAuth, onCheckOAuth, onSubmit, onCopy, onClose }: Props = $props()

  let name = $state('')
  let type = $state<ProviderType>('PROVIDER_TYPE_OPENAI')
  let customUrl = $state('')
  let authorizationData = $state('')
  let useProxy = $state(false)
  let proxy = $state('')
  let codexSession = $state<CodexAuthorization | null>(null)
  let providerSession = $state<ProviderAuthorization | null>(null)
  let codexReady = $state(false)
  let callbackSubmitted = $state(false)
  let localError = $state('')

  const draft = $derived<ProviderDraft>({ name, type, customUrl, authorizationData, useProxy, proxy, codexSession, providerSession })
  const canSubmit = $derived(
    !pending && (type === 'PROVIDER_TYPE_OPENAI' ? codexReady : type === 'PROVIDER_TYPE_CUSTOM' && Boolean(authorizationData.trim())),
  )

  function resetAuthorization() {
    authorizationData = ''
    codexSession = null
    providerSession = null
    codexReady = false
    callbackSubmitted = false
    localError = ''
  }

  async function generateCodex() {
    try {
      codexSession = await onGenerateCodex(draft)
    } catch {
      // Parent exposes request error through error prop.
    }
  }

  function validateCodexCallback() {
    localError = ''
    try {
      if (!codexSession) throw new Error('Generate a new authorization link first.')
      const callback = new URL(authorizationData)
      if (!callback.searchParams.get('code')) throw new Error('The callback URL does not contain an authorization code.')
      if (callback.searchParams.get('state') && callback.searchParams.get('state') !== codexSession.state) {
        throw new Error('The state parameter does not match. Generate a new link and sign in again.')
      }
      codexReady = true
    } catch (cause) {
      localError = cause instanceof Error ? cause.message : 'Enter a valid callback URL.'
    }
  }

  async function generateOAuth() {
    try {
      providerSession = await onGenerateOAuth(draft)
    } catch {
      // Parent exposes request error through error prop.
    }
  }

  async function checkOAuth() {
    try {
      callbackSubmitted = await onCheckOAuth(draft, callbackSubmitted)
    } catch {
      // Parent exposes request error through error prop.
    }
  }

  function submit(event: SubmitEvent) {
    event.preventDefault()
    void onSubmit(draft)
  }
</script>

<form onsubmit={submit}>
  <div class="dialog-header">
    <div><p class="eyebrow">NEW CONNECTION</p><h2 id="dialog-title">Add provider</h2><p>Connect credentials used to route model requests.</p></div>
    <button type="button" class="close-button" onclick={onClose} aria-label="Close">×</button>
  </div>
  <div class="dialog-body provider-form">
    <div class="form-grid">
      <div class="field-group"><label for="provider-name">Provider name</label><div class="text-field"><input id="provider-name" bind:value={name} placeholder="My provider" /></div></div>
      <div class="field-group">
        <label for="provider-type">Provider type</label>
        <div class="select-field">
          <select id="provider-type" bind:value={type} onchange={resetAuthorization}>
            {#each providerOptions as option}<option value={option.value}>{option.label}</option>{/each}
          </select>
        </div>
      </div>
    </div>
    <div class="field-group">
      <label for="custom-url">Custom URL <span>Optional</span></label>
      <div class="text-field"><input id="custom-url" type="url" bind:value={customUrl} placeholder={type === 'PROVIDER_TYPE_OPENAI' ? 'Leave empty for ChatGPT Codex' : 'https://api.example.com/v1'} /></div>
      <p class="supporting">{type === 'PROVIDER_TYPE_OPENAI' ? 'Leave empty for the ChatGPT Codex endpoint. OAuth tokens do not work with api.openai.com/v1.' : 'Leave empty to use the provider’s default endpoint.'}</p>
    </div>
    <ProxySettings bind:useProxy bind:proxy idPrefix="new-provider" disabled={pending} />

    {#if type === 'PROVIDER_TYPE_OPENAI'}
      <div class="oauth-panel">
        <div class="stepper" aria-label="Authorization progress">
          <span class:done={Boolean(codexSession)} class:active={!codexSession}>1</span><i></i>
          <span class:done={codexReady} class:active={Boolean(codexSession) && !codexReady}>2</span><i></i>
          <span class:done={codexReady} class:active={codexReady}>3</span>
        </div>
        <h3>{!codexSession ? 'Create a Codex authorization link' : !codexReady ? 'Complete authorization' : 'Authorization ready'}</h3>
        {#if !codexSession}
          <p>We’ll create a secure PKCE link that redirects to Codex’s local callback.</p>
          <button type="button" class="tonal" onclick={generateCodex} disabled={pending}>{pending ? 'Starting…' : 'Generate authorization link'}</button>
        {:else if !codexReady}
          <ol><li>Copy the link below and open it in the authorization window.</li><li>Sign in with OpenAI. The final page may fail to open if no local listener is running.</li><li>Copy the full <code>http://localhost:1455/auth/callback?…</code> URL from the address bar and paste it below.</li></ol>
          <div class="copy-field"><input readonly value={codexSession.authorizationUrl} aria-label="Codex authorization link" /><button type="button" onclick={() => onCopy(codexSession?.authorizationUrl ?? '', 'Authorization link copied.')}><Icon name="copy" size={19} /> Copy</button></div>
          <div class="field-group"><label for="callback-url">Callback URL</label><textarea id="callback-url" bind:value={authorizationData} placeholder="http://localhost:1455/auth/callback?code=…&state=…"></textarea></div>
          <button type="button" class="tonal" onclick={validateCodexCallback}>Check callback URL</button>
        {:else}
          <div class="oauth-success"><span><Icon name="check" /></span><div><strong>Authorization code ready</strong><p>The backend will exchange it and store the complete Codex credentials.</p></div></div>
        {/if}
      </div>
    {:else if type === 'PROVIDER_TYPE_CUSTOM'}
      <div class="field-group"><label for="auth-data">Authorization data</label><textarea id="auth-data" bind:value={authorizationData} placeholder="Paste API key or authorization token"></textarea><p class="supporting">Stored by the router and never displayed in the provider list.</p></div>
    {:else}
      <div class="oauth-panel">
        <h3>{providerSession ? 'Complete authorization' : `Authorize ${providerLabels[type]}`}</h3>
        {#if !providerSession}
          <p>Sign in with your provider account. No API key needs to be pasted.</p>
          <button type="button" class="tonal" onclick={generateOAuth} disabled={pending}>{pending ? 'Starting…' : 'Start authorization'}</button>
        {:else}
          <ol><li>Open the authorization link and sign in.</li>{#if providerSession.userCode}<li>Enter device code <code>{providerSession.userCode}</code> when asked.</li>{/if}<li>{providerSession.flow === 'callback' ? 'Copy the final localhost callback URL and paste it below.' : 'Return here after the provider confirms authorization.'}</li></ol>
          <div class="copy-field"><input readonly value={providerSession.authorizationUrl} aria-label="Provider authorization link" /><button type="button" onclick={() => onCopy(providerSession?.authorizationUrl ?? '', 'Authorization link copied.')}><Icon name="copy" size={19} /> Copy</button></div>
          {#if providerSession.flow === 'callback'}<div class="field-group"><label for="provider-callback-url">Callback URL</label><textarea id="provider-callback-url" bind:value={authorizationData} placeholder="http://localhost:54545/callback?code=…&state=…"></textarea></div>{/if}
          <button type="button" class="tonal" onclick={checkOAuth} disabled={pending || (providerSession.flow === 'callback' && !authorizationData.trim())}>{pending ? 'Checking…' : 'Check authorization'}</button>
        {/if}
      </div>
    {/if}
    {#if localError || error}<p class="form-error" role="alert"><Icon name="warning" size={18} />{localError || error}</p>{/if}
  </div>
  <div class="dialog-actions"><button type="button" class="text-button" onclick={onClose}>Cancel</button><button class="filled" disabled={!canSubmit}>{pending ? 'Adding…' : 'Add provider'}</button></div>
</form>
