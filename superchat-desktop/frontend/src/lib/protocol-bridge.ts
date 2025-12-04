// Protocol Bridge
// Connects the event-based SuperChat client to the reactive store
// Listens to all protocol events and updates store accordingly

import { SuperChatEventClient, type SuperChatEvent } from './superchat-events'
import { store, storeActions } from '../store/app-store'
import { rebuildIndexes, addMessageToIndexes } from './message-indexer'

/**
 * ProtocolBridge class
 * Manages the connection between the WebSocket client and the application store
 */
export class ProtocolBridge {
  private client: SuperChatEventClient
  private unsubscribe: (() => void) | null = null

  constructor() {
    this.client = new SuperChatEventClient()
  }

  /**
   * Start listening to client events and updating the store
   */
  start(): void {
    if (this.unsubscribe) {
      console.warn('ProtocolBridge already started')
      return
    }

    this.unsubscribe = this.client.on((event) => this.handleEvent(event))
  }

  /**
   * Stop listening to client events
   */
  stop(): void {
    if (this.unsubscribe) {
      this.unsubscribe()
      this.unsubscribe = null
    }
  }

  /**
   * Get the underlying client for direct method calls
   */
  getClient(): SuperChatEventClient {
    return this.client
  }

  /**
   * Handle a protocol event and update store
   */
  private handleEvent(event: SuperChatEvent): void {
    switch (event.type) {
      case 'connection-state':
        this.handleConnectionState(event.state)
        break

      case 'error':
        this.handleError(event.message)
        break

      case 'channel-list':
        this.handleChannelList(event.channels)
        break

      case 'join-response':
        this.handleJoinResponse(event.response)
        break

      case 'message-list':
        this.handleMessageList(event.messages)
        break

      case 'message-posted':
        this.handleMessagePosted(event.response)
        break

      case 'new-message':
        this.handleNewMessage(event.message)
        break

      case 'subscribe-ok':
        this.handleSubscribeOk(event.response)
        break

      case 'protocol-error':
        this.handleProtocolError(event.error)
        break

      case 'server-config':
        console.log('Received server config:', event.config)
        break

      case 'pong':
        console.log('Received pong:', event.timestamp)
        break

      case 'traffic-update':
        storeActions.updateTraffic({
          bytesSent: event.sent,
          bytesReceived: event.received
        })
        break

      default:
        console.warn('Unhandled event type:', (event as any).type)
    }
  }

  /**
   * Handle connection state changes
   */
  private handleConnectionState(state: 'disconnected' | 'connecting' | 'connected' | 'error'): void {
    store.setConnectionState(state)

    // Clear error message on successful connection
    if (state === 'connected') {
      store.setErrorMessage('')
    }

    // Reset state on disconnect
    if (state === 'disconnected') {
      storeActions.resetConnection()
    }
  }

  /**
   * Handle general errors
   */
  private handleError(message: string): void {
    store.setErrorMessage(message)
    console.error('Client error:', message)
  }

  /**
   * Handle CHANNEL_LIST message
   */
  private handleChannelList(channels: any[]): void {
    console.log(`Received ${channels.length} channels`)
    storeActions.addChannels(channels)
  }

  /**
   * Handle JOIN_RESPONSE message
   */
  private handleJoinResponse(response: any): void {
    if (response.success === 1) {
      console.log('Successfully joined channel:', response.channel_id)
      store.setActiveChannelId(response.channel_id)

      // Request message list for this channel
      // TODO: Call client.listMessages(response.channel_id, 0n, 100)
      // For now, just log
      console.log('TODO: Request message list for channel:', response.channel_id)
    } else {
      this.handleError(response.message)
    }
  }

  /**
   * Handle MESSAGE_LIST message
   */
  private handleMessageList(messages: any[]): void {
    console.log(`Received ${messages.length} messages`)

    // Add all messages to store
    storeActions.addMessages(messages)

    // Rebuild indexes from scratch
    const { threadIndex, replyIndex } = rebuildIndexes(store.messages)
    store.setThreadIndex(threadIndex)
    store.setReplyIndex(replyIndex)
  }

  /**
   * Handle MESSAGE_POSTED response (after posting a message)
   */
  private handleMessagePosted(response: any): void {
    if (response.success === 1) {
      console.log('Message posted successfully:', response.message_id)

      // Clear compose state
      storeActions.clearCompose()

      // Message will arrive via NEW_MESSAGE broadcast
    } else {
      this.handleError(response.message)
    }
  }

  /**
   * Handle NEW_MESSAGE broadcast (real-time message)
   */
  private handleNewMessage(message: any): void {
    console.log('Received new message:', message.message_id)

    // Add message to store
    storeActions.addMessage(message)

    // Update indexes incrementally
    const { threadIndex, replyIndex } = addMessageToIndexes(
      store.threadIndex,
      store.replyIndex,
      message
    )
    store.setThreadIndex(threadIndex)
    store.setReplyIndex(replyIndex)
  }

  /**
   * Handle SUBSCRIBE_OK response
   */
  private handleSubscribeOk(response: any): void {
    console.log('Subscription confirmed:', response)

    // Update subscription tracking based on type
    if (response.subscription_type === 2) {
      // Channel subscription
      store.setSubscribedChannelId(response.target_id)
    } else if (response.subscription_type === 1) {
      // Thread subscription
      store.setSubscribedThreadId(response.target_id)
    }
  }

  /**
   * Handle ERROR protocol message
   */
  private handleProtocolError(error: any): void {
    const errorMsg = `Error ${error.error_code}: ${error.message}`
    this.handleError(errorMsg)
  }
}

/**
 * Create and start a protocol bridge
 * Returns the bridge instance for cleanup
 */
export function createProtocolBridge(): ProtocolBridge {
  const bridge = new ProtocolBridge()
  bridge.start()
  return bridge
}

/**
 * Singleton instance for convenience
 */
let globalBridge: ProtocolBridge | null = null

/**
 * Get or create the global protocol bridge
 */
export function getProtocolBridge(): ProtocolBridge {
  if (!globalBridge) {
    globalBridge = createProtocolBridge()
  }
  return globalBridge
}

/**
 * Clean up the global protocol bridge
 */
export function destroyProtocolBridge(): void {
  if (globalBridge) {
    globalBridge.stop()
    globalBridge = null
  }
}
