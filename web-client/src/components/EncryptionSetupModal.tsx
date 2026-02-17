import { Component, Show, onMount, onCleanup } from 'solid-js'
import { store, storeActions, ModalState } from '../store/app-store'
import { getProtocolBridge } from '../lib/protocol-bridge'
import { generateX25519KeyPair, KEY_TYPE_GENERATED } from '../lib/crypto'

const EncryptionSetupModal: Component = () => {
  const isOpen = () => store.activeModal === ModalState.EncryptionSetup
  const reason = () => store.encryptionSetupReason
  const hasExistingKey = () => store.encryptionKeyPub !== null

  const handleGenerateKey = async () => {
    const keyPair = generateX25519KeyPair()
    store.setEncryptionKeyPub(keyPair.publicKey)
    store.setEncryptionKeyPriv(keyPair.privateKey)

    // Persist for registered users
    if (store.isRegistered) {
      try {
        localStorage.setItem('superchat-encryption-pub', btoa(String.fromCharCode(...keyPair.publicKey)))
        localStorage.setItem('superchat-encryption-priv', btoa(String.fromCharCode(...keyPair.privateKey)))
      } catch { /* localStorage not available */ }
    }

    // Send key to server
    const client = getProtocolBridge().getClient()
    client.providePublicKey(KEY_TYPE_GENERATED, keyPair.publicKey, 'web-client')
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
    storeActions.closeModal()
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (!isOpen()) return

    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        handleCancel()
        break
      case 'g':
      case 'Enter':
        e.preventDefault()
        handleGenerateKey()
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
      <div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
        <div class="bg-base-100 rounded-lg shadow-xl w-full max-w-md mx-4">
          {/* Header */}
          <div class="p-4 border-b border-base-300">
            <h2 class="text-lg font-bold">Encryption Setup</h2>
          </div>

          {/* Content */}
          <div class="p-4 space-y-3">
            <Show when={reason()}>
              <div class="alert alert-warning text-sm py-2">
                <span>{reason()}</span>
              </div>
            </Show>

            <p class="text-sm text-base-content/70">
              To enable end-to-end encryption for DMs, an X25519 key pair needs to be generated.
              Messages will be encrypted with AES-256-GCM so only you and your DM partner can read them.
            </p>

            <Show when={hasExistingKey()}>
              <div class="alert alert-success text-sm py-2">
                <span>You already have an encryption key. It will be used automatically.</span>
              </div>
            </Show>
          </div>

          {/* Actions */}
          <div class="p-4 border-t border-base-300 flex flex-col gap-2">
            <button
              class="btn btn-primary w-full"
              onClick={handleGenerateKey}
            >
              Generate Encryption Key (G)
            </button>
            <button
              class="btn btn-ghost btn-sm w-full"
              onClick={handleSkipEncryption}
            >
              Skip - Continue Without Encryption (S)
            </button>
          </div>

          {/* Footer */}
          <div class="px-4 pb-3 text-center">
            <span class="text-xs text-base-content/50">[Esc] Cancel</span>
          </div>
        </div>
      </div>
    </Show>
  )
}

export default EncryptionSetupModal
