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
  ServerConfig
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
export const onPong = createEventFilter('pong')
export const onTrafficUpdate = createEventFilter('traffic-update')
