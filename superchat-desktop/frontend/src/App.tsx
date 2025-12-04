import { Component, createEffect, onCleanup, For, Show, createSignal } from 'solid-js'
import { getProtocolBridge, destroyProtocolBridge } from './lib/protocol-bridge'
import { store, storeActions, ViewState } from './store/app-store'
import { selectors } from './store/selectors'
import ChatView from './components/ChatView'
import ThreadList from './components/ThreadList'
import ThreadDetail from './components/ThreadDetail'
import ServerSelector from './components/ServerSelector'
import type { Message } from './SuperChatCodec'

const App: Component = () => {
  let bridge = getProtocolBridge()
  let client = bridge.getClient()
  let messageScrollContainer: HTMLDivElement | undefined

  // Track if user has manually scrolled up
  const [userHasScrolledUp, setUserHasScrolledUp] = createSignal(false)

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

  // Cleanup on unmount
  onCleanup(() => {
    client.disconnect()
    destroyProtocolBridge()
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

    client.disconnect()
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
                  <p class="text-sm text-base-content/70">{store.nickname}</p>
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
              <h3 class="font-semibold text-sm uppercase text-base-content/70 mb-2">
                Channels
              </h3>
              <div class="space-y-1">
                <For each={channels()}>
                  {(channel) => (
                    <button
                      onClick={() => handleJoinChannel(channel.channel_id)}
                      class={`btn btn-ghost w-full justify-start text-left ${
                        store.activeChannelId === channel.channel_id ? 'btn-active' : ''
                      }`}
                    >
                      <span class="font-mono text-primary">
                        {channel.type === 0 ? '>' : '#'}
                      </span>
                      <span class="truncate">{channel.name}</span>
                    </button>
                  )}
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
                {/* Chat Channel: Flat message list */}
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
                    <div class="absolute bottom-4 right-4 badge badge-primary gap-2 pointer-events-none">
                      <span>↓</span>
                      <span>Scrolled up</span>
                    </div>
                  </Show>
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
                    />
                  }
                >
                  {/* Thread Detail View */}
                  <ThreadDetail
                    thread={currentThread()}
                    onReply={handleReply}
                    onBack={handleBackToThreadList}
                  />
                </Show>
              </Show>

              {/* Compose Area */}
              <Show when={
                // Chat channels: always show
                isCurrentChannelChat() ||
                // Forum thread list: show for new threads
                (isCurrentChannelForum() && store.currentView === ViewState.ThreadList) ||
                // Forum thread detail: only show when replying
                (isCurrentChannelForum() && store.currentView === ViewState.ThreadDetail && store.compose.replyToId !== null)
              }>
                <div class="border-t border-base-300 p-4 flex-shrink-0">
                  {/* Reply Context */}
                  <Show when={store.compose.replyToMessage}>
                    <div class="mb-2 p-2 bg-base-200 rounded-lg flex items-center justify-between">
                      <div class="flex-1">
                        <span class="text-xs text-base-content/70">Replying to </span>
                        <span class="text-xs font-semibold">{store.compose.replyToMessage!.author_nickname}</span>
                        <span class="text-xs text-base-content/70">: </span>
                        <span class="text-xs text-base-content/60">
                          {store.compose.replyToMessage!.content.slice(0, 50)}
                          {store.compose.replyToMessage!.content.length > 50 ? '...' : ''}
                        </span>
                      </div>
                      <button
                        onClick={() => storeActions.clearCompose()}
                        class="btn btn-ghost btn-xs btn-circle"
                        title="Cancel reply"
                      >
                        ✕
                      </button>
                    </div>
                  </Show>

                  <div class="flex gap-2">
                    <input
                      type="text"
                      placeholder={
                        isCurrentChannelChat() ? 'Type a message...' :
                        store.currentView === ViewState.ThreadList ? 'Start a new conversation...' :
                        'Type your reply...'
                      }
                      class="input input-bordered flex-1"
                      value={store.compose.content}
                      onInput={(e) => storeActions.updateCompose({ content: e.currentTarget.value })}
                      onKeyPress={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey) {
                          e.preventDefault()
                          if (store.compose.content.trim() && currentChannel()) {
                            client.postMessage(
                              currentChannel()!.channel_id,
                              store.compose.content,
                              store.compose.replyToId
                            )
                          }
                        }
                      }}
                    />
                    <button
                      class="btn btn-primary"
                      disabled={!store.compose.content.trim()}
                      onClick={() => {
                        if (store.compose.content.trim() && currentChannel()) {
                          client.postMessage(
                            currentChannel()!.channel_id,
                            store.compose.content,
                            store.compose.replyToId
                          )
                        }
                      }}
                    >
                      Send
                    </button>
                  </div>
                </div>
              </Show>
            </Show>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default App
