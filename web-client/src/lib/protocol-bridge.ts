// Protocol Bridge
// Connects the event-based SuperChat client to the reactive store
// Listens to all protocol events and updates store accordingly

import { SuperChatEventClient, type SuperChatEvent } from './superchat-events'
import { store, storeActions, ModalState } from '../store/app-store'
import { rebuildIndexes, addMessageToIndexes } from './message-indexer'
import { computeSharedSecret, deriveChannelKey, decryptMessage, KEY_TYPE_GENERATED } from './crypto'
import type { ChannelCreated, MessageEdited, MessageDeleted, ChannelDeleted, ServerPresence, ChannelPresence, AuthResponse, UserInfo, KeyRequired, DMReady, DMPending, DMRequest, DMParticipantLeft, DMDeclined, NewMessage, Message } from '../SuperChatCodec'

/**
 * Try to decrypt a message's content_raw using the channel's encryption key.
 * Returns the decrypted string, or the original content if decryption fails/not applicable.
 */
async function tryDecryptMessage(message: { content: string; content_raw?: Uint8Array; channel_id: bigint }): Promise<string> {
  const key = store.dmChannelKeys.get(message.channel_id)
  if (!key || !message.content_raw || message.content_raw.length === 0) {
    return message.content
  }

  try {
    const plaintext = await decryptMessage(key, message.content_raw)
    return new TextDecoder().decode(plaintext)
  } catch {
    // Not encrypted or decryption failed - return original
    return message.content
  }
}

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

      case 'channel-created':
        this.handleChannelCreated(event.data)
        break

      case 'message-edited':
        this.handleMessageEdited(event.data)
        break

      case 'message-deleted':
        this.handleMessageDeleted(event.data)
        break

      case 'channel-deleted':
        this.handleChannelDeleted(event.data)
        break

      case 'server-presence':
        this.handleServerPresence(event.data)
        break

      case 'channel-presence':
        this.handleChannelPresence(event.data)
        break

      case 'user-info':
        this.handleUserInfo(event.info)
        break

      case 'auth-response':
        this.handleAuthResponse(event.response)
        break

      case 'key-required':
        this.handleKeyRequired(event.data)
        break

      case 'dm-ready':
        this.handleDMReady(event.data)
        break

      case 'dm-pending':
        this.handleDMPending(event.data)
        break

      case 'dm-request':
        this.handleDMRequest(event.data)
        break

      case 'dm-participant-left':
        this.handleDMParticipantLeft(event.data)
        break

      case 'dm-declined':
        this.handleDMDeclined(event.data)
        break

      case 'server-config':
        console.log('Received server config:', event.config)
        break

      case 'pong':
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
    } else {
      this.handleError(response.message)
    }
  }

  /**
   * Handle MESSAGE_LIST message - with decryption for encrypted DM channels
   */
  private async handleMessageList(messages: Message[]): Promise<void> {
    console.log(`Received ${messages.length} messages`)

    // Decrypt messages from encrypted DM channels
    for (const msg of messages) {
      const decrypted = await tryDecryptMessage(msg)
      if (decrypted !== msg.content) {
        ;(msg as any).content = decrypted
      }
    }

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
   * Handle NEW_MESSAGE broadcast - with decryption for encrypted DM channels
   */
  private async handleNewMessage(message: NewMessage): Promise<void> {
    console.log('Received new message:', message.message_id)

    // Decrypt if this is from an encrypted DM channel
    const decrypted = await tryDecryptMessage(message)
    if (decrypted !== message.content) {
      ;(message as any).content = decrypted
    }

    // Increment unread count for DM channels not currently active
    const dm = store.dmChannels.get(message.channel_id)
    if (dm && store.activeChannelId !== message.channel_id) {
      storeActions.addDMChannel({ ...dm, unreadCount: dm.unreadCount + 1 })
    }

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
    if (response.type === 2) {
      // Channel subscription
      store.setSubscribedChannelId(response.id)
    } else if (response.type === 1) {
      // Thread subscription
      store.setSubscribedThreadId(response.id)
    }
  }

  /**
   * Handle ERROR protocol message
   */
  private handleProtocolError(error: any): void {
    const errorMsg = `Error ${error.error_code}: ${error.message}`
    this.handleError(errorMsg)
  }

  /**
   * Handle CHANNEL_CREATED broadcast
   */
  private handleChannelCreated(data: ChannelCreated): void {
    if (data.success !== 1 || !data.channel_id || !data.name) return

    storeActions.addChannel({
      channel_id: data.channel_id,
      name: data.name,
      description: data.description ?? '',
      type: data.type ?? 0,
      retention_hours: data.retention_hours ?? 168,
      user_count: 0,
      is_operator: 0,
      has_subchannels: 0,
      subchannel_count: 0,
    })
  }

  /**
   * Handle MESSAGE_EDITED broadcast
   */
  private handleMessageEdited(data: MessageEdited): void {
    if (data.success !== 1 || !data.new_content || !data.edited_at) return

    storeActions.updateMessageContent(data.message_id, data.new_content, data.edited_at)
  }

  /**
   * Handle MESSAGE_DELETED broadcast
   */
  private handleMessageDeleted(data: MessageDeleted): void {
    if (data.success !== 1) return

    storeActions.removeMessage(data.message_id)

    // Rebuild indexes after removal
    const { threadIndex, replyIndex } = rebuildIndexes(store.messages)
    store.setThreadIndex(threadIndex)
    store.setReplyIndex(replyIndex)
  }

  /**
   * Handle CHANNEL_DELETED broadcast
   */
  private handleChannelDeleted(data: ChannelDeleted): void {
    if (data.success !== 1) return

    storeActions.removeChannel(data.channel_id)
  }

  /**
   * Handle SERVER_PRESENCE broadcast - update server roster
   */
  private handleServerPresence(data: ServerPresence): void {
    const online = data.online === 1

    storeActions.updateServerRoster({
      sessionId: data.session_id,
      nickname: data.nickname,
      isRegistered: data.is_registered === 1,
      userId: data.user_id.present === 1 ? data.user_id.value! : null,
      userFlags: data.user_flags,
    }, online)

    // Track our own session ID: if the nickname matches ours and it's an online event
    if (online && data.nickname === store.nickname) {
      store.setSelfSessionId(data.session_id)
    }
  }

  /**
   * Handle CHANNEL_PRESENCE broadcast (user joined/left channel)
   */
  private handleChannelPresence(data: ChannelPresence): void {
    console.log(`Channel presence: ${data.nickname} ${data.joined ? 'joined' : 'left'} channel ${data.channel_id}`)
  }

  /**
   * Handle USER_INFO - server informs us about a nickname's registration status
   * If the nickname is registered and matches ours, offer authentication
   */
  private handleUserInfo(info: UserInfo): void {
    console.log('User info:', info.nickname, 'registered:', info.is_registered)

    if (info.is_registered && info.nickname === store.nickname) {
      store.setPendingAuthNickname(info.nickname)
      store.setAuthError('')
      storeActions.openModal(ModalState.Password)
    }
  }

  /**
   * Handle AUTH_RESPONSE - result of authentication attempt
   */
  private handleAuthResponse(response: AuthResponse): void {
    if (response.success === 1) {
      console.log('Authentication successful:', response.nickname)
      store.setIsRegistered(true)
      store.setNickname(response.nickname)
      store.setPendingAuthNickname('')
      store.setAuthError('')
      storeActions.closeModal()
    } else {
      console.log('Authentication failed:', response.message)
      store.setAuthError(response.message)
    }
  }

  /**
   * Handle KEY_REQUIRED - server needs encryption key to proceed with DM
   */
  private handleKeyRequired(data: KeyRequired): void {
    console.log(`Key required: ${data.reason}`, data.dm_channel_id)

    const channelId = data.dm_channel_id.present === 1 ? data.dm_channel_id.value! : null
    store.setPendingEncryptionChannelId(channelId)
    store.setEncryptionSetupReason(data.reason)

    // If we already have keys, provide them automatically
    if (store.encryptionKeyPub) {
      this.client.providePublicKey(KEY_TYPE_GENERATED, store.encryptionKeyPub, 'web-client')
      return
    }

    // Otherwise show the encryption setup modal
    storeActions.openModal(ModalState.EncryptionSetup)
  }

  /**
   * Handle DM_READY - DM channel is ready to use
   */
  private handleDMReady(data: DMReady): void {
    console.log(`DM ready: channel ${data.channel_id} with ${data.other_nickname} (encrypted: ${data.is_encrypted})`)

    const isEncrypted = data.is_encrypted === 1
    const otherUserId = data.other_user_id.present === 1 ? data.other_user_id.value! : null

    // Create the DM channel entry
    storeActions.addDMChannel({
      channelId: data.channel_id,
      otherUserId,
      otherNickname: data.other_nickname,
      isEncrypted,
      otherPubKey: data.other_public_key ?? null,
      unreadCount: 0,
      participantLeft: false,
    })

    // Derive encryption key if encrypted
    if (isEncrypted && data.other_public_key && store.encryptionKeyPriv) {
      try {
        const shared = computeSharedSecret(store.encryptionKeyPriv, data.other_public_key)
        const channelKey = deriveChannelKey(shared, data.channel_id)
        storeActions.setDMChannelKey(data.channel_id, channelKey)
      } catch (err) {
        console.error('Failed to derive DM encryption key:', err)
      }
    }

    // Clean up pending/outgoing invites for this nickname
    storeActions.removePendingDMInviteByNickname(data.other_nickname)
    storeActions.removeOutgoingDMInviteByNickname(data.other_nickname)

    // Close DM-related modals if open
    if (store.activeModal === ModalState.DMRequest || store.activeModal === ModalState.EncryptionSetup) {
      storeActions.closeModal()
    }
  }

  /**
   * Handle DM_PENDING - waiting for other party to accept
   */
  private handleDMPending(data: DMPending): void {
    console.log(`DM pending: waiting for ${data.waiting_for_nickname} (${data.reason})`)

    storeActions.addOutgoingDMInvite({
      channelId: data.dm_channel_id,
      toUserId: data.waiting_for_user_id.present === 1 ? data.waiting_for_user_id.value! : null,
      toNickname: data.waiting_for_nickname,
    })
  }

  /**
   * Handle DM_REQUEST - incoming DM request from another user
   */
  private handleDMRequest(data: DMRequest): void {
    console.log(`DM request from ${data.from_nickname} (encryption: ${data.encryption_status})`)

    const invite = {
      channelId: data.dm_channel_id,
      fromUserId: data.from_user_id.present === 1 ? data.from_user_id.value! : null,
      fromNickname: data.from_nickname,
      encryptionStatus: data.encryption_status,
    }

    storeActions.addPendingDMInvite(invite)
    store.setActiveDMInvite(invite)

    // Open DMRequest modal (only if no modal is already open)
    if (store.activeModal === ModalState.None) {
      storeActions.openModal(ModalState.DMRequest)
    }
  }

  /**
   * Handle DM_PARTICIPANT_LEFT - someone permanently left a DM
   */
  private handleDMParticipantLeft(data: DMParticipantLeft): void {
    console.log(`DM participant left: ${data.nickname} left DM channel ${data.dm_channel_id}`)

    storeActions.markDMParticipantLeft(data.dm_channel_id)
  }

  /**
   * Handle DM_DECLINED - DM request was declined
   */
  private handleDMDeclined(data: DMDeclined): void {
    console.log(`DM declined by ${data.nickname} for DM channel ${data.dm_channel_id}`)

    storeActions.removeOutgoingDMInvite(data.dm_channel_id)
    store.setErrorMessage(`${data.nickname} declined your DM request`)
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
