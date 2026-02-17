import { Component, Show, For, createSignal, onMount, onCleanup, createMemo } from 'solid-js'
import { store, storeActions, ModalState } from '../store/app-store'
import { selectors } from '../store/selectors'
import { getProtocolBridge } from '../lib/protocol-bridge'
import { DM_TARGET_BY_USER_ID, DM_TARGET_BY_SESSION_ID } from '../SuperChatCodec'
import type { PresenceEntry } from '../store/app-store'

const StartDMModal: Component = () => {
  const [search, setSearch] = createSignal('')
  const [selectedIndex, setSelectedIndex] = createSignal(0)
  let searchInputRef: HTMLInputElement | undefined

  const isOpen = () => store.activeModal === ModalState.StartDM
  const users = selectors.onlineUsersForDM

  const filteredUsers = createMemo(() => {
    const q = search().toLowerCase()
    const all = users()
    if (!q) return all
    return all.filter(u => u.nickname.toLowerCase().includes(q))
  })

  const handleSearchInput = (value: string) => {
    setSearch(value)
    setSelectedIndex(0)
  }

  const handleSelect = (user: PresenceEntry) => {
    storeActions.closeModal()
    const client = getProtocolBridge().getClient()
    if (user.isRegistered && user.userId !== null) {
      client.startDM(DM_TARGET_BY_USER_ID, user.userId, null, false)
    } else {
      client.startDM(DM_TARGET_BY_SESSION_ID, user.sessionId, null, false)
    }
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (!isOpen()) return

    switch (e.key) {
      case 'ArrowUp':
        e.preventDefault()
        setSelectedIndex(i => Math.max(0, i - 1))
        break
      case 'ArrowDown':
        e.preventDefault()
        setSelectedIndex(i => Math.min(filteredUsers().length - 1, i + 1))
        break
      case 'Enter': {
        e.preventDefault()
        const list = filteredUsers()
        if (list.length > 0 && selectedIndex() < list.length) {
          handleSelect(list[selectedIndex()])
        }
        break
      }
      case 'Escape':
        e.preventDefault()
        storeActions.closeModal()
        break
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown)
  })

  onCleanup(() => {
    window.removeEventListener('keydown', handleKeyDown)
  })

  // Focus search input when modal opens
  createMemo(() => {
    if (isOpen()) {
      setSearch('')
      setSelectedIndex(0)
      setTimeout(() => searchInputRef?.focus(), 50)
    }
  })

  return (
    <Show when={isOpen()}>
      <div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
        <div class="bg-base-100 rounded-lg shadow-xl w-full max-w-md mx-4 flex flex-col max-h-[70vh]">
          {/* Header */}
          <div class="p-4 border-b border-base-300">
            <h2 class="text-lg font-bold">Start Direct Message</h2>
            <input
              ref={searchInputRef}
              type="text"
              placeholder="Search users..."
              value={search()}
              onInput={(e) => handleSearchInput(e.currentTarget.value)}
              class="input input-bordered input-sm w-full mt-2"
              autofocus
            />
          </div>

          {/* User List */}
          <div class="flex-1 overflow-y-auto p-2">
            <Show
              when={filteredUsers().length > 0}
              fallback={
                <div class="p-4 text-center text-base-content/50">
                  <Show when={users().length === 0} fallback={<span>No matching users</span>}>
                    <span>No other users online</span>
                  </Show>
                </div>
              }
            >
              <For each={filteredUsers()}>
                {(user, index) => (
                  <button
                    onClick={() => handleSelect(user)}
                    class={`btn btn-ghost w-full justify-start text-left gap-2 ${
                      selectedIndex() === index() ? 'btn-active' : ''
                    }`}
                  >
                    <span class="text-primary font-mono">
                      {user.isRegistered ? '@' : '~'}
                    </span>
                    <span class="truncate">{user.nickname}</span>
                    <Show when={user.isRegistered}>
                      <span class="badge badge-xs badge-outline ml-auto">registered</span>
                    </Show>
                  </button>
                )}
              </For>
            </Show>
          </div>

          {/* Footer */}
          <div class="p-3 border-t border-base-300 flex justify-between items-center">
            <div class="text-xs text-base-content/50 font-mono space-x-3">
              <span>[Up/Down] Navigate</span>
              <span>[Enter] Select</span>
              <span>[Esc] Cancel</span>
            </div>
            <button class="btn btn-ghost btn-sm" onClick={() => storeActions.closeModal()}>
              Cancel
            </button>
          </div>
        </div>
      </div>
    </Show>
  )
}

export default StartDMModal
