// Selectors for deriving state from the store
// These compute derived data reactively using SolidJS memos

import { createMemo } from 'solid-js'
import { store } from './app-store'
import type { Channel, Message } from '../SuperChatCodec'
import type { DMChannel, PresenceEntry } from './app-store'

/**
 * Get the currently active channel (or null if none selected)
 * Also covers DM channels by synthesizing a Channel object from DMChannel data
 */
export const currentChannel = createMemo<Channel | null>(() => {
  const channelId = store.activeChannelId
  if (channelId === null) return null

  // Check regular channels first
  const channel = store.channels.get(channelId)
  if (channel) return channel

  // Fall back to DM channels (synthesize a Channel-shaped object)
  const dm = store.dmChannels.get(channelId)
  if (dm) {
    return {
      channel_id: dm.channelId,
      name: `DM: ${dm.otherNickname}`,
      description: '',
      type: 0, // DM channels behave like chat channels
      retention_hours: 0,
      user_count: 2,
      is_operator: 0,
      has_subchannels: 0,
      subchannel_count: 0,
    }
  }

  return null
})

/**
 * Get all messages for the current channel (flat list, sorted by timestamp)
 */
export const currentChannelMessages = createMemo<Message[]>(() => {
  const channelId = store.activeChannelId
  if (channelId === null) return []

  const allMessages = Array.from(store.messages.values())
  return allMessages
    .filter(msg => msg.channel_id === channelId)
    .sort((a, b) => Number(a.created_at - b.created_at))
})

/**
 * Get thread list for the current channel (root messages only)
 * Only for forum channels (type=1)
 */
export const currentThreadList = createMemo<Message[]>(() => {
  const channelId = store.activeChannelId
  if (channelId === null) return []

  const threadIds = store.threadIndex.get(channelId) || []
  const threads = threadIds
    .map(id => store.messages.get(id))
    .filter((msg): msg is Message => msg !== undefined)
    .sort((a, b) => Number(b.created_at - a.created_at)) // Newest first

  return threads
})

/**
 * Build a reply tree for a given message
 * Returns the message with its children recursively nested
 */
interface MessageWithReplies extends Message {
  replies: MessageWithReplies[]
}

function buildReplyTree(messageId: bigint, messages: Map<bigint, Message>, replyIndex: Map<bigint, bigint[]>): MessageWithReplies | null {
  const message = messages.get(messageId)
  if (!message) return null

  const childIds = replyIndex.get(messageId) || []
  const replies = childIds
    .map(id => buildReplyTree(id, messages, replyIndex))
    .filter((reply): reply is MessageWithReplies => reply !== null)
    .sort((a, b) => Number(a.created_at - b.created_at)) // Oldest first for replies

  return {
    ...message,
    replies
  }
}

/**
 * Get the current thread with nested replies
 * Returns null if no thread is active or thread not found
 */
export const currentThread = createMemo<MessageWithReplies | null>(() => {
  const threadId = store.activeThreadId
  if (threadId === null) return null

  return buildReplyTree(threadId, store.messages, store.replyIndex)
})

/**
 * Get reply count for a message (direct + nested)
 */
export function getReplyCount(messageId: bigint): number {
  const replyIds = store.replyIndex.get(messageId) || []
  let count = replyIds.length

  // Add nested reply counts recursively
  for (const replyId of replyIds) {
    count += getReplyCount(replyId)
  }

  return count
}

/**
 * Get the channel type for the current channel
 * 0 = chat, 1 = forum
 */
export const currentChannelType = createMemo<number>(() => {
  const channel = currentChannel()
  return channel?.type ?? 1 // Default to forum if no channel
})

/**
 * Check if current channel is a chat channel (type=0)
 */
export const isCurrentChannelChat = createMemo<boolean>(() => {
  return currentChannelType() === 0
})

/**
 * Check if current channel is a forum channel (type=1)
 */
export const isCurrentChannelForum = createMemo<boolean>(() => {
  return currentChannelType() === 1
})

/**
 * Get channels as an array (sorted by name)
 */
export const channelsArray = createMemo<Channel[]>(() => {
  return Array.from(store.channels.values())
    .sort((a, b) => a.name.localeCompare(b.name))
})

/**
 * Check if we're connected to the server
 */
export const isConnected = createMemo<boolean>(() => {
  return store.connectionState === 'connected'
})

/**
 * Check if we're currently connecting
 */
export const isConnecting = createMemo<boolean>(() => {
  return store.connectionState === 'connecting'
})

/**
 * Check if there's a connection error
 */
export const hasConnectionError = createMemo<boolean>(() => {
  return store.connectionState === 'error'
})

/**
 * Get formatted traffic stats string
 */
export const formattedTrafficStats = createMemo<string>(() => {
  const { bytesSent, bytesReceived } = store.traffic

  function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes}B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)}MB`
  }

  return `↑${formatBytes(bytesSent)} ↓${formatBytes(bytesReceived)}`
})

/**
 * Check if compose area should be visible
 * - Chat channels: always visible
 * - Forum thread list: visible (for new threads)
 * - Forum thread detail: only visible when replyToId is set
 */
export const isComposeVisible = createMemo<boolean>(() => {
  const channelType = currentChannelType()
  const view = store.currentView
  const { replyToId } = store.compose

  // Chat channels: always visible
  if (channelType === 0) return true

  // Forum thread list: visible for new threads
  if (view === 'thread-list') return true

  // Forum thread detail: only when replying
  return replyToId !== null
})

/**
 * Get compose placeholder text based on context
 */
export const composePlaceholder = createMemo<string>(() => {
  const channelType = currentChannelType()
  const view = store.currentView
  const { replyToId } = store.compose

  // Chat channels
  if (channelType === 0) return 'Type a message...'

  // Forum thread list (new thread)
  if (view === 'thread-list') return 'Start a new conversation...'

  // Forum thread detail (replying)
  if (replyToId !== null) return 'Type your reply...'

  return 'Type a message...'
})

/**
 * Get the message being replied to (if any)
 */
export const replyTargetMessage = createMemo<Message | null>(() => {
  const { replyToId } = store.compose
  if (replyToId === null) return null
  return store.messages.get(replyToId) || null
})

/**
 * Get DM channels as a sorted array
 */
export const dmChannelsArray = createMemo<DMChannel[]>(() => {
  return Array.from(store.dmChannels.values())
    .sort((a, b) => a.otherNickname.localeCompare(b.otherNickname))
})

/**
 * Check if the current channel is a DM channel
 */
export const isCurrentChannelDM = createMemo<boolean>(() => {
  const channelId = store.activeChannelId
  if (channelId === null) return false
  return store.dmChannels.has(channelId)
})

/**
 * Get the current DM channel info (or null)
 */
export const currentDMChannel = createMemo<DMChannel | null>(() => {
  const channelId = store.activeChannelId
  if (channelId === null) return null
  return store.dmChannels.get(channelId) ?? null
})

/**
 * Check if the current channel is encrypted
 */
export const isCurrentChannelEncrypted = createMemo<boolean>(() => {
  const dm = currentDMChannel()
  if (!dm) return false
  return dm.isEncrypted
})

/**
 * Get online users available for DM (excludes self, sorted by nickname)
 */
export const onlineUsersForDM = createMemo<PresenceEntry[]>(() => {
  const selfId = store.selfSessionId
  return Array.from(store.serverRoster.values())
    .filter(entry => entry.sessionId !== selfId)
    .sort((a, b) => a.nickname.localeCompare(b.nickname))
})

/**
 * Export a convenience object with all selectors
 */
export const selectors = {
  currentChannel,
  currentChannelMessages,
  currentThreadList,
  currentThread,
  currentChannelType,
  isCurrentChannelChat,
  isCurrentChannelForum,
  channelsArray,
  isConnected,
  isConnecting,
  hasConnectionError,
  formattedTrafficStats,
  isComposeVisible,
  composePlaceholder,
  replyTargetMessage,
  getReplyCount,
  dmChannelsArray,
  isCurrentChannelDM,
  currentDMChannel,
  isCurrentChannelEncrypted,
  onlineUsersForDM,
}

export default selectors
