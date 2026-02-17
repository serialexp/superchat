import { Component, Show, onMount, onCleanup, createSignal } from 'solid-js'
import { store, storeActions, ModalState } from '../store/app-store'
import { getProtocolBridge } from '../lib/protocol-bridge'
import { generateX25519KeyPair, publicKeyFromPrivate, KEY_TYPE_GENERATED } from '../lib/crypto'

/**
 * Build a .x25519 filename matching the Go TUI client convention.
 * Registered: {host}_{port}_{nickname}.x25519
 * Anonymous:  {host}_{port}_anon_{nickname}.x25519
 *
 * The TUI client uses userID instead of nickname for registered users,
 * so the user may need to rename the file — but the format is otherwise
 * directly compatible with ~/.config/superchat-client/keys/
 */
function buildKeyFilename(): string {
  const url = store.serverUrl
  const nickname = store.nickname
  const isRegistered = store.isRegistered

  let host = 'unknown'
  let port = '6465'
  try {
    const parsed = new URL(url)
    host = parsed.hostname
    port = parsed.port || (parsed.protocol === 'wss:' ? '443' : '6465')
  } catch { /* use defaults */ }

  const safeHost = host.replace(/[:/\\]/g, '_')
  const safeNick = nickname.replace(/[:/\\]/g, '_')

  if (isRegistered) {
    return `${safeHost}_${port}_${safeNick}.x25519`
  }
  return `${safeHost}_${port}_anon_${safeNick}.x25519`
}

function downloadPrivateKey() {
  const priv = store.encryptionKeyPriv
  if (!priv) return

  const blob = new Blob([priv], { type: 'application/octet-stream' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = buildKeyFilename()
  a.click()
  URL.revokeObjectURL(url)
}

const EncryptionSetupModal: Component = () => {
  const [keyJustGenerated, setKeyJustGenerated] = createSignal(false)
  const [loadError, setLoadError] = createSignal('')
  let fileInputRef: HTMLInputElement | undefined

  const isOpen = () => store.activeModal === ModalState.EncryptionSetup
  const reason = () => store.encryptionSetupReason
  const hasExistingKey = () => store.encryptionKeyPub !== null

  const activateKey = (privateKey: Uint8Array) => {
    const publicKey = publicKeyFromPrivate(privateKey)
    store.setEncryptionKeyPub(publicKey)
    store.setEncryptionKeyPriv(privateKey)

    const client = getProtocolBridge().getClient()
    client.providePublicKey(KEY_TYPE_GENERATED, publicKey, 'web-client')
  }

  const handleGenerateKey = () => {
    setLoadError('')
    const keyPair = generateX25519KeyPair()
    activateKey(keyPair.privateKey)
    setKeyJustGenerated(true)
  }

  const handleLoadKey = () => {
    fileInputRef?.click()
  }

  const handleFileSelected = (e: Event) => {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return

    // Reset for next selection
    input.value = ''

    if (file.size !== 32) {
      setLoadError(`Invalid key file: expected 32 bytes, got ${file.size}`)
      return
    }

    const reader = new FileReader()
    reader.onload = () => {
      const privateKey = new Uint8Array(reader.result as ArrayBuffer)
      if (privateKey.length !== 32) {
        setLoadError(`Invalid key file: expected 32 bytes, got ${privateKey.length}`)
        return
      }

      setLoadError('')
      activateKey(privateKey)
      storeActions.closeModal()
    }
    reader.onerror = () => {
      setLoadError('Failed to read key file')
    }
    reader.readAsArrayBuffer(file)
  }

  const handleDone = () => {
    setKeyJustGenerated(false)
    setLoadError('')
    storeActions.closeModal()
  }

  const handleSkipEncryption = () => {
    const channelId = store.pendingEncryptionChannelId
    storeActions.closeModal()
    if (channelId !== null) {
      const client = getProtocolBridge().getClient()
      client.allowUnencryptedDM(channelId, false)
    }
  }

  const handleCancel = () => {
    setKeyJustGenerated(false)
    setLoadError('')
    storeActions.closeModal()
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (!isOpen()) return

    if (keyJustGenerated()) {
      switch (e.key) {
        case 'Escape':
        case 'Enter':
          e.preventDefault()
          handleDone()
          break
        case 'd':
          e.preventDefault()
          downloadPrivateKey()
          break
      }
      return
    }

    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        handleCancel()
        break
      case 'g':
        e.preventDefault()
        handleGenerateKey()
        break
      case 'l':
        e.preventDefault()
        handleLoadKey()
        break
      case 's':
        e.preventDefault()
        handleSkipEncryption()
        break
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown)
  })

  onCleanup(() => {
    window.removeEventListener('keydown', handleKeyDown)
  })

  return (
    <Show when={isOpen()}>
      {/* Hidden file input for key loading */}
      <input
        ref={fileInputRef}
        type="file"
        accept=".x25519"
        class="hidden"
        onChange={handleFileSelected}
      />

      <div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
        <div class="bg-base-100 rounded-lg shadow-xl w-full max-w-md mx-4">
          {/* Header */}
          <div class="p-4 border-b border-base-300">
            <h2 class="text-lg font-bold">
              {keyJustGenerated() ? 'Key Generated' : 'Encryption Setup'}
            </h2>
          </div>

          {/* Content */}
          <div class="p-4 space-y-3">
            <Show when={loadError()}>
              <div class="alert alert-error text-sm py-2">
                <span>{loadError()}</span>
              </div>
            </Show>

            <Show when={keyJustGenerated()}>
              <div class="alert alert-success text-sm py-2">
                <span>Encryption key generated and sent to server.</span>
              </div>

              <p class="text-sm text-base-content/70">
                This key only exists in memory and will be lost when you close this tab.
                Download it to use with the TUI/desktop client.
              </p>

              <p class="text-xs text-base-content/50">
                Place the downloaded file in <code class="bg-base-300 px-1 rounded">~/.config/superchat-client/keys/</code> to
                use it with the TUI client.
              </p>
            </Show>

            <Show when={!keyJustGenerated()}>
              <Show when={reason()}>
                <div class="alert alert-warning text-sm py-2">
                  <span>{reason()}</span>
                </div>
              </Show>

              <p class="text-sm text-base-content/70">
                To enable end-to-end encryption for DMs, an X25519 key pair needs to be generated.
                Messages will be encrypted with AES-256-GCM so only you and your DM partner can read them.
              </p>

              <div class="alert alert-warning text-sm py-2">
                <span>Encryption keys are ephemeral in the web client. Encrypted messages will not
                be readable after you close this tab or refresh the page.</span>
              </div>

              <Show when={hasExistingKey()}>
                <div class="alert alert-success text-sm py-2">
                  <span>You already have an encryption key. It will be used automatically.</span>
                </div>
              </Show>
            </Show>
          </div>

          {/* Actions */}
          <div class="p-4 border-t border-base-300 flex flex-col gap-2">
            <Show when={keyJustGenerated()}>
              <button
                class="btn btn-primary w-full"
                onClick={downloadPrivateKey}
              >
                Download Key (.x25519) (D)
              </button>
              <button
                class="btn btn-ghost btn-sm w-full"
                onClick={handleDone}
              >
                Done (Enter)
              </button>
            </Show>

            <Show when={!keyJustGenerated()}>
              <button
                class="btn btn-primary w-full"
                onClick={handleGenerateKey}
              >
                Generate New Key (G)
              </button>
              <button
                class="btn btn-outline w-full"
                onClick={handleLoadKey}
              >
                Load Existing Key (.x25519) (L)
              </button>
              <Show when={hasExistingKey()}>
                <button
                  class="btn btn-outline btn-sm w-full"
                  onClick={downloadPrivateKey}
                >
                  Download Current Key (D)
                </button>
              </Show>
              <button
                class="btn btn-ghost btn-sm w-full"
                onClick={handleSkipEncryption}
              >
                Skip - Continue Without Encryption (S)
              </button>
            </Show>
          </div>

          {/* Footer */}
          <div class="px-4 pb-3 text-center">
            <span class="text-xs text-base-content/50">[Esc] Close</span>
          </div>
        </div>
      </div>
    </Show>
  )
}

export default EncryptionSetupModal
