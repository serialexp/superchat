// Global application store using SolidJS signals
// Manages all client state: connection, channels, messages, UI state

import { createSignal, createMemo } from 'solid-js'
import type { Channel, Message } from '../SuperChatCodec'

// Connection state
export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'error'

// View states for forum channels
export enum ViewState {
  ChannelList = 'channel-list',  // Used when no channel selected
  ThreadList = 'thread-list',
  ThreadDetail = 'thread-detail',
  ChatView = 'chat-view'
}

// Modal states for keyboard navigation context
export enum ModalState {
  None = 'none',
  Compose = 'compose',
  Help = 'help',
  ServerSelector = 'server-selector',
  ConfirmDelete = 'confirm-delete',
  StartDM = 'start-dm',
  DMRequest = 'dm-request',
  EncryptionSetup = 'encryption-setup'
}

// Focus area for keyboard navigation
export enum FocusArea {
  Sidebar = 'sidebar',
  Content = 'content'
}

// UI state for compose area
export interface ComposeState {
  content: string
  replyToId: bigint | null
  replyToMessage: Message | null
}

// DM channel info
export interface DMChannel {
  channelId: bigint
  otherUserId: bigint | null
  otherNickname: string
  isEncrypted: boolean
  otherPubKey: Uint8Array | null
  unreadCount: number
  participantLeft: boolean
}

// Incoming DM invite
export interface DMInvite {
  channelId: bigint
  fromUserId: bigint | null
  fromNickname: string
  encryptionStatus: number
}

// Outgoing DM invite (waiting for acceptance)
export interface OutgoingDMInvite {
  channelId: bigint
  toUserId: bigint | null
  toNickname: string
}

// Presence entry for server roster
export interface PresenceEntry {
  sessionId: bigint
  nickname: string
  isRegistered: boolean
  userId: bigint | null
  userFlags: number
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

// Modal and focus state for keyboard navigation
const [activeModal, setActiveModal] = createSignal<ModalState>(ModalState.None)
const [focusArea, setFocusArea] = createSignal<FocusArea>(FocusArea.Sidebar)

// Selection indices for keyboard navigation
const [selectedChannelIndex, setSelectedChannelIndex] = createSignal<number>(0)
const [selectedMessageIndex, setSelectedMessageIndex] = createSignal<number>(0)

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

// DM state
const [dmChannels, setDmChannels] = createSignal<Map<bigint, DMChannel>>(new Map())
const [pendingDMInvites, setPendingDMInvites] = createSignal<Map<bigint, DMInvite>>(new Map())
const [outgoingDMInvites, setOutgoingDMInvites] = createSignal<Map<bigint, OutgoingDMInvite>>(new Map())
const [dmChannelKeys, setDmChannelKeys] = createSignal<Map<bigint, Uint8Array>>(new Map())
const [encryptionKeyPub, setEncryptionKeyPub] = createSignal<Uint8Array | null>(null)
const [encryptionKeyPriv, setEncryptionKeyPriv] = createSignal<Uint8Array | null>(null)
const [serverRoster, setServerRoster] = createSignal<Map<bigint, PresenceEntry>>(new Map())
const [selfSessionId, setSelfSessionId] = createSignal<bigint | null>(null)
const [activeDMInvite, setActiveDMInvite] = createSignal<DMInvite | null>(null)
const [pendingEncryptionChannelId, setPendingEncryptionChannelId] = createSignal<bigint | null>(null)
const [encryptionSetupReason, setEncryptionSetupReason] = createSignal<string>('')

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

  // Modal and focus state
  get activeModal() { return activeModal() },
  setActiveModal,

  get focusArea() { return focusArea() },
  setFocusArea,

  // Selection indices
  get selectedChannelIndex() { return selectedChannelIndex() },
  setSelectedChannelIndex,

  get selectedMessageIndex() { return selectedMessageIndex() },
  setSelectedMessageIndex,

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

  // DM state
  get dmChannels() { return dmChannels() },
  setDmChannels,

  get pendingDMInvites() { return pendingDMInvites() },
  setPendingDMInvites,

  get outgoingDMInvites() { return outgoingDMInvites() },
  setOutgoingDMInvites,

  get dmChannelKeys() { return dmChannelKeys() },
  setDmChannelKeys,

  get encryptionKeyPub() { return encryptionKeyPub() },
  setEncryptionKeyPub,

  get encryptionKeyPriv() { return encryptionKeyPriv() },
  setEncryptionKeyPriv,

  get serverRoster() { return serverRoster() },
  setServerRoster,

  get selfSessionId() { return selfSessionId() },
  setSelfSessionId,

  get activeDMInvite() { return activeDMInvite() },
  setActiveDMInvite,

  get pendingEncryptionChannelId() { return pendingEncryptionChannelId() },
  setPendingEncryptionChannelId,

  get encryptionSetupReason() { return encryptionSetupReason() },
  setEncryptionSetupReason,
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
    setActiveModal(ModalState.None)
    setFocusArea(FocusArea.Sidebar)
    setSelectedChannelIndex(0)
    setSelectedMessageIndex(0)
    this.clearMessages()
    this.clearCompose()
    this.clearDMState()
  },

  // Open/close modals
  openModal(modal: ModalState) {
    setActiveModal(modal)
  },

  closeModal() {
    setActiveModal(ModalState.None)
  },

  // Update a message's content and edited_at timestamp (for MESSAGE_EDITED broadcast)
  updateMessageContent(messageId: bigint, content: string, editedAt: bigint) {
    setMessages(prev => {
      const existing = prev.get(messageId)
      if (!existing) return prev
      const newMap = new Map(prev)
      newMap.set(messageId, {
        ...existing,
        content,
        edited_at: { present: 1, value: editedAt }
      })
      return newMap
    })
  },

  // Remove a message from the store (for MESSAGE_DELETED broadcast)
  removeMessage(messageId: bigint) {
    setMessages(prev => {
      if (!prev.has(messageId)) return prev
      const newMap = new Map(prev)
      newMap.delete(messageId)
      return newMap
    })
  },

  // Remove a channel from the store (for CHANNEL_DELETED broadcast)
  removeChannel(channelId: bigint) {
    setChannels(prev => {
      if (!prev.has(channelId)) return prev
      const newMap = new Map(prev)
      newMap.delete(channelId)
      return newMap
    })

    // If the deleted channel is the active one, reset to channel list
    if (activeChannelId() === channelId) {
      setActiveChannelId(null)
      setCurrentView(ViewState.ThreadList)
      setActiveThreadId(null)
    }
  },

  // Toggle focus between sidebar and content
  toggleFocus() {
    setFocusArea(prev => prev === FocusArea.Sidebar ? FocusArea.Content : FocusArea.Sidebar)
  },

  // DM actions
  addDMChannel(dm: DMChannel) {
    setDmChannels(prev => new Map(prev).set(dm.channelId, dm))
  },

  removeDMChannel(channelId: bigint) {
    setDmChannels(prev => {
      const next = new Map(prev)
      next.delete(channelId)
      return next
    })
  },

  markDMParticipantLeft(channelId: bigint) {
    setDmChannels(prev => {
      const existing = prev.get(channelId)
      if (!existing) return prev
      const next = new Map(prev)
      next.set(channelId, { ...existing, participantLeft: true })
      return next
    })
  },

  addPendingDMInvite(invite: DMInvite) {
    setPendingDMInvites(prev => new Map(prev).set(invite.channelId, invite))
  },

  removePendingDMInvite(channelId: bigint) {
    setPendingDMInvites(prev => {
      const next = new Map(prev)
      next.delete(channelId)
      return next
    })
  },

  removePendingDMInviteByNickname(nickname: string) {
    setPendingDMInvites(prev => {
      const next = new Map(prev)
      for (const [id, invite] of next) {
        if (invite.fromNickname === nickname) next.delete(id)
      }
      return next
    })
  },

  addOutgoingDMInvite(invite: OutgoingDMInvite) {
    setOutgoingDMInvites(prev => new Map(prev).set(invite.channelId, invite))
  },

  removeOutgoingDMInvite(channelId: bigint) {
    setOutgoingDMInvites(prev => {
      const next = new Map(prev)
      next.delete(channelId)
      return next
    })
  },

  removeOutgoingDMInviteByNickname(nickname: string) {
    setOutgoingDMInvites(prev => {
      const next = new Map(prev)
      for (const [id, invite] of next) {
        if (invite.toNickname === nickname) next.delete(id)
      }
      return next
    })
  },

  setDMChannelKey(channelId: bigint, key: Uint8Array) {
    setDmChannelKeys(prev => new Map(prev).set(channelId, key))
  },

  removeDMChannelKey(channelId: bigint) {
    setDmChannelKeys(prev => {
      const next = new Map(prev)
      next.delete(channelId)
      return next
    })
  },

  updateServerRoster(entry: PresenceEntry, online: boolean) {
    if (online) {
      setServerRoster(prev => new Map(prev).set(entry.sessionId, entry))
    } else {
      setServerRoster(prev => {
        const next = new Map(prev)
        next.delete(entry.sessionId)
        return next
      })
    }
  },

  clearDMState() {
    setDmChannels(new Map())
    setPendingDMInvites(new Map())
    setOutgoingDMInvites(new Map())
    setDmChannelKeys(new Map())
    setServerRoster(new Map())
    setSelfSessionId(null)
    setActiveDMInvite(null)
    setPendingEncryptionChannelId(null)
    setEncryptionSetupReason('')
  }
}
