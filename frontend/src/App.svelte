<script lang="ts">
  import { ApiClient } from './lib/api'
  import { getSection, providerLabels, type Dialog, type Section } from './lib/app'
  import ApiKeyList from './lib/components/ApiKeyList.svelte'
  import ApiKeyEditDialog from './lib/components/ApiKeyEditDialog.svelte'
  import CollectionToolbar from './lib/components/CollectionToolbar.svelte'
  import DeleteDialog from './lib/components/DeleteDialog.svelte'
  import KeyDialog from './lib/components/KeyDialog.svelte'
  import LogList from './lib/components/LogList.svelte'
  import Navigation from './lib/components/Navigation.svelte'
  import PageHeader from './lib/components/PageHeader.svelte'
  import ProviderDialog, { type ProviderDraft } from './lib/components/ProviderDialog.svelte'
  import ProviderEditDialog from './lib/components/ProviderEditDialog.svelte'
  import ProviderList from './lib/components/ProviderList.svelte'
  import ProxyList from './lib/components/ProxyList.svelte'
  import SecretDialog from './lib/components/SecretDialog.svelte'
  import SignIn from './lib/components/SignIn.svelte'
  import Icon from './lib/Icon.svelte'
  import { ApiError } from './lib/models/apierror'
  import type { ApiKey, ApiKeyUsage, UpdateApiKeyInput } from './lib/models/apikey'
  import type { LogEntry } from './lib/models/logentry'
  import type { Endpoint, LiveProxy } from './lib/models/liveproxy'
  import type { Provider, ProviderModel, UpdateProviderInput } from './lib/models/providers'
  import { validateProxy } from './lib/models/proxy'
    import ProxyEditDialog from './lib/components/ProxyEditDialog.svelte';

  const PASSWORD_KEY = 'aoyo.password'
  const storedPassword = sessionStorage.getItem(PASSWORD_KEY) ?? ''

  let password = $state(storedPassword)
  let section = $state<Section>('keys')
  let dialog = $state<Dialog>(null)
  let loading = $state(false)
  let actionPending = $state(false)
  let pageError = $state('')
  let dialogError = $state('')
  let notice = $state('')
  let search = $state('')

  let apiKeys = $state<ApiKey[]>([])
  let providers = $state<Provider[]>([])
  let proxies = $state<LiveProxy[]>([])
  let endpoints = $state<Endpoint[]>([])
  let logs = $state<LogEntry[]>([])
  let keyUsage = $state<Record<string, ApiKeyUsage>>({})
  let keyUsageLoading = $state('')
  let keyUsageErrors = $state<Record<string, string>>({})

  let createdKey = $state('')
  let targetKey = $state<ApiKey | null>(null)
  let targetProvider = $state<Provider | null>(null)
  let targetProxy = $state<LiveProxy | null>(null)

  let models = $state<ProviderModel[]>([])


  const client = $derived(new ApiClient(password))
  const sectionInfo = $derived(getSection(section))
  const normalizedSearch = $derived(search.trim().toLowerCase())
  const filteredKeys = $derived(apiKeys.filter((key) => `${key.name} ${key.id}`.toLowerCase().includes(normalizedSearch)))
  const filteredProviders = $derived(
    providers.filter((provider) =>
      `${provider.name} ${provider.customUrl} ${providerLabels[provider.type]}`.toLowerCase().includes(normalizedSearch),
    ),
  )
  const filteredProxies = $derived(
    proxies.filter((proxy) =>
      `${proxy.name} ${proxy.id} ${proxy.url} ${proxy.cloudflareAddress}`.toLowerCase().includes(normalizedSearch),
    ),
  )

  $effect(() => {
    if (password) void loadSection(section)
  })

  function errorMessage(error: unknown): string {
    if (error instanceof ApiError && error.status === 401) {
      signOut(false)
      return 'Your session is no longer valid. Sign in again.'
    }
    return error instanceof Error ? error.message : 'Something went wrong. Try again.'
  }

  async function signIn(candidate: string) {
    try {
      await new ApiClient(candidate).signIn()
      sessionStorage.setItem(PASSWORD_KEY, candidate)
      password = candidate
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        throw new Error('The password is incorrect.')
      }
      throw new Error(errorMessage(error))
    }
  }

  function signOut(showNotice = true) {
    sessionStorage.removeItem(PASSWORD_KEY)
    password = ''
    apiKeys = []
    providers = []
    proxies = []
    logs = []
    closeDialog()
    if (showNotice) notice = 'Signed out.'
  }

  async function loadSection(nextSection: Section) {
    loading = true
    pageError = ''
    try {
      switch (nextSection) {
        case 'keys':
          apiKeys = await client.getApiKeys()
          break
        case 'providers':
          providers = await client.getProviders()
          models = await client.getModels()
          break
        case 'proxies':
          const { resp_proxies, availableEndpoints } = await client.getProxies()
          proxies = resp_proxies
          endpoints = availableEndpoints
          break
        case 'logs':
          logs = await client.getUsageLogs()
      }
    } catch (error) {
      pageError = errorMessage(error)
    } finally {
      loading = false
    }
  }

  function navigate(nextSection: Section) {
    section = nextSection
    search = ''
  }

  function openDialog(nextDialog: Exclude<Dialog, null>) {
    dialogError = ''
    actionPending = false
    dialog = nextDialog
  }

  function closeDialog() {
    dialog = null
    dialogError = ''
    actionPending = false
    targetKey = null
    targetProvider = null
  }

  function openSectionDialog() {
    if (section === 'keys') openDialog('key')
    if (section === 'providers') openDialog('provider')
  }

  async function runDialogAction<T>(action: () => Promise<T>): Promise<T> {
    actionPending = true
    dialogError = ''
    try {
      return await action()
    } catch (error) {
      dialogError = errorMessage(error)
      throw error
    } finally {
      actionPending = false
    }
  }

  async function createKey(name: string, isAdmin: boolean) {
    if (!name.trim()) {
      dialogError = 'Give this key a name.'
      return
    }
    await runDialogAction(async () => {
      createdKey = (await client.createApiKey(name.trim(), isAdmin)).value
      await loadSection('keys')
      dialog = 'secret'
    }).catch(() => undefined)
  }

  async function loadKeyUsage(key: ApiKey) {
    keyUsageLoading = key.id
    keyUsageErrors = { ...keyUsageErrors, [key.id]: '' }
    try {
      keyUsage = { ...keyUsage, [key.id]: await client.getProviderLogsByKeyID(key.id) }
    } catch (error) {
      keyUsageErrors = { ...keyUsageErrors, [key.id]: errorMessage(error) }
    } finally {
      keyUsageLoading = ''
    }
  }

  function requestKeyDelete(key: ApiKey) {
    targetKey = key
    openDialog('delete-key')
  }

  function requestKeyEdit(key: ApiKey) {
    targetKey = key
    openDialog('edit-key')
  }

  async function updateKey(input: UpdateApiKeyInput) {
    await runDialogAction(async () => {
      if (!input.name.trim()) throw new Error('Give this key a name.')
      if (input.quotaSet && (!Number.isSafeInteger(input.reservedTokens) || input.reservedTokens <= 0)) {
        throw new Error('Quota must be greater than zero million tokens.')
      }
      await client.updateApiKey({ ...input, name: input.name.trim() })
      await loadSection('keys')
      delete keyUsage[input.id]
      keyUsage = { ...keyUsage }
      notice = `“${input.name.trim()}” updated.`
      closeDialog()
    }).catch(() => undefined)
  }

  async function deleteKey() {
    if (!targetKey) return
    const key = targetKey
    await runDialogAction(async () => {
      await client.deleteApiKey(key.id)
      apiKeys = apiKeys.filter((item) => item.id !== key.id)
      notice = `“${key.name}” deleted.`
      closeDialog()
    }).catch(() => undefined)
  }

  function validateProvider(draft: ProviderDraft) {
    if (!draft.name.trim()) throw new Error('Give this provider a name.')
    if (draft.customUrl) {
      try {
        new URL(draft.customUrl)
      } catch {
        throw new Error('Enter a valid custom URL, including https://.')
      }
    }
    if (draft.type === 'PROVIDER_TYPE_OPENAI' && /(^|\.)api\.openai\.com$/i.test(new URL(draft.customUrl || 'https://chatgpt.com').hostname)) {
      throw new Error('Codex OAuth tokens cannot use api.openai.com. Leave Custom URL empty to use the ChatGPT Codex endpoint.')
    }
    return validateProxy(draft)
  }


  async function generateProviderAuthorization(draft: ProviderDraft) {
    return runDialogAction(async () => {
      const proxy = validateProvider(draft)
      if (draft.type === 'PROVIDER_TYPE_CUSTOM') {
        throw new Error('This provider does not use OAuth authorization.')
      }
      return client.createProviderAuthorization({ name: draft.name.trim(), type: draft.type, customUrl: draft.customUrl.trim(), ...proxy })
    })
  }

  async function checkProviderAuthorization(draft: ProviderDraft, callbackSubmitted: boolean) {
    return runDialogAction(async () => {
      if (!draft.providerSession) throw new Error('Start provider authorization first.')

      const auth = draft.providerSession
      const proxy = validateProxy(draft)
      let submitted = callbackSubmitted
      let result
      if (auth.flow === 'callback' && !submitted) {
        result = await client.completeProviderAuthorization(auth.state, draft.authorizationData.trim(), proxy.useProxy, proxy.proxy)
        submitted = true
      } else {
        result = await client.getProviderAuthorizationStatus(auth.state, proxy.useProxy, proxy.proxy)
      }

      for (let attempt = 0; result.status === 'pending' && attempt < 20; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 750))
        result = await client.getProviderAuthorizationStatus(auth.state, proxy.useProxy, proxy.proxy)
      }

      if (result.status === 'error') throw new Error(result.error || 'Authorization failed. Start a new authorization flow.')
      if (result.status === 'pending') {
        dialogError = 'Authorization is still pending. Finish signing in, then check again.'
        return submitted
      }

      await loadSection('providers')
      notice = `“${draft.name.trim()}” added.`
      closeDialog()
      return submitted
    })
  }

  async function createProvider(draft: ProviderDraft) {
    await runDialogAction(async () => {
      const proxy = validateProvider(draft)
      if (draft.type === 'PROVIDER_TYPE_CUSTOM') {
        if (!draft.authorizationData.trim()) throw new Error('Add authorization data before saving.')
        await client.createProvider({
          name: draft.name.trim(),
          type: draft.type,
          customUrl: draft.customUrl.trim(),
          authorizationData: draft.authorizationData.trim(),
          isCloudflare: draft.proxy === "",
          ...proxy
        })
      } else {
        throw new Error('Complete provider authorization first.')
      }
      await loadSection('providers')
      notice = `“${draft.name.trim()}” added.`
      closeDialog()
    }).catch(() => undefined)
  }

  function requestProviderDelete(provider: Provider) {
    targetProvider = provider
    openDialog('delete-provider')
  }

  function requestProviderEdit(provider: Provider) {
    targetProvider = provider
    openDialog('edit-provider')
  }

  function requestProxyEdit(proxy: LiveProxy) {
    targetProxy = proxy
    openDialog('edit-proxy')
  }

  async function updateProvider(input: UpdateProviderInput) {
    await runDialogAction(async () => {
      if (!input.name.trim()) throw new Error('Give this provider a name.')
      if (!input.authorizationData.trim()) throw new Error('Authorization data is required.')
      if (input.customUrl.trim()) {
        try {
          new URL(input.customUrl.trim())
        } catch {
          throw new Error('Enter a valid custom URL, including https://.')
        }
      }
      const proxy = validateProxy(input)
      await client.updateProvider({
        ...input,
        name: input.name.trim(),
        customUrl: input.customUrl.trim(),
        authorizationData: input.authorizationData.trim(),
        ...proxy,
      })
      await loadSection('providers')
      notice = `“${input.name.trim()}” updated.`
      closeDialog()
    }).catch(() => undefined)
  }

  async function updateProxy(input: { id: string, endpoint: string, newEndpoint: string }){
    await runDialogAction(async () => {
      await client.updateProxy({
        id: input.id,
        cloudflareEndpoint: input.endpoint,
        newEndpoint: input.newEndpoint,
      })
      await loadSection('proxies')
      notice = `“${input.id.trim()}” updated.`
      closeDialog()
    }).catch(() => undefined)
  }

  async function deleteProvider() {
    if (!targetProvider) return
    const provider = targetProvider
    await runDialogAction(async () => {
      await client.deleteProvider(provider.id)
      providers = providers.filter((item) => item.id !== provider.id)
      notice = `“${provider.name}” deleted.`
      closeDialog()
    }).catch(() => undefined)
  }

  async function copy(value: string, success: string) {
    try {
      await navigator.clipboard.writeText(value)
      notice = success
    } catch {
      notice = 'Could not copy. Select the text manually.'
    }
  }
</script>

<svelte:head>
  <title>{password ? `${sectionInfo.label} · Aoyo Router` : 'Sign in · Aoyo Router'}</title>
  <meta name="theme-color" content="#f8f9ff" />
</svelte:head>

{#if !password}
  <SignIn onSignIn={signIn} />
{:else}
  <div class="app-shell">
    <Navigation current={section} onNavigate={navigate} onSignOut={() => signOut()} />
    <main class="workspace">
      <PageHeader section={sectionInfo} onAction={openSectionDialog} />
      <section class="content" aria-live="polite">
        {#if section !== 'logs'}
          <CollectionToolbar
            bind:search
            entity={section === 'keys' ? 'API keys' : section === 'providers' ? 'providers' : 'live proxies'}
            total={section === 'keys' ? filteredKeys.length : section === 'providers' ? filteredProviders.length : filteredProxies.length}
            onRefresh={() => loadSection(section)}
          />
        {/if}

        {#if pageError}
          <div class="state-panel error-state" role="alert">
            <div class="state-icon"><Icon name="warning" /></div><div><h2>Couldn’t load this page</h2><p>{pageError}</p></div>
            <button class="tonal" onclick={() => loadSection(section)}>Try again</button>
          </div>
        {:else if loading}
          <div class="loading-list" aria-label="Loading">
            {#each [1, 2, 3] as item (item)}<div class="skeleton-row"><span></span><div><i></i><i></i></div></div>{/each}
          </div>
        {:else if section === 'keys'}
          <ApiKeyList
            keys={filteredKeys}
            {search}
            usage={keyUsage}
            usageLoading={keyUsageLoading}
            usageErrors={keyUsageErrors}
            onClearSearch={() => (search = '')}
            onCreate={() => openDialog('key')}
            onEdit={requestKeyEdit}
            onDelete={requestKeyDelete}
            onLoadUsage={loadKeyUsage}
          />
        {:else if section === 'providers'}
          <ProviderList
            providers={filteredProviders}
            {search}
            models={models}
            onClearSearch={() => (search = '')}
            onCreate={() => openDialog('provider')}
            onEdit={requestProviderEdit}
            onDelete={requestProviderDelete}
          />
        {:else if section === 'proxies'}
          <ProxyList proxies={filteredProxies} {search} onEdit={requestProxyEdit} onClearSearch={() => (search = '')} onCopy={copy} />
        {:else}
          <LogList {logs} />
        {/if}
      </section>
    </main>
  </div>
{/if}

{#if dialog}
  <div class="scrim" role="presentation" onclick={(event) => event.target === event.currentTarget && closeDialog()}>
    <div class:wide-dialog={dialog === 'provider' || dialog === 'edit-provider' || dialog === 'edit-key'} class="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title" tabindex="-1">
      {#if dialog === 'key'}
        <KeyDialog pending={actionPending} error={dialogError} onSubmit={createKey} onClose={closeDialog} />
      {:else if dialog === 'edit-key' && targetKey}
        <ApiKeyEditDialog apiKey={targetKey} pending={actionPending} error={dialogError} onSubmit={updateKey} onClose={closeDialog} />
      {:else if dialog === 'secret'}
        <SecretDialog value={createdKey} onCopy={() => copy(createdKey, 'API key copied.')} onClose={closeDialog} />
      {:else if dialog === 'provider'}
        <ProviderDialog
          pending={actionPending}
          error={dialogError}
          onGenerateOAuth={generateProviderAuthorization}
          onCheckOAuth={checkProviderAuthorization}
          onSubmit={createProvider}
          onCopy={copy}
          onClose={closeDialog}
        />
      {:else if dialog === 'edit-provider' && targetProvider}
        <ProviderEditDialog provider={targetProvider} pending={actionPending} error={dialogError} onSubmit={updateProvider} onClose={closeDialog} />
      {:else if dialog === 'delete-key' && targetKey}
        <DeleteDialog entity="API key" name={targetKey.name} pending={actionPending} error={dialogError} onDelete={deleteKey} onClose={closeDialog} />
      {:else if dialog === 'delete-provider' && targetProvider}
        <DeleteDialog entity="provider" name={targetProvider.name} pending={actionPending} error={dialogError} onDelete={deleteProvider} onClose={closeDialog} />
      {:else if dialog === 'edit-proxy' && targetProxy}
        <ProxyEditDialog proxy={targetProxy} availableEndpoints={endpoints} pending={actionPending} error={dialogError} onSubmit={updateProxy} onClose={closeDialog} />
      {/if}
    </div>
  </div>
{/if}

{#if notice}
  <div class="snackbar" role="status"><Icon name="check" size={19} /><span>{notice}</span><button onclick={() => (notice = '')} aria-label="Dismiss">×</button></div>
{/if}
