import { Component, createEffect, onCleanup, onMount, For, Show, createSignal, createMemo } from 'solid-js'
import { getProtocolBridge, destroyProtocolBridge } from './lib/protocol-bridge'
import { store, storeActions, ViewState, ModalState, FocusArea } from './store/app-store'
import { selectors } from './store/selectors'
import ChatView from './components/ChatView'
import ThreadList from './components/ThreadList'
import ThreadDetail from './components/ThreadDetail'
import ServerSelector from './components/ServerSelector'
import ComposeModal from './components/ComposeModal'
import StartDMModal from './components/StartDMModal'
import DMRequestModal from './components/DMRequestModal'
import EncryptionSetupModal from './components/EncryptionSetupModal'
import PasswordModal from './components/PasswordModal'
import { DM_TARGET_BY_USER_ID, DM_TARGET_BY_SESSION_ID, type Message } from './SuperChatCodec'
import { encryptMessage } from './lib/crypto'
import {
  CommandExecutor,
  ViewState as CmdViewState,
  ModalState as CmdModalState,
  useKeyboardShortcuts,
  useFooterShortcuts
} from './lib/commands'

const App: Component = () => {
  let bridge = getProtocolBridge()
  let client = bridge.getClient()
  let messageScrollContainer: HTMLDivElement | undefined
  let chatInputRef: HTMLTextAreaElement | undefined

  // Track if user has manually scrolled up
  const [userHasScrolledUp, setUserHasScrolledUp] = createSignal(false)

  // Chat input state (inline input for chat channels)
  const [chatInput, setChatInput] = createSignal('')

  // Initialize with connection screen
  const isConnected = selectors.isConnected
  const isConnecting = selectors.isConnecting
  const channels = selectors.channelsArray
  const currentChannel = selectors.currentChannel
  const currentChannelMessages = selectors.currentChannelMessages
  const currentThreadList = selectors.currentThreadList
  const currentThread = selectors.currentThread
  const isCurrentChannelChat = selectors.isCurrentChannelChat
  const isCurrentChannelForum = selectors.isCurrentChannelForum
  const formattedTrafficStats = selectors.formattedTrafficStats
  const dmChannels = selectors.dmChannelsArray
  const isCurrentChannelDM = selectors.isCurrentChannelDM
  const currentDMChannel = selectors.currentDMChannel
  const onlineUsers = selectors.onlineUsers
  // Flatten thread messages for keyboard navigation (root + all nested replies in display order)
  const flattenedThreadMessages = createMemo(() => {
    const thread = currentThread()
    if (!thread) return []

    const result: Message[] = []

    // Helper to flatten recursively
    const flatten = (msg: typeof thread) => {
      result.push(msg)
      for (const reply of msg.replies) {
        flatten(reply)
      }
    }

    flatten(thread)
    return result
  })

  // Get selected message ID based on current view and selection index
  const selectedMessageId = createMemo((): bigint | null => {
    if (store.focusArea !== FocusArea.Content) return null

    if (store.currentView === ViewState.ThreadDetail) {
      const messages = flattenedThreadMessages()
      if (store.selectedMessageIndex >= 0 && store.selectedMessageIndex < messages.length) {
        return messages[store.selectedMessageIndex].message_id
      }
    }
    return null
  })

  // Command executor for keyboard shortcuts
  const commandExecutor: CommandExecutor = {
    getCurrentView: () => {
      // Map store ViewState to command ViewState
      if (!isConnected()) return CmdViewState.ChannelList
      if (!currentChannel()) return CmdViewState.ChannelList
      if (isCurrentChannelChat()) return CmdViewState.ChatView
      if (store.currentView === ViewState.ThreadDetail) return CmdViewState.ThreadDetail
      return CmdViewState.ThreadList
    },
    getActiveModal: () => {
      // Map store ModalState to command ModalState
      switch (store.activeModal) {
        case ModalState.Help: return CmdModalState.Help
        case ModalState.Compose: return CmdModalState.Compose
        case ModalState.ServerSelector: return CmdModalState.ServerSelector
        case ModalState.ConfirmDelete: return CmdModalState.ConfirmDelete
        case ModalState.StartDM: return CmdModalState.StartDM
        case ModalState.DMRequest: return CmdModalState.DMRequest
        case ModalState.EncryptionSetup: return CmdModalState.EncryptionSetup
        default: return CmdModalState.None
      }
    },
    hasSelectedChannel: () => store.activeChannelId !== null,
    hasSelectedMessage: () => store.selectedMessageIndex >= 0,
    hasSelectedThread: () => currentThreadList().length > 0 && store.selectedMessageIndex >= 0,
    hasComposeContent: () => store.compose.content.trim().length > 0,
    canGoBack: () => {
      // Can go back if:
      // - A modal is open (close it)
      // - In thread detail (go to thread list)
      // - In content focus area (switch to sidebar)
      // - In sidebar with channel selected (deselect channel)
      return store.activeModal !== ModalState.None ||
             store.currentView === ViewState.ThreadDetail ||
             store.focusArea === FocusArea.Content ||
             store.activeChannelId !== null
    },
    isAdmin: () => false, // TODO: Track admin status
    isConnected: () => isConnected()
  }

  // Get footer shortcuts text
  const footerShortcuts = useFooterShortcuts(commandExecutor)

  // Handle keyboard commands
  const handleCommand = (actionId: string) => {
    console.log('[Keyboard] Command:', actionId)

    switch (actionId) {
      case 'help':
        storeActions.openModal(ModalState.Help)
        break

      case 'quit':
        handleDisconnect()
        break

      case 'go-back': {
        // Helper to deselect channel and return to channel list
        const deselectChannel = () => {
          if (store.subscribedChannelId !== null) {
            client.unsubscribeChannel(store.subscribedChannelId)
            store.setSubscribedChannelId(null)
          }
          store.setActiveChannelId(null)
          store.setCurrentView(ViewState.ChannelList)
          store.setFocusArea(FocusArea.Sidebar)
          storeActions.clearMessages()
        }

        if (store.activeModal !== ModalState.None) {
          storeActions.closeModal()
        } else if (store.compose.replyToId !== null) {
          storeActions.clearCompose()
        } else if (store.focusArea === FocusArea.Content) {
          // Escape from content area
          if (store.currentView === ViewState.ThreadDetail) {
            // Forum: go back to thread list
            handleBackToThreadList()
          } else if (isCurrentChannelChat()) {
            // Chat channel: go directly back to channel list (no thread list to navigate)
            deselectChannel()
          } else {
            // Forum thread list: switch focus to sidebar first
            store.setFocusArea(FocusArea.Sidebar)
          }
        } else {
          // Already in sidebar, deselect channel
          if (store.activeChannelId !== null) {
            deselectChannel()
          }
        }
        break
      }

      case 'navigate-up':
        if (store.focusArea === FocusArea.Sidebar) {
          const newIndex = Math.max(0, store.selectedChannelIndex - 1)
          store.setSelectedChannelIndex(newIndex)
        } else {
          const newIndex = Math.max(0, store.selectedMessageIndex - 1)
          store.setSelectedMessageIndex(newIndex)
        }
        break

      case 'navigate-down':
        if (store.focusArea === FocusArea.Sidebar) {
          const maxIndex = channels().length - 1
          const newIndex = Math.min(maxIndex, store.selectedChannelIndex + 1)
          store.setSelectedChannelIndex(newIndex)
        } else {
          // Determine max based on view
          let maxIndex = 0
          if (isCurrentChannelChat()) {
            maxIndex = currentChannelMessages().length - 1
          } else if (store.currentView === ViewState.ThreadList) {
            maxIndex = currentThreadList().length - 1
          } else if (store.currentView === ViewState.ThreadDetail) {
            // Use flattened messages for proper navigation through nested structure
            maxIndex = flattenedThreadMessages().length - 1
          }
          const newIndex = Math.min(Math.max(0, maxIndex), store.selectedMessageIndex + 1)
          store.setSelectedMessageIndex(newIndex)
        }
        break

      case 'select':
        if (store.focusArea === FocusArea.Sidebar) {
          const channelsList = channels()
          if (channelsList.length > 0 && store.selectedChannelIndex < channelsList.length) {
            const channel = channelsList[store.selectedChannelIndex]
            handleJoinChannel(channel.channel_id)
            store.setFocusArea(FocusArea.Content)
            store.setSelectedMessageIndex(0)
          }
        } else if (store.currentView === ViewState.ThreadList) {
          const threads = currentThreadList()
          if (threads.length > 0 && store.selectedMessageIndex < threads.length) {
            const thread = threads[store.selectedMessageIndex]
            handleThreadClick(thread.message_id)
            store.setSelectedMessageIndex(0)
          }
        }
        break

      case 'switch-focus':
        storeActions.toggleFocus()
        break

      case 'compose-new-thread':
        if (currentChannel()) {
          // Clear any reply context and open compose modal
          storeActions.clearCompose()
          storeActions.openModal(ModalState.Compose)
        }
        break

      case 'compose-reply':
        if (store.currentView === ViewState.ThreadDetail) {
          // Get the selected message from flattened list and set up reply
          const messages = flattenedThreadMessages()
          const idx = store.selectedMessageIndex
          if (idx >= 0 && idx < messages.length) {
            const msg = messages[idx]
            handleReply(msg.message_id, msg)
          }
        }
        break

      case 'focus-compose':
        if (currentChannel()) {
          storeActions.openModal(ModalState.Compose)
        }
        break

      case 'start-dm':
        storeActions.openModal(ModalState.StartDM)
        break

      default:
        console.log('[Keyboard] Unhandled command:', actionId)
    }
  }

  // Set up keyboard shortcuts
  useKeyboardShortcuts(
    commandExecutor,
    handleCommand,
    () => isConnected() || store.activeModal === ModalState.Help
  )

  // Cleanup on unmount
  onCleanup(() => {
    client.disconnect()
    destroyProtocolBridge()
  })

  // Auto-connect on page load if saved credentials exist
  onMount(() => {
    const savedUrl = localStorage.getItem('superchat_last_url')
    const savedNickname = localStorage.getItem('superchat_nickname')
    const savedThrottle = localStorage.getItem('superchat_throttle_speed')

    if (savedUrl && savedNickname) {
      const throttleBps = savedThrottle ? parseInt(savedThrottle, 10) : 0
      handleConnect(savedUrl, savedNickname, throttleBps)
    }
  })

  // Auto-request message list when channel changes
  createEffect(() => {
    const channelId = store.activeChannelId
    if (channelId !== null) {
      console.log('Active channel changed to:', channelId)
      // Reset scroll tracking when changing channels
      setUserHasScrolledUp(false)
      // Request messages for this channel
      client.listMessages(channelId, 0n, 100)
      // Subscribe to real-time updates
      client.subscribeChannel(channelId)
    }
  })

  // Auto-focus chat input when entering a chat channel
  createEffect(() => {
    const channel = currentChannel()
    if (channel && channel.type === 0 && chatInputRef) {
      // Small delay to ensure DOM is ready
      setTimeout(() => chatInputRef?.focus(), 100)
    }
  })

  // Clear chat input when leaving a channel
  createEffect(() => {
    if (store.activeChannelId === null) {
      setChatInput('')
    }
  })

  // Auto-scroll to bottom when messages change
  createEffect(() => {
    const messages = currentChannelMessages()
    const container = messageScrollContainer

    if (!container || messages.length === 0) return

    // Check if user is near the bottom (within 100px)
    const isNearBottom = () => {
      const threshold = 100
      return container.scrollHeight - container.scrollTop - container.clientHeight < threshold
    }

    // Always scroll on initial load or if user is near bottom
    if (!userHasScrolledUp() || isNearBottom()) {
      // Use setTimeout to ensure DOM has updated, and a longer delay for initial load
      setTimeout(() => {
        container.scrollTop = container.scrollHeight
      }, 50)
    }
  })

  // Track user scroll behavior
  const handleScroll = (e: Event) => {
    const container = e.target as HTMLDivElement
    const threshold = 100
    const isAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < threshold

    setUserHasScrolledUp(!isAtBottom)
  }

  const handleConnect = (url: string, nickname: string, throttleBps: number) => {
    console.log('Connecting:', { url, nickname, throttleBps })
    store.setServerUrl(url)
    store.setNickname(nickname)
    storeActions.updateTraffic({ throttleBytesPerSecond: throttleBps })

    client.connect(url, nickname)
  }

  const handleDisconnect = () => {
    // Unsubscribe from current channel if any
    if (store.subscribedChannelId !== null) {
      client.unsubscribeChannel(store.subscribedChannelId)
    }

    // Clear saved URL so next page load shows ServerSelector instead of auto-connecting
    localStorage.removeItem('superchat_last_url')

    client.disconnect()
    store.setNickname('')
    store.setServerUrl('')
    storeActions.resetConnection()
  }

  const handleJoinChannel = (channelId: bigint) => {
    // Unsubscribe from previous channel
    if (store.subscribedChannelId !== null && store.subscribedChannelId !== channelId) {
      client.unsubscribeChannel(store.subscribedChannelId)
    }

    // Reset view state
    store.setCurrentView(ViewState.ThreadList)
    store.setActiveThreadId(null)
    storeActions.clearCompose()

    client.joinChannel(channelId)
  }

  const handleThreadClick = (threadId: bigint) => {
    console.log('Opening thread:', threadId)
    store.setActiveThreadId(threadId)
    store.setCurrentView(ViewState.ThreadDetail)

    // Subscribe to thread updates
    client.subscribeThread(threadId)

    // Load replies for this thread
    if (store.activeChannelId !== null) {
      client.listMessagesForThread(store.activeChannelId, threadId, 100)
    }
  }

  const handleBackToThreadList = () => {
    // Unsubscribe from thread
    if (store.subscribedThreadId !== null) {
      client.unsubscribeThread(store.subscribedThreadId)
    }

    store.setActiveThreadId(null)
    store.setCurrentView(ViewState.ThreadList)
    storeActions.clearCompose()
  }

  const handleReply = (messageId: bigint, message: Message) => {
    console.log('Replying to:', messageId)
    storeActions.updateCompose({
      replyToId: messageId,
      replyToMessage: message
    })
    storeActions.openModal(ModalState.Compose)
  }

  const handleComposeSend = async (content: string) => {
    if (!currentChannel()) return

    const channelId = currentChannel()!.channel_id
    const key = store.dmChannelKeys.get(channelId)

    if (key) {
      // Encrypted DM: encrypt and send raw bytes
      const plaintext = new TextEncoder().encode(content)
      const encrypted = await encryptMessage(key, plaintext)
      client.postMessageRaw(channelId, encrypted, store.compose.replyToId)
    } else {
      client.postMessage(channelId, content, store.compose.replyToId)
    }

    storeActions.clearCompose()
    storeActions.closeModal()
  }

  const handleComposeCancel = () => {
    storeActions.clearCompose()
    storeActions.closeModal()
  }

  // Send chat message (inline input, not modal)
  const handleChatSend = async () => {
    const content = chatInput().trim()
    if (!content || !currentChannel()) return

    const channelId = currentChannel()!.channel_id
    const key = store.dmChannelKeys.get(channelId)

    if (key) {
      const plaintext = new TextEncoder().encode(content)
      const encrypted = await encryptMessage(key, plaintext)
      client.postMessageRaw(channelId, encrypted, null)
    } else {
      client.postMessage(channelId, content, null)
    }

    setChatInput('')
  }

  // Handle keydown in chat input
  const handleChatInputKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleChatSend()
    }
    // Escape is handled by the global keyboard handler (go-back)
  }

  const handleJoinDMChannel = (channelId: bigint) => {
    // Clear unread count
    const dm = store.dmChannels.get(channelId)
    if (dm) {
      storeActions.addDMChannel({ ...dm, unreadCount: 0 })
    }

    // Join the DM channel (treated as a regular channel join)
    store.setCurrentView(ViewState.ChatView)
    store.setActiveThreadId(null)
    storeActions.clearCompose()
    client.joinChannel(channelId)
  }

  const handleLeaveDMPermanent = (channelId: bigint) => {
    client.leaveChannel(channelId, true)
    storeActions.removeDMChannel(channelId)
    storeActions.removeDMChannelKey(channelId)

    // If we were viewing this DM, go back to channel list
    if (store.activeChannelId === channelId) {
      store.setActiveChannelId(null)
      store.setCurrentView(ViewState.ChannelList)
      store.setFocusArea(FocusArea.Sidebar)
      storeActions.clearMessages()
    }
  }

  return (
    <div class="min-h-screen bg-base-100 flex flex-col overflow-hidden">
      {/* Server Selector - First Screen */}
      <Show when={!isConnected() && !isConnecting()}>
        <ServerSelector onConnect={handleConnect} />
      </Show>

      {/* Connecting State */}
      <Show when={isConnecting()}>
        <div class="fixed inset-0 bg-black/95 flex items-center justify-center z-50">
          <div class="text-center">
            <span class="loading loading-spinner loading-lg text-primary"></span>
            <p class="mt-4 text-lg">Connecting to server...</p>
          </div>
        </div>
      </Show>

      {/* Main App UI */}
      <Show when={isConnected()}>
        <div class="flex h-screen overflow-hidden">
          {/* Sidebar - Channel List */}
          <div class="w-64 bg-base-200 border-r border-base-300 flex flex-col">
            <div class="p-4 border-b border-base-300 flex-shrink-0">
              <div class="flex items-center justify-between mb-2">
                <div>
                  <h2 class="font-bold text-lg">SuperChat</h2>
                  <p class="text-sm text-base-content/70">{store.isRegistered ? '' : '~'}{store.nickname}</p>
                </div>
                <button
                  onClick={handleDisconnect}
                  class="btn btn-ghost btn-sm"
                  title="Disconnect"
                >
                  ✕
                </button>
              </div>
              {/* Traffic Stats */}
              <div class="text-xs text-base-content/50 font-mono">
                {formattedTrafficStats()}
              </div>
            </div>

            <div class="p-4 flex-1 overflow-y-auto">
              {/* DM Channels */}
              <Show when={dmChannels().length > 0 || Array.from(store.outgoingDMInvites.values()).length > 0 || Array.from(store.pendingDMInvites.values()).length > 0}>
                <h3 class="font-semibold text-sm uppercase text-base-content/70 mb-2">
                  Direct Messages
                </h3>
                <div class="space-y-1 mb-4">
                  {/* Active DM channels */}
                  <For each={dmChannels()}>
                    {(dm) => {
                      const isSelected = () => store.activeChannelId === dm.channelId
                      return (
                        <div class="flex items-center gap-1">
                          <button
                            onClick={() => handleJoinDMChannel(dm.channelId)}
                            class={`btn btn-ghost btn-sm flex-1 justify-start text-left gap-1 ${
                              isSelected() ? 'btn-active' : ''
                            }`}
                          >
                            <span class="text-xs">
                              {dm.isEncrypted ? '\u{1F512}' : '\u{2709}'}
                            </span>
                            <span class="truncate">{dm.otherNickname}</span>
                            <Show when={dm.participantLeft}>
                              <span class="badge badge-xs badge-warning">left</span>
                            </Show>
                            <Show when={dm.unreadCount > 0}>
                              <span class="badge badge-xs badge-primary">{dm.unreadCount}</span>
                            </Show>
                          </button>
                          <button
                            onClick={() => handleLeaveDMPermanent(dm.channelId)}
                            class="btn btn-ghost btn-xs opacity-50 hover:opacity-100"
                            title="Leave DM permanently"
                          >
                            x
                          </button>
                        </div>
                      )
                    }}
                  </For>

                  {/* Outgoing DM invites (waiting for acceptance) */}
                  <For each={Array.from(store.outgoingDMInvites.values())}>
                    {(invite) => (
                      <div class="flex items-center gap-2 px-3 py-1 text-sm text-base-content/50">
                        <span class="loading loading-spinner loading-xs"></span>
                        <span class="truncate">{invite.toNickname}</span>
                        <span class="text-xs">(pending)</span>
                      </div>
                    )}
                  </For>

                  {/* Pending incoming DM requests */}
                  <For each={Array.from(store.pendingDMInvites.values())}>
                    {(invite) => (
                      <button
                        onClick={() => {
                          store.setActiveDMInvite(invite)
                          storeActions.openModal(ModalState.DMRequest)
                        }}
                        class="btn btn-ghost btn-sm w-full justify-start text-left gap-1 text-warning"
                      >
                        <span class="text-xs">!</span>
                        <span class="truncate">{invite.fromNickname}</span>
                        <span class="badge badge-xs badge-warning">request</span>
                      </button>
                    )}
                  </For>
                </div>
              </Show>

              <h3 class="font-semibold text-sm uppercase text-base-content/70 mb-2">
                Channels
              </h3>
              <div class="space-y-1">
                <For each={channels()}>
                  {(channel, index) => {
                    const isSelected = () => store.activeChannelId === channel.channel_id
                    const isKeyboardSelected = () =>
                      store.focusArea === FocusArea.Sidebar && store.selectedChannelIndex === index()

                    return (
                      <button
                        onClick={() => {
                          store.setSelectedChannelIndex(index())
                          handleJoinChannel(channel.channel_id)
                        }}
                        class={`btn btn-ghost w-full justify-start text-left ${
                          isSelected() ? 'btn-active' : ''
                        } ${isKeyboardSelected() ? 'ring-2 ring-primary ring-offset-1' : ''}`}
                      >
                        <span class="font-mono text-primary">
                          {channel.type === 0 ? '>' : '#'}
                        </span>
                        <span class="truncate">{channel.name}</span>
                      </button>
                    )
                  }}
                </For>
              </div>
            </div>
          </div>

          {/* Main Content Area */}
          <div class="flex-1 flex flex-col overflow-hidden">
            <Show
              when={currentChannel()}
              fallback={
                <div class="flex-1 flex items-center justify-center p-8">
                  <div class="text-center text-base-content/50">
                    <h3 class="text-xl font-semibold mb-2">Select a channel to start chatting</h3>
                    <p>Choose a channel from the sidebar</p>
                  </div>
                </div>
              }
            >
              {/* Channel Header */}
              <div class="border-b border-base-300 p-4 flex items-center justify-between flex-shrink-0">
                <div class="flex items-center gap-2">
                  {/* Back button for thread detail view */}
                  <Show when={isCurrentChannelForum() && store.currentView === ViewState.ThreadDetail}>
                    <button
                      onClick={handleBackToThreadList}
                      class="btn btn-ghost btn-sm btn-circle"
                      title="Back to thread list"
                    >
                      ←
                    </button>
                  </Show>

                  <span class="font-mono text-primary text-lg">
                    {currentChannel()!.type === 0 ? '>' : '#'}
                  </span>
                  <h3 class="font-bold text-lg">{currentChannel()!.name}</h3>
                  <Show when={store.subscribedChannelId === currentChannel()!.channel_id}>
                    <span class="badge badge-success badge-sm">Live</span>
                  </Show>
                  <Show when={isCurrentChannelDM()}>
                    <Show when={currentDMChannel()?.isEncrypted}>
                      <span class="badge badge-info badge-sm gap-1" title="Ephemeral encryption - keys lost on page close">{'\u{1F512}'} Encrypted (session only)</span>
                    </Show>
                    <Show when={currentDMChannel()?.participantLeft}>
                      <span class="badge badge-warning badge-sm">Partner left</span>
                    </Show>
                  </Show>
                </div>
                <div class="text-sm text-base-content/70">
                  <Show
                    when={isCurrentChannelChat()}
                    fallback={
                      <span>{currentThreadList().length} threads</span>
                    }
                  >
                    <span>{currentChannelMessages().length} messages</span>
                  </Show>
                </div>
              </div>

              {/* Message Display Area - Conditional rendering based on channel type and view */}
              <Show when={isCurrentChannelChat()}>
                {/* Chat Channel: Flat message list + inline input */}
                <div class="relative flex-1 flex flex-col overflow-hidden">
                  <div
                    ref={messageScrollContainer}
                    onScroll={handleScroll}
                    class="flex-1 overflow-auto"
                  >
                    <ChatView
                      messages={currentChannelMessages()}
                    />
                  </div>
                  {/* Scroll indicator */}
                  <Show when={userHasScrolledUp()}>
                    <div class="absolute bottom-16 right-4 badge badge-primary gap-2 pointer-events-none">
                      <span>↓</span>
                      <span>Scrolled up</span>
                    </div>
                  </Show>
                </div>
                {/* Inline chat input */}
                <div class="border-t border-base-300 p-3 bg-base-100">
                  <div class="flex gap-2">
                    <textarea
                      ref={chatInputRef}
                      value={chatInput()}
                      onInput={(e) => setChatInput(e.currentTarget.value)}
                      onKeyDown={handleChatInputKeyDown}
                      placeholder="Type a message..."
                      rows={2}
                      class="textarea textarea-bordered flex-1 resize-none focus:textarea-primary"
                    />
                    <button
                      onClick={handleChatSend}
                      disabled={!chatInput().trim()}
                      class="btn btn-primary self-end"
                    >
                      Send
                    </button>
                  </div>
                </div>
              </Show>

              <Show when={isCurrentChannelForum()}>
                <Show
                  when={store.currentView === ViewState.ThreadDetail}
                  fallback={
                    /* Thread List View */
                    <ThreadList
                      threads={currentThreadList()}
                      onThreadClick={handleThreadClick}
                      selectedIndex={store.selectedMessageIndex}
                      isFocused={store.focusArea === FocusArea.Content}
                    />
                  }
                >
                  {/* Thread Detail View */}
                  <ThreadDetail
                    thread={currentThread()}
                    onReply={handleReply}
                    onBack={handleBackToThreadList}
                    selectedMessageId={selectedMessageId()}
                    isFocused={store.focusArea === FocusArea.Content}
                  />
                </Show>
              </Show>

            </Show>

            {/* Keyboard shortcuts footer */}
            <div class="border-t border-base-300 px-4 py-2 flex-shrink-0 bg-base-200">
              <div class="flex justify-between items-center">
                <div class="text-xs text-base-content/60 font-mono">
                  {footerShortcuts()}
                </div>
                <div class="text-xs text-base-content/40">
                  <Show when={store.focusArea === FocusArea.Sidebar}>
                    <span class="badge badge-outline badge-xs">Sidebar</span>
                  </Show>
                  <Show when={store.focusArea === FocusArea.Content}>
                    <span class="badge badge-outline badge-xs">Content</span>
                  </Show>
                </div>
              </div>
            </div>
          </div>

          {/* Right Panel - Online Users */}
          <div class="w-48 bg-base-200 border-l border-base-300 flex flex-col">
            <div class="p-3 border-b border-base-300 flex-shrink-0">
              <h3 class="font-semibold text-sm uppercase text-base-content/70">
                Online ({onlineUsers().length})
              </h3>
            </div>
            <div class="p-2 flex-1 overflow-y-auto">
              <Show
                when={onlineUsers().length > 0}
                fallback={
                  <div class="text-xs text-base-content/50 p-2">No users yet</div>
                }
              >
                <div class="space-y-0.5">
                  <For each={onlineUsers()}>
                    {(user) => {
                      const isSelf = () => user.sessionId === store.selfSessionId
                      const flagPrefix = () => {
                        if (user.userFlags & 1) return '$'  // admin
                        if (user.userFlags & 2) return '@'  // moderator
                        return ''
                      }
                      return (
                        <div
                          class={`flex items-center gap-1 px-2 py-1 rounded text-sm ${
                            isSelf() ? 'text-primary' : 'text-base-content/80 hover:bg-base-300 cursor-pointer'
                          }`}
                          onClick={() => {
                            if (!isSelf()) {
                              // Start DM with this user
                              const client = bridge.getClient()
                              if (user.isRegistered && user.userId !== null) {
                                client.startDM(DM_TARGET_BY_USER_ID, user.userId, null, false)
                              } else {
                                client.startDM(DM_TARGET_BY_SESSION_ID, user.sessionId, null, false)
                              }
                            }
                          }}
                          title={isSelf() ? 'You' : `Click to DM ${user.nickname}`}
                        >
                          <span class="font-mono text-xs text-primary shrink-0">
                            {flagPrefix()}{user.isRegistered ? '' : '~'}
                          </span>
                          <span class="truncate">{user.nickname}</span>
                          <Show when={isSelf()}>
                            <span class="text-xs text-base-content/40 shrink-0">(you)</span>
                          </Show>
                        </div>
                      )
                    }}
                  </For>
                </div>
              </Show>
            </div>
          </div>
        </div>
      </Show>

      {/* Help Modal */}
      <Show when={store.activeModal === ModalState.Help}>
        <div
          class="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
          onClick={() => storeActions.closeModal()}
        >
          <div
            class="bg-base-100 rounded-lg shadow-xl max-w-lg w-full mx-4 max-h-[80vh] overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div class="p-4 border-b border-base-300 flex justify-between items-center">
              <h2 class="text-lg font-bold">Keyboard Shortcuts</h2>
              <button
                onClick={() => storeActions.closeModal()}
                class="btn btn-ghost btn-sm btn-circle"
              >
                ✕
              </button>
            </div>
            <div class="p-4 overflow-y-auto max-h-[60vh]">
              <div class="space-y-4">
                <div>
                  <h3 class="font-semibold text-sm text-base-content/70 mb-2">Navigation</h3>
                  <div class="space-y-1">
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">↑/↓ or K/J</span>
                      <span class="text-base-content/70">Move selection</span>
                    </div>
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">Enter</span>
                      <span class="text-base-content/70">Select item</span>
                    </div>
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">Tab</span>
                      <span class="text-base-content/70">Switch focus (sidebar/content)</span>
                    </div>
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">Esc</span>
                      <span class="text-base-content/70">Go back / Close</span>
                    </div>
                  </div>
                </div>

                <div>
                  <h3 class="font-semibold text-sm text-base-content/70 mb-2">Messaging</h3>
                  <div class="space-y-1">
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">N</span>
                      <span class="text-base-content/70">New thread (forum)</span>
                    </div>
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">R</span>
                      <span class="text-base-content/70">Reply to message</span>
                    </div>
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">I</span>
                      <span class="text-base-content/70">Focus compose (chat)</span>
                    </div>
                  </div>
                </div>

                <div>
                  <h3 class="font-semibold text-sm text-base-content/70 mb-2">Direct Messages</h3>
                  <div class="space-y-1">
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">Ctrl+D</span>
                      <span class="text-base-content/70">Start DM with user</span>
                    </div>
                  </div>
                </div>

                <div>
                  <h3 class="font-semibold text-sm text-base-content/70 mb-2">General</h3>
                  <div class="space-y-1">
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">H or ?</span>
                      <span class="text-base-content/70">Show this help</span>
                    </div>
                    <div class="flex justify-between text-sm">
                      <span class="font-mono text-primary">Q</span>
                      <span class="text-base-content/70">Disconnect</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div class="p-4 border-t border-base-300 text-center">
              <span class="text-xs text-base-content/50">Press Esc to close</span>
            </div>
          </div>
        </div>
      </Show>

      {/* Compose Modal */}
      <ComposeModal
        isOpen={store.activeModal === ModalState.Compose}
        replyTo={store.compose.replyToMessage}
        channelName={currentChannel()?.name || ''}
        onSend={handleComposeSend}
        onCancel={handleComposeCancel}
      />

      {/* Self-contained modals (read from store) */}
      <StartDMModal />
      <DMRequestModal />
      <EncryptionSetupModal />
      <PasswordModal />
    </div>
  )
}

export default App
