// Global application store using SolidJS signals
// Manages all client state: connection, channels, messages, UI state

import { createSignal, createMemo } from 'solid-js'
import type { Channel, Message } from '../SuperChatCodec'

// Connection state
export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'error'

// View states for forum channels
export enum ViewState {
  ThreadList = 'thread-list',
  ThreadDetail = 'thread-detail'
}

// UI state for compose area
export interface ComposeState {
  content: string
  replyToId: bigint | null
  replyToMessage: Message | null
}

// Traffic statistics
export interface TrafficStats {
  bytesSent: number
  bytesReceived: number
  throttleBytesPerSecond: number
}

// Store interface
export interface AppStore {
  // Connection state
  connectionState: ConnectionState
  serverUrl: string
  nickname: string
  isRegistered: boolean
  errorMessage: string

  // Data (normalized)
  channels: Map<bigint, Channel>
  messages: Map<bigint, Message>

  // Message indexes (for efficient lookups)
  threadIndex: Map<bigint, bigint[]> // channelId -> rootMessageIds (parent_id.present === 0)
  replyIndex: Map<bigint, bigint[]> // parentId -> childMessageIds

  // UI state
  activeChannelId: bigint | null
  currentView: ViewState
  activeThreadId: bigint | null // When viewing a specific thread
  compose: ComposeState

  // Subscription tracking
  subscribedChannelId: bigint | null
  subscribedThreadId: bigint | null

  // Traffic stats
  traffic: TrafficStats

  // Server selection
  servers: Array<{
    name: string
    wsUrl: string
    wssUrl: string
    status: 'checking' | 'online' | 'offline'
    isSecure: boolean
  }>
  selectedServerIndex: number
}

// Create signals for each store property
const [connectionState, setConnectionState] = createSignal<ConnectionState>('disconnected')
const [serverUrl, setServerUrl] = createSignal<string>('')
const [nickname, setNickname] = createSignal<string>('')
const [isRegistered, setIsRegistered] = createSignal<boolean>(false)
const [errorMessage, setErrorMessage] = createSignal<string>('')

// Data stores (using Maps for O(1) lookups)
const [channels, setChannels] = createSignal<Map<bigint, Channel>>(new Map())
const [messages, setMessages] = createSignal<Map<bigint, Message>>(new Map())

// Indexes
const [threadIndex, setThreadIndex] = createSignal<Map<bigint, bigint[]>>(new Map())
const [replyIndex, setReplyIndex] = createSignal<Map<bigint, bigint[]>>(new Map())

// UI state
const [activeChannelId, setActiveChannelId] = createSignal<bigint | null>(null)
const [currentView, setCurrentView] = createSignal<ViewState>(ViewState.ThreadList)
const [activeThreadId, setActiveThreadId] = createSignal<bigint | null>(null)
const [compose, setCompose] = createSignal<ComposeState>({
  content: '',
  replyToId: null,
  replyToMessage: null
})

// Subscription tracking
const [subscribedChannelId, setSubscribedChannelId] = createSignal<bigint | null>(null)
const [subscribedThreadId, setSubscribedThreadId] = createSignal<bigint | null>(null)

// Traffic stats
const [traffic, setTraffic] = createSignal<TrafficStats>({
  bytesSent: 0,
  bytesReceived: 0,
  throttleBytesPerSecond: 0
})

// Server selection
const [servers, setServers] = createSignal<AppStore['servers']>([])
const [selectedServerIndex, setSelectedServerIndex] = createSignal<number>(-1)

// Export the store as an object with getters and setters
export const store = {
  // Connection state
  get connectionState() { return connectionState() },
  setConnectionState,

  get serverUrl() { return serverUrl() },
  setServerUrl,

  get nickname() { return nickname() },
  setNickname,

  get isRegistered() { return isRegistered() },
  setIsRegistered,

  get errorMessage() { return errorMessage() },
  setErrorMessage,

  // Data
  get channels() { return channels() },
  setChannels,

  get messages() { return messages() },
  setMessages,

  // Indexes
  get threadIndex() { return threadIndex() },
  setThreadIndex,

  get replyIndex() { return replyIndex() },
  setReplyIndex,

  // UI state
  get activeChannelId() { return activeChannelId() },
  setActiveChannelId,

  get currentView() { return currentView() },
  setCurrentView,

  get activeThreadId() { return activeThreadId() },
  setActiveThreadId,

  get compose() { return compose() },
  setCompose,

  // Subscriptions
  get subscribedChannelId() { return subscribedChannelId() },
  setSubscribedChannelId,

  get subscribedThreadId() { return subscribedThreadId() },
  setSubscribedThreadId,

  // Traffic
  get traffic() { return traffic() },
  setTraffic,

  // Servers
  get servers() { return servers() },
  setServers,

  get selectedServerIndex() { return selectedServerIndex() },
  setSelectedServerIndex,
}

// Helper actions for common operations
export const storeActions = {
  // Add or update a channel
  addChannel(channel: Channel) {
    setChannels(prev => new Map(prev).set(channel.channel_id, channel))
  },

  // Add or update multiple channels
  addChannels(channelList: Channel[]) {
    setChannels(prev => {
      const newMap = new Map(prev)
      channelList.forEach(ch => newMap.set(ch.channel_id, ch))
      return newMap
    })
  },

  // Add or update a message
  addMessage(message: Message) {
    setMessages(prev => new Map(prev).set(message.message_id, message))
  },

  // Add or update multiple messages
  addMessages(messageList: Message[]) {
    setMessages(prev => {
      const newMap = new Map(prev)
      messageList.forEach(msg => newMap.set(msg.message_id, msg))
      return newMap
    })
  },

  // Clear all messages (e.g., when leaving channel)
  clearMessages() {
    setMessages(new Map())
    setThreadIndex(new Map())
    setReplyIndex(new Map())
  },

  // Update compose state
  updateCompose(updates: Partial<ComposeState>) {
    setCompose(prev => ({ ...prev, ...updates }))
  },

  // Clear compose state
  clearCompose() {
    setCompose({
      content: '',
      replyToId: null,
      replyToMessage: null
    })
  },

  // Update traffic stats
  updateTraffic(updates: Partial<TrafficStats>) {
    setTraffic(prev => ({ ...prev, ...updates }))
  },

  // Add bytes to traffic counters
  addTrafficBytes(sent: number = 0, received: number = 0) {
    setTraffic(prev => ({
      ...prev,
      bytesSent: prev.bytesSent + sent,
      bytesReceived: prev.bytesReceived + received
    }))
  },

  // Reset connection state
  resetConnection() {
    setConnectionState('disconnected')
    setServerUrl('')
    setNickname('')
    setIsRegistered(false)
    setErrorMessage('')
    setActiveChannelId(null)
    setCurrentView(ViewState.ThreadList)
    setActiveThreadId(null)
    setSubscribedChannelId(null)
    setSubscribedThreadId(null)
    this.clearMessages()
    this.clearCompose()
  }
}
