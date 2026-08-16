<script lang="ts">
  import Icon from '../Icon.svelte'

  interface Props {
    onSignIn: (password: string) => Promise<void>
  }

  let { onSignIn }: Props = $props()
  let password = $state('')
  let showPassword = $state(false)
  let pending = $state(false)
  let error = $state('')

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    error = ''

    if (!password.trim()) {
      error = 'Enter your administrator password.'
      return
    }

    pending = true
    try {
      await onSignIn(password)
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Something went wrong. Try again.'
    } finally {
      pending = false
    }
  }
</script>

<main class="auth-shell">
  <section class="auth-card" aria-labelledby="signin-title">
    <div class="brand-mark" aria-hidden="true"><span></span><span></span><span></span></div>
    <p class="eyebrow">AOYO ROUTER</p>
    <h1 id="signin-title">Welcome back</h1>
    <p class="auth-intro">Sign in to manage API access and model providers.</p>

    <form onsubmit={submit} novalidate>
      <div class="field-group">
        <label for="password">Administrator password</label>
        <div class:error class="text-field with-action">
          <input
            id="password"
            type={showPassword ? 'text' : 'password'}
            bind:value={password}
            autocomplete="current-password"
            aria-describedby="password-support password-error"
            aria-invalid={Boolean(error)}
          />
          <button
            class="field-action"
            type="button"
            onclick={() => (showPassword = !showPassword)}
            aria-label={showPassword ? 'Hide password' : 'Show password'}
          >
            <Icon name={showPassword ? 'eye-off' : 'eye'} size={20} />
          </button>
        </div>
        <p id="password-support" class="supporting">Used only to authorize requests to your router.</p>
        {#if error}<p id="password-error" class="field-error" role="alert">{error}</p>{/if}
      </div>
      <button class="filled wide" type="submit" disabled={pending}>
        {#if pending}<span class="spinner"></span> Signing in…{:else}Sign in <Icon name="chevron" size={19} />{/if}
      </button>
    </form>
    <div class="security-note"><Icon name="shield" size={18} /><span>Your password stays in this browser tab.</span></div>
  </section>
</main>
