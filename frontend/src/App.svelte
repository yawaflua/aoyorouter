<script lang="ts">
  import Icon from './lib/Icon.svelte'
  import { ApiClient, ApiError, type ApiKey, type ApiKeyUsage, type LogEntry, type Provider, type ProviderAuthorization, type ProviderType } from './lib/api'
  import { CODEX_CALLBACK_KEY } from './lib/codex'
  import type { CodexAuthorization } from './lib/api'

  type Section = 'keys' | 'providers' | 'logs'
  type Dialog = 'key' | 'provider' | 'secret' | 'delete-key' | 'delete-provider' | null

  const providerLabels: Record<ProviderType, string> = {
    PROVIDER_TYPE_CUSTOM: 'OpenAI-compatible',
    PROVIDER_TYPE_OPENAI: 'OpenAI Codex',
    PROVIDER_TYPE_ANTHROPIC: 'Anthropic',
    PROVIDER_TYPE_KIMI: 'Kimi',
    PROVIDER_TYPE_GROK: 'xAI Grok',
    PROVIDER_TYPE_ANTIGRAVITY: 'Google Antigravity',
  }

  let password = $state(sessionStorage.getItem('aoyo.password') ?? '')
  let passwordInput = $state('')
  let showPassword = $state(false)
  let signedIn = $state(Boolean(sessionStorage.getItem('aoyo.password')))
  let signingIn = $state(false)
  let authError = $state('')

  let section = $state<Section>('keys')
  let dialog = $state<Dialog>(null)
  let loading = $state(false)
  let actionPending = $state(false)
  let pageError = $state('')
  let dialogError = $state('')
  let notice = $state('')

  let apiKeys = $state<ApiKey[]>([])
  let providers = $state<Provider[]>([])
  let logs = $state<LogEntry[]>([])
  let search = $state('')

  let keyName = $state('')
  let keyAdmin = $state(false)
  let createdKey = $state('')
  let targetKey = $state<ApiKey | null>(null)
  let expandedKeyId = $state('')
  let keyUsage = $state<Record<string, ApiKeyUsage>>({})
  let keyUsageLoading = $state('')
  let keyUsageErrors = $state<Record<string, string>>({})

  let providerName = $state('')
  let providerType = $state<ProviderType>('PROVIDER_TYPE_OPENAI')
  let customUrl = $state('')
  let authorizationData = $state('')
  let codexSession = $state<CodexAuthorization | null>(null)
  let codexStep = $state<1 | 2 | 3>(1)
  let providerSession = $state<ProviderAuthorization | null>(null)
  let providerAuthReady = $state(false)
  let providerCallbackSubmitted = $state(false)
  let targetProvider = $state<Provider | null>(null)

  const client = $derived(new ApiClient(password))
  const filteredKeys = $derived(
    apiKeys.filter((key) => `${key.name} ${key.id}`.toLowerCase().includes(search.toLowerCase())),
  )
  const filteredProviders = $derived(
    providers.filter((provider) =>
      `${provider.name} ${provider.customUrl} ${providerLabels[provider.type]}`
        .toLowerCase()
        .includes(search.toLowerCase()),
    ),
  )

  $effect(() => {
    if (!signedIn) return
    void loadSection(section)
  })

  function message(error: unknown): string {
    if (error instanceof ApiError && error.status === 401) {
      signOut(false)
      return 'Your session is no longer valid. Sign in again.'
    }
    return error instanceof Error ? error.message : 'Something went wrong. Try again.'
  }

  async function signIn(event: SubmitEvent) {
    event.preventDefault()
    authError = ''
    if (!passwordInput.trim()) {
      authError = 'Enter your administrator password.'
      return
    }

    signingIn = true
    try {
      const nextClient = new ApiClient(passwordInput)
      await nextClient.signIn()
      password = passwordInput
      sessionStorage.setItem('aoyo.password', password)
      signedIn = true
      passwordInput = ''
    } catch (error) {
      authError = message(error)
    } finally {
      signingIn = false
    }
  }

  function signOut(showNotice = true) {
    sessionStorage.removeItem('aoyo.password')
    password = ''
    passwordInput = ''
    signedIn = false
    apiKeys = []
    providers = []
    logs = []
    closeDialog()
    if (showNotice) notice = 'Signed out.'
  }

  async function loadSection(nextSection: Section) {
    loading = true
    pageError = ''
    try {
      if (nextSection === 'keys') apiKeys = await client.getApiKeys()
      if (nextSection === 'providers') providers = await client.getProviders()
      if (nextSection === 'logs') logs = await client.getUsageLogs()
    } catch (error) {
      pageError = message(error)
    } finally {
      loading = false
    }
  }

  function navigate(nextSection: Section) {
    section = nextSection
    search = ''
  }

  function openKeyDialog() {
    keyName = ''
    keyAdmin = false
    dialogError = ''
    dialog = 'key'
  }

  async function toggleKeyDetails(key: ApiKey) {
    if (expandedKeyId === key.id) {
      expandedKeyId = ''
      return
    }
    expandedKeyId = key.id
    if (keyUsage[key.id]) return

    await loadKeyUsage(key)
  }

  async function loadKeyUsage(key: ApiKey) {
    keyUsageLoading = key.id
    keyUsageErrors = { ...keyUsageErrors, [key.id]: '' }
    try {
      keyUsage = { ...keyUsage, [key.id]: await client.getProviderLogsByKeyID(key.id) }
    } catch (error) {
      keyUsageErrors = { ...keyUsageErrors, [key.id]: message(error) }
    } finally {
      keyUsageLoading = ''
    }
  }

  function handleKeyRowKeydown(event: KeyboardEvent, key: ApiKey) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    void toggleKeyDetails(key)
  }

  function openProviderDialog() {
    providerName = ''
    providerType = 'PROVIDER_TYPE_OPENAI'
    customUrl = ''
    authorizationData = ''
    codexSession = null
    codexStep = 1
    providerSession = null
    providerAuthReady = false
    providerCallbackSubmitted = false
    dialogError = ''
    dialog = 'provider'
  }

  function closeDialog() {
    dialog = null
    dialogError = ''
    actionPending = false
  }

  async function createKey(event: SubmitEvent) {
    event.preventDefault()
    if (!keyName.trim()) {
      dialogError = 'Give this key a name.'
      return
    }
    actionPending = true
    dialogError = ''
    try {
      const result = await client.createApiKey(keyName.trim(), keyAdmin)
      createdKey = result.value
      await loadSection('keys')
      dialog = 'secret'
    } catch (error) {
      dialogError = message(error)
    } finally {
      actionPending = false
    }
  }

  async function deleteKey() {
    if (!targetKey) return
    actionPending = true
    dialogError = ''
    try {
      await client.deleteApiKey(targetKey.id)
      apiKeys = apiKeys.filter((key) => key.id !== targetKey?.id)
      if (expandedKeyId === targetKey.id) expandedKeyId = ''
      notice = `“${targetKey.name}” deleted.`
      closeDialog()
    } catch (error) {
      dialogError = message(error)
      actionPending = false
    }
  }

  async function generateCodexLink() {
    dialogError = ''
    if (!providerName.trim()) {
      dialogError = 'Give this provider a name first.'
      return
    }
    try {
      codexSession = await client.createCodexAuthorization(providerName.trim(), customUrl.trim())
      codexStep = 2
    } catch (error) {
      dialogError = message(error)
    }
  }

  function isOAuthProvider(type: ProviderType): boolean {
    return type !== 'PROVIDER_TYPE_CUSTOM'
  }

  function resetProviderAuthorization() {
    authorizationData = ''
    codexSession = null
    codexStep = 1
    providerSession = null
    providerAuthReady = false
    providerCallbackSubmitted = false
    dialogError = ''
  }

  async function generateProviderAuthorization() {
    dialogError = ''
    if (!providerName.trim()) {
      dialogError = 'Give this provider a name first.'
      return
    }
    if (providerType === 'PROVIDER_TYPE_CUSTOM' || providerType === 'PROVIDER_TYPE_OPENAI') return
    actionPending = true
    try {
      providerSession = await client.createProviderAuthorization({
        name: providerName.trim(),
        type: providerType,
        customUrl: customUrl.trim(),
      })
      providerAuthReady = false
      providerCallbackSubmitted = false
    } catch (error) {
      dialogError = message(error)
    } finally {
      actionPending = false
    }
  }

  async function checkProviderAuthorization() {
    if (!providerSession) return
    actionPending = true
    dialogError = ''
    try {
      let result
      if (providerSession.flow === 'callback' && !providerCallbackSubmitted) {
        result = await client.completeProviderAuthorization(providerSession.state, authorizationData.trim())
        providerCallbackSubmitted = true
      } else {
        result = await client.getProviderAuthorizationStatus(providerSession.state)
      }
      for (let attempt = 0; result.status === 'pending' && attempt < 20; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 750))
        result = await client.getProviderAuthorizationStatus(providerSession.state)
      }
      if (result.status === 'error') throw new Error(result.error || 'Authorization failed. Start a new authorization flow.')
      if (result.status === 'pending') {
        dialogError = 'Authorization is still pending. Finish signing in, then check again.'
        return
      }
      await loadSection('providers')
      notice = `“${providerName.trim()}” added.`
      closeDialog()
    } catch (error) {
      dialogError = message(error)
    } finally {
      actionPending = false
    }
  }

  async function copy(value: string, success: string) {
    try {
      await navigator.clipboard.writeText(value)
      notice = success
    } catch {
      notice = 'Could not copy. Select the text manually.'
    }
  }

  function parseCodexAuthorization() {
    if (!codexSession) {
      dialogError = 'Generate a new authorization link first.'
      return
    }
    dialogError = ''
    try {
      const callback = new URL(authorizationData)
      if (!callback.searchParams.get('code')) throw new Error('The callback URL does not contain an authorization code.')
      if (callback.searchParams.get('state') && callback.searchParams.get('state') !== codexSession.state) {
        throw new Error('The state parameter does not match. Generate a new link and sign in again.')
      }
      codexStep = 3
    } catch (error) {
      dialogError = message(error)
    }
  }

  async function createProvider(event: SubmitEvent) {
    event.preventDefault()
    if (!providerName.trim()) {
      dialogError = 'Give this provider a name.'
      return
    }
    if (providerType === 'PROVIDER_TYPE_CUSTOM' && !authorizationData.trim()) {
      dialogError = 'Add authorization data before saving.'
      return
    }
    try {
      if (customUrl) new URL(customUrl)
    } catch {
      dialogError = 'Enter a valid custom URL, including https://.'
      return
    }
    if (providerType === 'PROVIDER_TYPE_OPENAI' && /(^|\.)api\.openai\.com$/i.test(new URL(customUrl || 'https://chatgpt.com').hostname)) {
      dialogError = 'Codex OAuth tokens cannot use api.openai.com. Leave Custom URL empty to use the ChatGPT Codex endpoint.'
      return
    }

    actionPending = true
    dialogError = ''
    try {
      if (providerType === 'PROVIDER_TYPE_OPENAI') {
        if (!codexSession) throw new Error('Generate a new Codex authorization link.')
        await client.completeCodexAuthorization({
          state: codexSession.state,
          callbackUrl: authorizationData.trim(),
        })
      } else if (providerType === 'PROVIDER_TYPE_CUSTOM') {
        await client.createProvider({
          name: providerName.trim(),
          type: providerType,
          customUrl: customUrl.trim(),
          authorizationData: authorizationData.trim(),
        })
      } else if (!providerAuthReady) {
        throw new Error('Complete provider authorization before saving.')
      }
      await loadSection('providers')
      notice = `“${providerName.trim()}” added.`
      closeDialog()
    } catch (error) {
      dialogError = message(error)
    } finally {
      actionPending = false
    }
  }

  async function deleteProvider() {
    if (!targetProvider) return
    actionPending = true
    dialogError = ''
    try {
      await client.deleteProvider(targetProvider.id)
      providers = providers.filter((provider) => provider.id !== targetProvider?.id)
      notice = `“${targetProvider.name}” deleted.`
      closeDialog()
    } catch (error) {
      dialogError = message(error)
      actionPending = false
    }
  }

  function handleOAuthCallback() {
    if (window.location.pathname !== '/oauth/codex/callback') return
    sessionStorage.setItem(CODEX_CALLBACK_KEY, window.location.href)
    window.history.replaceState({}, '', '/')
    authorizationData = sessionStorage.getItem(CODEX_CALLBACK_KEY) ?? ''
    signedIn = Boolean(password)
    if (signedIn) {
      section = 'providers'
      openProviderDialog()
      codexStep = 1
    }
  }

  function quotaLabel(provider: Provider): string {
    const quota = provider.quota?.primary
    if (!quota) return provider.quota?.error?.includes('active subscription') ? 'Subscription required' : provider.quota?.error ? 'Quota unavailable' : ''
    return `${Math.max(0, Math.round(100 - quota.usedPercent))}% left`
  }

  function quotaReset(provider: Provider): string {
    if (provider.quota?.error) return provider.quota.error
    const reset = provider.quota?.primary?.resetsAt
    if (!reset) return ''
    const date = new Date(reset)
    return Number.isNaN(date.getTime()) ? '' : `Resets ${date.toLocaleString()}`
  }

  function formatTokens(tokens: number): string {
    return `${(tokens / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: 3 })}M tokens`
  }

  function formatLogTime(entry: LogEntry): string {
    const value = entry.requestTime || entry.createdAt
    if (!value) return 'Unknown time'
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
  }

  handleOAuthCallback()
</script>

<svelte:head>
  <title>{signedIn ? `${section === 'keys' ? 'API keys' : section === 'providers' ? 'Providers' : 'Logs'} · Aoyo Router` : 'Sign in · Aoyo Router'}</title>
  <meta name="theme-color" content="#f8f9ff" />
</svelte:head>

{#if !signedIn}
  <main class="auth-shell">
    <section class="auth-card" aria-labelledby="signin-title">
      <div class="brand-mark" aria-hidden="true"><span></span><span></span><span></span></div>
      <p class="eyebrow">AOYO ROUTER</p>
      <h1 id="signin-title">Welcome back</h1>
      <p class="auth-intro">Sign in to manage API access and model providers.</p>

      <form onsubmit={signIn} novalidate>
        <div class="field-group">
          <label for="password">Administrator password</label>
          <div class:error={authError} class="text-field with-action">
            <input
              id="password"
              type={showPassword ? 'text' : 'password'}
              bind:value={passwordInput}
              autocomplete="current-password"
              aria-describedby="password-support password-error"
              aria-invalid={Boolean(authError)}
            />
            <button
              class="field-action"
              type="button"
              onclick={() => (showPassword = !showPassword)}
              aria-label={showPassword ? 'Hide password' : 'Show password'}
            ><Icon name={showPassword ? 'eye-off' : 'eye'} size={20} /></button>
          </div>
          <p id="password-support" class="supporting">Used only to authorize requests to your router.</p>
          {#if authError}<p id="password-error" class="field-error" role="alert">{authError}</p>{/if}
        </div>
        <button class="filled wide" type="submit" disabled={signingIn}>
          {#if signingIn}<span class="spinner"></span> Signing in…{:else}Sign in <Icon name="chevron" size={19} />{/if}
        </button>
      </form>
      <div class="security-note"><Icon name="shield" size={18} /><span>Your password stays in this browser tab.</span></div>
    </section>
  </main>
{:else}
  <div class="app-shell">
    <aside class="rail" aria-label="Primary navigation">
      <div class="rail-brand"><div class="brand-mark small"><span></span><span></span><span></span></div><strong>Aoyo</strong></div>
      <nav>
        <button class:active={section === 'keys'} onclick={() => navigate('keys')} aria-current={section === 'keys' ? 'page' : undefined}>
          <Icon name="key" /><span>API keys</span>
        </button>
        <button class:active={section === 'providers'} onclick={() => navigate('providers')} aria-current={section === 'providers' ? 'page' : undefined}>
          <Icon name="provider" /><span>Providers</span>
        </button>
        <button class:active={section === 'logs'} onclick={() => navigate('logs')} aria-current={section === 'logs' ? 'page' : undefined}>
          <Icon name="logs" /><span>Logs</span><span class="beta">BETA</span>
        </button>
      </nav>
      <button class="rail-signout" onclick={() => signOut()}><Icon name="logout" /><span>Sign out</span></button>
    </aside>

    <main class="workspace">
      <header class="topbar">
        <div>
          <p class="eyebrow">ADMIN CONSOLE</p>
          <h1>{section === 'keys' ? 'API keys' : section === 'providers' ? 'Providers' : 'Request logs'}</h1>
          <p>{section === 'keys' ? 'Create and revoke credentials used by your applications.' : section === 'providers' ? 'Connect the model services available through your router.' : 'Inspect requests routed through each provider.'}</p>
        </div>
        {#if section !== 'logs'}
          <button class="filled top-action" onclick={section === 'keys' ? openKeyDialog : openProviderDialog}>
            <Icon name="plus" size={20} /> {section === 'keys' ? 'Create key' : 'Add provider'}
          </button>
        {/if}
      </header>

      <section class="content" aria-live="polite">
        {#if section !== 'logs'}
          <div class="collection-toolbar">
            <label class="search-field">
              <Icon name="search" size={20} />
              <span class="sr-only">Search {section === 'keys' ? 'API keys' : 'providers'}</span>
              <input bind:value={search} placeholder={section === 'keys' ? 'Search API keys' : 'Search providers'} />
            </label>
            <span class="count">{section === 'keys' ? filteredKeys.length : filteredProviders.length} total</span>
            <button class="icon-button" onclick={() => loadSection(section)} aria-label="Refresh"><Icon name="refresh" size={20} /></button>
          </div>
        {/if}

        {#if pageError}
          <div class="state-panel error-state" role="alert">
            <div class="state-icon"><Icon name="warning" /></div><div><h2>Couldn’t load this page</h2><p>{pageError}</p></div>
            <button class="tonal" onclick={() => loadSection(section)}>Try again</button>
          </div>
        {:else if loading}
          <div class="loading-list" aria-label="Loading">
            {#each [1, 2, 3] as item}<div class="skeleton-row"><span></span><div><i></i><i></i></div></div>{/each}
          </div>
        {:else if section === 'keys'}
          {#if filteredKeys.length}
            <div class="data-list" aria-label="API keys">
              {#each filteredKeys as key (key.id)}
                <article class:key-expanded={expandedKeyId === key.id} class="key-item">
                  <div
                    class="data-row key-row"
                    role="button"
                    tabindex="0"
                    aria-expanded={expandedKeyId === key.id}
                    onclick={() => toggleKeyDetails(key)}
                    onkeydown={(event) => handleKeyRowKeydown(event, key)}
                  >
                    <div class="entity-icon key-icon"><Icon name="key" /></div>
                    <div class="entity-main"><h2>{key.name}</h2><p><code>{key.id}</code></p></div>
                    {#if key.isAdmin === 'true'}<span class="status-chip">Admin</span>{/if}
                    <span class:expanded={expandedKeyId === key.id} class="expand-icon" aria-hidden="true"><Icon name="chevron" size={19} /></span>
                    <button class="icon-button danger" onclick={(event) => { event.stopPropagation(); targetKey = key; dialogError = ''; dialog = 'delete-key' }} aria-label={`Delete ${key.name}`}><Icon name="trash" size={20} /></button>
                  </div>
                  {#if expandedKeyId === key.id}
                    <section class="key-details" aria-label={`${key.name} usage details`}>
                      {#if keyUsageLoading === key.id}
                        <div class="details-loading"><span class="dark-spinner"></span> Loading usage…</div>
                      {:else if keyUsageErrors[key.id]}
                        <div class="details-error" role="alert"><Icon name="warning" size={19} /><span>{keyUsageErrors[key.id]}</span><button class="text-button" onclick={() => loadKeyUsage(key)}>Retry</button></div>
                      {:else if keyUsage[key.id]}
                        <div class="usage-summary">
                          <div><span>Used quota</span><strong>{formatTokens(keyUsage[key.id].totalTokens)}</strong></div>
                          <div><span>Recent requests</span><strong>{keyUsage[key.id].logs.length} of 10</strong></div>
                        </div>
                        {#if keyUsage[key.id].logs.length}
                          <div class="key-log-list">
                            {#each keyUsage[key.id].logs as entry}
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
              <div class="state-icon"><Icon name="key" /></div><div><h2>{search ? 'No matching keys' : 'No API keys yet'}</h2><p>{search ? 'Try a different name or clear your search.' : 'Create a key to let an application use this router.'}</p></div>
              <button class="tonal" onclick={search ? () => (search = '') : openKeyDialog}>{search ? 'Clear search' : 'Create key'}</button>
            </div>
          {/if}
        {:else if section === 'providers'}
          {#if filteredProviders.length}
            <div class="data-list" aria-label="Providers">
              {#each filteredProviders as provider (provider.id)}
                <article class="data-row provider-row">
                  <div class="entity-icon provider-icon"><span>{provider.name.slice(0, 1).toUpperCase()}</span></div>
                  <div class="entity-main"><h2>{provider.name}</h2><p>{provider.customUrl || 'Default endpoint'}</p></div>
                  <span class="type-label">{providerLabels[provider.type]}</span>
                  {#if provider.quota}
                    <div class="quota" title={quotaReset(provider)}>
                      <div><span style={`width:${Math.max(0, 100 - (provider.quota.primary?.usedPercent ?? 100))}%`}></span></div>
                      <small>{quotaLabel(provider)}</small>
                    </div>
                  {/if}
                  <span class="status-dot"><i></i> Connected</span>
                  <button class="icon-button danger" onclick={() => { targetProvider = provider; dialogError = ''; dialog = 'delete-provider' }} aria-label={`Delete ${provider.name}`}><Icon name="trash" size={20} /></button>
                </article>
              {/each}
            </div>
          {:else}
            <div class="state-panel empty-state">
              <div class="state-icon"><Icon name="provider" /></div><div><h2>{search ? 'No matching providers' : 'No providers connected'}</h2><p>{search ? 'Try a different name or clear your search.' : 'Add a provider to start routing model requests.'}</p></div>
              <button class="tonal" onclick={search ? () => (search = '') : openProviderDialog}>{search ? 'Clear search' : 'Add provider'}</button>
            </div>
          {/if}
        {:else}
          {#if logs.length}
            <div class="log-list">
              {#each logs as entry}
                <article class:failed={entry.failed} class="log-row">
                  <div><strong>{entry.model || 'Unknown model'}</strong><time>{formatLogTime(entry)}</time></div>
                  <div class="log-provider"><strong>{entry.provider || 'Unknown provider'}</strong><code>{entry.apiKeyId || 'No key ID'}</code></div>
                  <div class="token-breakdown"><span>{entry.inputTokens.toLocaleString()} input</span><span>{entry.outputTokens.toLocaleString()} output</span><strong>{entry.totalTokens.toLocaleString()} total</strong></div>
                  <span>{entry.failed ? 'Failed' : 'Completed'}</span>
                </article>
              {/each}
            </div>
          {:else}
            <div class="state-panel empty-state compact"><div class="state-icon"><Icon name="logs" /></div><div><h2>No logs to show</h2><p>New request records will appear here after traffic reaches the router.</p></div></div>
          {/if}
        {/if}
      </section>
    </main>

    <nav class="bottom-nav" aria-label="Primary navigation">
      <button class:active={section === 'keys'} onclick={() => navigate('keys')}><Icon name="key" /><span>API keys</span></button>
      <button class:active={section === 'providers'} onclick={() => navigate('providers')}><Icon name="provider" /><span>Providers</span></button>
      <button class:active={section === 'logs'} onclick={() => navigate('logs')}><Icon name="logs" /><span>Logs</span></button>
    </nav>
  </div>
{/if}

{#if dialog}
  <div class="scrim" role="presentation" onclick={(event) => event.target === event.currentTarget && closeDialog()}>
    <div class:wide-dialog={dialog === 'provider'} class="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title" tabindex="-1">
      {#if dialog === 'key'}
        <form onsubmit={createKey}>
          <div class="dialog-header"><div><p class="eyebrow">NEW CREDENTIAL</p><h2 id="dialog-title">Create API key</h2><p>The key is shown once after creation.</p></div><button type="button" class="close-button" onclick={closeDialog} aria-label="Close">×</button></div>
          <div class="dialog-body">
            <div class="field-group"><label for="key-name">Key name</label><div class="text-field"><input id="key-name" bind:value={keyName} placeholder="Production app" /></div><p class="supporting">Use a name that identifies the application.</p></div>
            <label class="switch-row"><span><strong>Administrator access</strong><small>Allows this key to manage router resources.</small></span><input type="checkbox" bind:checked={keyAdmin} /><i></i></label>
            {#if dialogError}<p class="form-error" role="alert"><Icon name="warning" size={18} />{dialogError}</p>{/if}
          </div>
          <div class="dialog-actions"><button type="button" class="text-button" onclick={closeDialog}>Cancel</button><button class="filled" disabled={actionPending}>{actionPending ? 'Creating…' : 'Create key'}</button></div>
        </form>
      {:else if dialog === 'secret'}
        <div class="dialog-header"><div><p class="eyebrow">KEY CREATED</p><h2 id="dialog-title">Save your API key</h2><p>This is the only time the full key will be shown.</p></div></div>
        <div class="dialog-body"><div class="secret-box"><code>{createdKey}</code><button class="icon-button" onclick={() => copy(createdKey, 'API key copied.')} aria-label="Copy API key"><Icon name="copy" size={20} /></button></div><div class="inline-message warning"><Icon name="warning" size={20} /><span>Store this key somewhere secure before closing.</span></div></div>
        <div class="dialog-actions"><button class="filled" onclick={closeDialog}>I’ve saved it</button></div>
      {:else if dialog === 'provider'}
        <form onsubmit={createProvider}>
          <div class="dialog-header"><div><p class="eyebrow">NEW CONNECTION</p><h2 id="dialog-title">Add provider</h2><p>Connect credentials used to route model requests.</p></div><button type="button" class="close-button" onclick={closeDialog} aria-label="Close">×</button></div>
          <div class="dialog-body provider-form">
            <div class="form-grid">
              <div class="field-group"><label for="provider-name">Provider name</label><div class="text-field"><input id="provider-name" bind:value={providerName} placeholder="My provider" /></div></div>
              <div class="field-group"><label for="provider-type">Provider type</label><div class="select-field"><select id="provider-type" bind:value={providerType} onchange={resetProviderAuthorization}><option value="PROVIDER_TYPE_OPENAI">OpenAI Codex</option><option value="PROVIDER_TYPE_ANTHROPIC">Anthropic</option><option value="PROVIDER_TYPE_KIMI">Kimi</option><option value="PROVIDER_TYPE_GROK">xAI Grok</option><option value="PROVIDER_TYPE_ANTIGRAVITY">Google Antigravity</option><option value="PROVIDER_TYPE_CUSTOM">OpenAI-compatible</option></select></div></div>
            </div>
            <div class="field-group"><label for="custom-url">Custom URL <span>Optional</span></label><div class="text-field"><input id="custom-url" type="url" bind:value={customUrl} placeholder={providerType === 'PROVIDER_TYPE_OPENAI' ? 'Leave empty for ChatGPT Codex' : 'https://api.example.com/v1'} /></div><p class="supporting">{providerType === 'PROVIDER_TYPE_OPENAI' ? 'Leave empty for the ChatGPT Codex endpoint. OAuth tokens do not work with api.openai.com/v1.' : 'Leave empty to use the provider’s default endpoint.'}</p></div>

            {#if providerType === 'PROVIDER_TYPE_OPENAI'}
              <div class="oauth-panel">
                <div class="stepper" aria-label="Authorization progress"><span class:done={codexStep > 1} class:active={codexStep === 1}>1</span><i></i><span class:done={codexStep > 2} class:active={codexStep === 2}>2</span><i></i><span class:done={codexStep === 3} class:active={codexStep === 3}>3</span></div>
                <h3>{codexStep === 1 ? 'Create a Codex authorization link' : codexStep === 2 ? 'Complete authorization' : 'Authorization ready'}</h3>
                {#if codexStep === 1}
                  <p>We’ll create a secure PKCE link that redirects to Codex’s local callback.</p><button type="button" class="tonal" onclick={generateCodexLink}>Generate authorization link</button>
                {:else if codexStep === 2 && codexSession}
                  <ol><li>Copy the link below and open it in the authorization window.</li><li>Sign in with OpenAI. The final page may fail to open if no local listener is running.</li><li>Copy the full <code>http://localhost:1455/auth/callback?…</code> URL from the address bar and paste it below.</li></ol>
                  <div class="copy-field"><input readonly value={codexSession.authorizationUrl} aria-label="Codex authorization link" /><button type="button" onclick={() => copy(codexSession?.authorizationUrl ?? '', 'Authorization link copied.')}><Icon name="copy" size={19} /> Copy</button></div>
                  <div class="field-group"><label for="callback-url">Callback URL</label><textarea id="callback-url" bind:value={authorizationData} placeholder="http://localhost:1455/auth/callback?code=…&state=…"></textarea></div>
                  <button type="button" class="tonal" onclick={parseCodexAuthorization}>Check callback URL</button>
                {:else}
                  <div class="oauth-success"><span><Icon name="check" /></span><div><strong>Authorization code ready</strong><p>The backend will exchange it and store the complete Codex credentials.</p></div></div>
                {/if}
              </div>
            {:else if providerType === 'PROVIDER_TYPE_CUSTOM'}
              <div class="field-group"><label for="auth-data">Authorization data</label><textarea id="auth-data" bind:value={authorizationData} placeholder="Paste API key or authorization token"></textarea><p class="supporting">Stored by the router and never displayed in the provider list.</p></div>
            {:else}
              <div class="oauth-panel">
                <h3>{providerAuthReady ? 'Authorization complete' : providerSession ? 'Complete authorization' : `Authorize ${providerLabels[providerType]}`}</h3>
                {#if providerAuthReady}
                  <div class="oauth-success"><span><Icon name="check" /></span><div><strong>Provider connected</strong><p>Credentials were stored securely by the backend.</p></div></div>
                {:else if !providerSession}
                  <p>Sign in with your provider account. No API key needs to be pasted.</p>
                  <button type="button" class="tonal" onclick={generateProviderAuthorization} disabled={actionPending}>{actionPending ? 'Starting…' : 'Start authorization'}</button>
                {:else}
                  <ol>
                    <li>Open the authorization link and sign in.</li>
                    {#if providerSession.userCode}<li>Enter device code <code>{providerSession.userCode}</code> when asked.</li>{/if}
                    <li>{providerSession.flow === 'callback' ? 'Copy the final localhost callback URL and paste it below.' : 'Return here after the provider confirms authorization.'}</li>
                  </ol>
                  <div class="copy-field"><input readonly value={providerSession.authorizationUrl} aria-label="Provider authorization link" /><button type="button" onclick={() => copy(providerSession?.authorizationUrl ?? '', 'Authorization link copied.')}><Icon name="copy" size={19} /> Copy</button></div>
                  {#if providerSession.flow === 'callback'}
                    <div class="field-group"><label for="provider-callback-url">Callback URL</label><textarea id="provider-callback-url" bind:value={authorizationData} placeholder="http://localhost:54545/callback?code=…&state=…"></textarea></div>
                  {/if}
                  <button type="button" class="tonal" onclick={checkProviderAuthorization} disabled={actionPending || (providerSession.flow === 'callback' && !authorizationData.trim())}>{actionPending ? 'Checking…' : 'Check authorization'}</button>
                {/if}
              </div>
            {/if}
            {#if dialogError}<p class="form-error" role="alert"><Icon name="warning" size={18} />{dialogError}</p>{/if}
          </div>
          <div class="dialog-actions"><button type="button" class="text-button" onclick={closeDialog}>Cancel</button><button class="filled" disabled={actionPending || (providerType === 'PROVIDER_TYPE_OPENAI' && codexStep !== 3) || (isOAuthProvider(providerType) && providerType !== 'PROVIDER_TYPE_OPENAI' && !providerAuthReady)}>{actionPending ? 'Adding…' : 'Add provider'}</button></div>
        </form>
      {:else if dialog === 'delete-key' || dialog === 'delete-provider'}
        <div class="dialog-header destructive"><div class="state-icon"><Icon name="trash" /></div><div><h2 id="dialog-title">Delete {dialog === 'delete-key' ? 'API key' : 'provider'}?</h2><p>This action cannot be undone.</p></div></div>
        <div class="dialog-body"><p class="confirmation-copy">{dialog === 'delete-key' ? `Applications using “${targetKey?.name}” will immediately lose access.` : `Requests can no longer be routed through “${targetProvider?.name}”.`}</p>{#if dialogError}<p class="form-error" role="alert"><Icon name="warning" size={18} />{dialogError}</p>{/if}</div>
        <div class="dialog-actions"><button class="text-button" onclick={closeDialog}>Cancel</button><button class="danger-button" onclick={dialog === 'delete-key' ? deleteKey : deleteProvider} disabled={actionPending}>{actionPending ? 'Deleting…' : 'Delete'}</button></div>
      {/if}
    </div>
  </div>
{/if}

{#if notice}
  <div class="snackbar" role="status"><Icon name="check" size={19} /><span>{notice}</span><button onclick={() => (notice = '')} aria-label="Dismiss">×</button></div>
{/if}
