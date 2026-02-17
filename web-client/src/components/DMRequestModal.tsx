import { Component, Show, onMount, onCleanup } from 'solid-js'
import { store, storeActions, ModalState } from '../store/app-store'
import { getProtocolBridge } from '../lib/protocol-bridge'
import { DM_ENCRYPTION_NOT_POSSIBLE, DM_ENCRYPTION_REQUIRED, DM_ENCRYPTION_OPTIONAL } from '../SuperChatCodec'

const DMRequestModal: Component = () => {
  const isOpen = () => store.activeModal === ModalState.DMRequest
  const invite = () => store.activeDMInvite
  const hasEncryptionKey = () => store.encryptionKeyPub !== null

  const handleAccept = (channelId: bigint) => {
    storeActions.closeModal()
    storeActions.removePendingDMInvite(channelId)
    const client = getProtocolBridge().getClient()
    client.allowUnencryptedDM(channelId, false)
  }

  const handleDecline = (channelId: bigint) => {
    storeActions.closeModal()
    storeActions.removePendingDMInvite(channelId)
    const client = getProtocolBridge().getClient()
    client.declineDM(channelId)
  }

  const handleSetupEncryption = (channelId: bigint) => {
    store.setPendingEncryptionChannelId(channelId)
    storeActions.openModal(ModalState.EncryptionSetup)
  }

  const handleCancel = () => {
    storeActions.closeModal()
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    const inv = invite()
    if (!isOpen() || !inv) return

    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        handleCancel()
        break
      case 'a':
      case 'Enter':
        e.preventDefault()
        handleAccept(inv.channelId)
        break
      case 'd':
        e.preventDefault()
        handleDecline(inv.channelId)
        break
      case 'e':
        if (inv.encryptionStatus !== DM_ENCRYPTION_NOT_POSSIBLE) {
          e.preventDefault()
          handleSetupEncryption(inv.channelId)
        }
        break
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown)
  })

  onCleanup(() => {
    window.removeEventListener('keydown', handleKeyDown)
  })

  const encryptionLabel = () => {
    const inv = invite()
    if (!inv) return ''
    switch (inv.encryptionStatus) {
      case DM_ENCRYPTION_NOT_POSSIBLE: return 'Not possible (anonymous user)'
      case DM_ENCRYPTION_REQUIRED: return 'Required'
      case DM_ENCRYPTION_OPTIONAL: return 'Available'
      default: return 'Unknown'
    }
  }

  return (
    <Show when={isOpen() && invite()}>
      {(_inv) => {
        const inv = invite()!
        return (
          <div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
            <div class="bg-base-100 rounded-lg shadow-xl w-full max-w-md mx-4">
              {/* Header */}
              <div class="p-4 border-b border-base-300">
                <h2 class="text-lg font-bold">Incoming DM Request</h2>
              </div>

              {/* Content */}
              <div class="p-4 space-y-3">
                <p class="text-base">
                  <span class="font-semibold text-primary">{inv.fromNickname}</span>
                  {' wants to start a direct message with you.'}
                </p>

                <div class="flex items-center gap-2 text-sm text-base-content/70">
                  <span>Encryption:</span>
                  <span class={
                    inv.encryptionStatus === DM_ENCRYPTION_REQUIRED ? 'text-success font-semibold' :
                    inv.encryptionStatus === DM_ENCRYPTION_OPTIONAL ? 'text-warning' :
                    'text-base-content/50'
                  }>
                    {encryptionLabel()}
                  </span>
                </div>

                <Show when={inv.encryptionStatus === DM_ENCRYPTION_OPTIONAL && !hasEncryptionKey()}>
                  <div class="alert alert-info text-sm py-2">
                    <span>You can set up encryption before accepting to enable E2E encrypted messaging.</span>
                  </div>
                </Show>
              </div>

              {/* Actions */}
              <div class="p-4 border-t border-base-300 flex flex-col gap-2">
                <div class="flex gap-2">
                  <button
                    class="btn btn-primary flex-1"
                    onClick={() => handleAccept(inv.channelId)}
                  >
                    Accept (A)
                  </button>
                  <button
                    class="btn btn-error btn-outline flex-1"
                    onClick={() => handleDecline(inv.channelId)}
                  >
                    Decline (D)
                  </button>
                </div>
                <Show when={inv.encryptionStatus !== DM_ENCRYPTION_NOT_POSSIBLE && !hasEncryptionKey()}>
                  <button
                    class="btn btn-outline btn-sm w-full"
                    onClick={() => handleSetupEncryption(inv.channelId)}
                  >
                    Setup Encryption First (E)
                  </button>
                </Show>
              </div>

              {/* Footer */}
              <div class="px-4 pb-3 text-center">
                <span class="text-xs text-base-content/50">[Esc] Cancel</span>
              </div>
            </div>
          </div>
        )
      }}
    </Show>
  )
}

export default DMRequestModal
