// Event-based SuperChat client wrapper
// Extends the basic SuperChatClient with event emission for all protocol messages

import { SuperChatClient } from './superchat'
import type { ConnectionState } from './superchat'
import type {
  Channel,
  Message,
  NicknameResponse,
  JoinResponse,
  MessagePosted,
  NewMessage,
  SubscribeOk,
  Error_,
  ServerConfig,
  ChannelCreated,
  MessageEdited,
  MessageDeleted,
  ChannelDeleted,
  ServerPresence,
  ChannelPresence,
  AuthResponse,
  UserInfo,
  KeyRequired,
  DMReady,
  DMPending,
  DMRequest,
  DMParticipantLeft,
  DMDeclined
} from '../SuperChatCodec'

// Event types for all protocol messages
export type SuperChatEvent =
  | { type: 'connection-state', state: ConnectionState }
  | { type: 'error', message: string }
  | { type: 'server-config', config: ServerConfig }
  | { type: 'nickname-response', response: NicknameResponse }
  | { type: 'channel-list', channels: Channel[] }
  | { type: 'join-response', response: JoinResponse }
  | { type: 'message-list', messages: Message[] }
  | { type: 'message-posted', response: MessagePosted }
  | { type: 'new-message', message: NewMessage }
  | { type: 'subscribe-ok', response: SubscribeOk }
  | { type: 'protocol-error', error: Error_ }
  | { type: 'channel-created', data: ChannelCreated }
  | { type: 'message-edited', data: MessageEdited }
  | { type: 'message-deleted', data: MessageDeleted }
  | { type: 'channel-deleted', data: ChannelDeleted }
  | { type: 'server-presence', data: ServerPresence }
  | { type: 'channel-presence', data: ChannelPresence }
  | { type: 'user-info', info: UserInfo }
  | { type: 'auth-response', response: AuthResponse }
  | { type: 'key-required', data: KeyRequired }
  | { type: 'dm-ready', data: DMReady }
  | { type: 'dm-pending', data: DMPending }
  | { type: 'dm-request', data: DMRequest }
  | { type: 'dm-participant-left', data: DMParticipantLeft }
  | { type: 'dm-declined', data: DMDeclined }
  | { type: 'pong', timestamp: bigint }
  | { type: 'traffic-update', sent: number, received: number }

export type EventListener = (event: SuperChatEvent) => void

/**
 * Event-based SuperChat client
 * Wraps the basic SuperChatClient and emits events for all protocol messages
 */
export class SuperChatEventClient {
  private client: SuperChatClient
  private listeners: EventListener[] = []

  constructor() {
    // Create underlying client with callbacks that emit events
    this.client = new SuperChatClient({
      onStateChange: (state) => {
        this.emit({ type: 'connection-state', state })
      },
      onChannelsReceived: (channels) => {
        this.emit({ type: 'channel-list', channels })
      },
      onJoinResponse: (response) => {
        this.emit({ type: 'join-response', response })
      },
      onMessagesReceived: (messages) => {
        this.emit({ type: 'message-list', messages })
      },
      onMessagePosted: (response) => {
        this.emit({ type: 'message-posted', response })
      },
      onNewMessage: (message) => {
        this.emit({ type: 'new-message', message })
      },
      onSubscribeOk: (response) => {
        this.emit({ type: 'subscribe-ok', response })
      },
      onServerConfig: (config) => {
        this.emit({ type: 'server-config', config })
      },
      onProtocolError: (error) => {
        this.emit({ type: 'protocol-error', error })
      },
      onChannelCreated: (data) => {
        this.emit({ type: 'channel-created', data })
      },
      onMessageEdited: (data) => {
        this.emit({ type: 'message-edited', data })
      },
      onMessageDeleted: (data) => {
        this.emit({ type: 'message-deleted', data })
      },
      onChannelDeleted: (data) => {
        this.emit({ type: 'channel-deleted', data })
      },
      onServerPresence: (data) => {
        this.emit({ type: 'server-presence', data })
      },
      onChannelPresence: (data) => {
        this.emit({ type: 'channel-presence', data })
      },
      onUserInfo: (info) => {
        this.emit({ type: 'user-info', info })
      },
      onAuthResponse: (response) => {
        this.emit({ type: 'auth-response', response })
      },
      onKeyRequired: (data) => {
        this.emit({ type: 'key-required', data })
      },
      onDMReady: (data) => {
        this.emit({ type: 'dm-ready', data })
      },
      onDMPending: (data) => {
        this.emit({ type: 'dm-pending', data })
      },
      onDMRequest: (data) => {
        this.emit({ type: 'dm-request', data })
      },
      onDMParticipantLeft: (data) => {
        this.emit({ type: 'dm-participant-left', data })
      },
      onDMDeclined: (data) => {
        this.emit({ type: 'dm-declined', data })
      },
      onTrafficUpdate: (bytesSent, bytesReceived) => {
        this.emit({ type: 'traffic-update', sent: bytesSent, received: bytesReceived })
      },
      onError: (message) => {
        this.emit({ type: 'error', message })
      }
    })
  }

  /**
   * Add an event listener
   * Returns unsubscribe function
   */
  on(listener: EventListener): () => void {
    this.listeners.push(listener)
    return () => {
      const index = this.listeners.indexOf(listener)
      if (index > -1) {
        this.listeners.splice(index, 1)
      }
    }
  }

  /**
   * Remove an event listener
   */
  off(listener: EventListener): void {
    const index = this.listeners.indexOf(listener)
    if (index > -1) {
      this.listeners.splice(index, 1)
    }
  }

  /**
   * Emit an event to all listeners
   */
  private emit(event: SuperChatEvent): void {
    for (const listener of this.listeners) {
      try {
        listener(event)
      } catch (error) {
        console.error('Error in event listener:', error)
      }
    }
  }

  /**
   * Connect to server
   */
  connect(url: string, nickname: string): void {
    this.client.connect(url, nickname)
  }

  /**
   * Disconnect from server
   */
  disconnect(): void {
    this.client.disconnect()
  }

  /**
   * Join a channel
   */
  joinChannel(channelId: bigint): void {
    this.client.joinChannel(channelId)
  }

  /**
   * List messages in a channel
   */
  listMessages(channelId: bigint, fromMessageId: bigint = 0n, limit: number = 100): void {
    this.client.listMessages(channelId, fromMessageId, limit)
  }

  /**
   * List messages for a specific thread (replies only)
   */
  listMessagesForThread(channelId: bigint, threadId: bigint, limit: number = 100): void {
    this.client.listMessagesForThread(channelId, threadId, limit)
  }

  /**
   * Post a message to a channel
   */
  postMessage(channelId: bigint, content: string, parentId: bigint | null = null): void {
    this.client.postMessage(channelId, content, parentId)
  }

  /**
   * Subscribe to channel broadcasts
   */
  subscribeChannel(channelId: bigint): void {
    this.client.subscribeChannel(channelId)
  }

  /**
   * Unsubscribe from channel broadcasts
   */
  unsubscribeChannel(channelId: bigint): void {
    this.client.unsubscribeChannel(channelId)
  }

  /**
   * Subscribe to thread broadcasts
   */
  subscribeThread(messageId: bigint): void {
    this.client.subscribeThread(messageId)
  }

  /**
   * Unsubscribe from thread broadcasts
   */
  unsubscribeThread(messageId: bigint): void {
    this.client.unsubscribeThread(messageId)
  }

  /**
   * Start a DM with another user
   */
  startDM(targetType: number, targetId: bigint | null, targetNickname: string | null, allowUnencrypted: boolean): void {
    this.client.startDM(targetType, targetId, targetNickname, allowUnencrypted)
  }

  /**
   * Accept an unencrypted DM
   */
  allowUnencryptedDM(dmChannelId: bigint, permanent: boolean): void {
    this.client.allowUnencryptedDM(dmChannelId, permanent)
  }

  /**
   * Decline a DM request
   */
  declineDM(dmChannelId: bigint): void {
    this.client.declineDM(dmChannelId)
  }

  /**
   * Provide X25519 public key for encryption
   */
  providePublicKey(keyType: number, publicKey: Uint8Array, label: string): void {
    this.client.providePublicKey(keyType, publicKey, label)
  }

  /**
   * Post a message with raw binary content (for encrypted messages)
   */
  postMessageRaw(channelId: bigint, contentRaw: Uint8Array, parentId: bigint | null = null): void {
    this.client.postMessageRaw(channelId, contentRaw, parentId)
  }

  /**
   * Leave a channel (with optional permanent flag for DMs)
   */
  leaveChannel(channelId: bigint, permanent: boolean = false): void {
    this.client.leaveChannel(channelId, permanent)
  }

  /**
   * Send authentication request with pre-hashed password
   */
  sendAuthRequest(nickname: string, hashedPassword: string): void {
    this.client.sendAuthRequest(nickname, hashedPassword)
  }
}

/**
 * Helper to create typed event listeners for specific event types
 */
export function createEventFilter<T extends SuperChatEvent['type']>(
  eventType: T
): (listener: (event: Extract<SuperChatEvent, { type: T }>) => void) => EventListener {
  return (listener) => {
    return (event) => {
      if (event.type === eventType) {
        listener(event as Extract<SuperChatEvent, { type: T }>)
      }
    }
  }
}

// Convenience filters for common event types
export const onConnectionState = createEventFilter('connection-state')
export const onError = createEventFilter('error')
export const onServerConfig = createEventFilter('server-config')
export const onNicknameResponse = createEventFilter('nickname-response')
export const onChannelList = createEventFilter('channel-list')
export const onJoinResponse = createEventFilter('join-response')
export const onMessageList = createEventFilter('message-list')
export const onMessagePosted = createEventFilter('message-posted')
export const onNewMessage = createEventFilter('new-message')
export const onSubscribeOk = createEventFilter('subscribe-ok')
export const onProtocolError = createEventFilter('protocol-error')
export const onChannelCreated = createEventFilter('channel-created')
export const onMessageEdited = createEventFilter('message-edited')
export const onMessageDeleted = createEventFilter('message-deleted')
export const onChannelDeleted = createEventFilter('channel-deleted')
export const onServerPresence = createEventFilter('server-presence')
export const onChannelPresence = createEventFilter('channel-presence')
export const onUserInfo = createEventFilter('user-info')
export const onAuthResponse = createEventFilter('auth-response')
export const onKeyRequired = createEventFilter('key-required')
export const onDMReady = createEventFilter('dm-ready')
export const onDMPending = createEventFilter('dm-pending')
export const onDMRequest = createEventFilter('dm-request')
export const onDMParticipantLeft = createEventFilter('dm-participant-left')
export const onDMDeclined = createEventFilter('dm-declined')
export const onPong = createEventFilter('pong')
export const onTrafficUpdate = createEventFilter('traffic-update')
