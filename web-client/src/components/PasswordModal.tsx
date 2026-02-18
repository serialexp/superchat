import { Component, Show, createSignal, createMemo, onMount, onCleanup } from 'solid-js'
import { store, storeActions, ModalState } from '../store/app-store'
import { getProtocolBridge } from '../lib/protocol-bridge'
import { argon2id } from '@noble/hashes/argon2.js'

// Argon2id parameters (must match Go server: pkg/client/auth/password.go)
const ARGON_TIME = 3
const ARGON_MEMORY = 64 * 1024  // 64 MB in KB
const ARGON_PARALLELISM = 4
const ARGON_KEY_LEN = 32

/** Base64 URL-safe encoding without padding (matches Go's base64.RawURLEncoding) */
function base64RawURLEncode(bytes: Uint8Array): string {
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

/** Hash password with argon2id using nickname as salt */
function hashPassword(password: string, nickname: string): string {
  const passwordBytes = new TextEncoder().encode(password)
  const saltBytes = new TextEncoder().encode(nickname)

  const hash = argon2id(passwordBytes, saltBytes, {
    t: ARGON_TIME,
    m: ARGON_MEMORY,
    p: ARGON_PARALLELISM,
    dkLen: ARGON_KEY_LEN,
  })

  return base64RawURLEncode(hash)
}

const PasswordModal: Component = () => {
  const [password, setPassword] = createSignal('')
  const [hashing, setHashing] = createSignal(false)
  let passwordInputRef: HTMLInputElement | undefined

  const isOpen = () => store.activeModal === ModalState.Password
  const nickname = () => store.pendingAuthNickname

  const handleSubmit = async () => {
    const pw = password()
    const nick = nickname()
    if (!pw || !nick || hashing()) return

    setHashing(true)
    store.setAuthError('')

    try {
      // argon2id is CPU-intensive — run in a microtask to allow UI update
      const hashedPassword = await new Promise<string>((resolve) => {
        setTimeout(() => resolve(hashPassword(pw, nick)), 0)
      })

      const client = getProtocolBridge().getClient()
      client.sendAuthRequest(nick, hashedPassword)
    } catch (err) {
      store.setAuthError('Password hashing failed')
      console.error('argon2id error:', err)
    } finally {
      setHashing(false)
    }
  }

  const handleContinueAnonymous = () => {
    // Just close the modal — user is already connected with the nickname (as anonymous)
    setPassword('')
    setHashing(false)
    store.setAuthError('')
    store.setPendingAuthNickname('')
    storeActions.closeModal()
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (!isOpen()) return

    if (e.key === 'Escape') {
      e.preventDefault()
      handleContinueAnonymous()
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown)
  })

  onCleanup(() => {
    window.removeEventListener('keydown', handleKeyDown)
  })

  // Focus password input and reset state when modal opens
  createMemo(() => {
    if (isOpen()) {
      setPassword('')
      setHashing(false)
      setTimeout(() => passwordInputRef?.focus(), 50)
    }
  })

  return (
    <Show when={isOpen()}>
      <div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
        <div class="bg-base-100 rounded-lg shadow-xl w-full max-w-sm mx-4">
          {/* Header */}
          <div class="p-4 border-b border-base-300">
            <h2 class="text-lg font-bold">Sign In</h2>
            <p class="text-sm text-base-content/60 mt-1">
              The nickname <span class="font-mono font-bold text-primary">{nickname()}</span> is registered. Enter your password to sign in, or continue as anonymous.
            </p>
          </div>

          {/* Body */}
          <div class="p-4">
            <Show when={store.authError}>
              <div class="alert alert-error mb-3 py-2 text-sm">
                {store.authError}
              </div>
            </Show>

            <form onSubmit={(e) => { e.preventDefault(); handleSubmit() }}>
              <input
                ref={passwordInputRef}
                type="password"
                placeholder="Password"
                value={password()}
                onInput={(e) => setPassword(e.currentTarget.value)}
                class="input input-bordered w-full"
                disabled={hashing()}
                autofocus
              />

              <div class="flex gap-2 mt-4">
                <button
                  type="submit"
                  class="btn btn-primary flex-1"
                  disabled={!password() || hashing()}
                >
                  <Show when={hashing()} fallback="Sign In">
                    <span class="loading loading-spinner loading-sm"></span>
                    Authenticating...
                  </Show>
                </button>
              </div>
            </form>

            <div class="divider text-xs text-base-content/40 my-3">or</div>

            <button
              class="btn btn-ghost btn-sm w-full"
              onClick={handleContinueAnonymous}
              disabled={hashing()}
            >
              Continue as ~{nickname()}
            </button>
          </div>

          {/* Footer hint */}
          <div class="p-3 border-t border-base-300">
            <div class="text-xs text-base-content/50 font-mono text-center">
              [Enter] Sign in · [Esc] Continue anonymous
            </div>
          </div>
        </div>
      </div>
    </Show>
  )
}

export default PasswordModal
