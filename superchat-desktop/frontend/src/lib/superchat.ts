// SuperChat WebSocket client
// Handles binary protocol communication with SuperChat server

import {
  FrameHeaderEncoder, FrameHeaderDecoder,
  SetNicknameEncoder, NicknameResponseDecoder,
  ListChannelsEncoder, ChannelListDecoder,
  JoinChannelEncoder, JoinResponseDecoder,
  ListMessagesEncoder, MessageListDecoder,
  PostMessageEncoder, MessagePostedDecoder,
  NewMessageDecoder,
  SubscribeChannelEncoder, UnsubscribeChannelEncoder,
  SubscribeThreadEncoder, UnsubscribeThreadEncoder,
  SubscribeOkDecoder,
  Error_Decoder,
  ServerConfigDecoder,
  PingEncoder,
  type Channel,
  type Message,
  type NewMessage,
  type MessagePosted,
  type SubscribeOk,
  type Error_,
  type ServerConfig,
} from '../SuperChatCodec'

// Protocol message type codes
const MSG_SET_NICKNAME = 0x02
const MSG_NICKNAME_RESPONSE = 0x82
const MSG_LIST_CHANNELS = 0x04
const MSG_CHANNEL_LIST = 0x84
const MSG_JOIN_CHANNEL = 0x05
const MSG_JOIN_RESPONSE = 0x85
const MSG_LIST_MESSAGES = 0x09
const MSG_MESSAGE_LIST = 0x89
const MSG_POST_MESSAGE = 0x0A
const MSG_MESSAGE_POSTED = 0x8A
const MSG_NEW_MESSAGE = 0x8D
const MSG_PING = 0x10
const MSG_PONG = 0x90
const MSG_SUBSCRIBE_THREAD = 0x51
const MSG_UNSUBSCRIBE_THREAD = 0x52
const MSG_SUBSCRIBE_CHANNEL = 0x53
const MSG_UNSUBSCRIBE_CHANNEL = 0x54
const MSG_SUBSCRIBE_OK = 0x99
const MSG_ERROR = 0x91
const MSG_SERVER_CONFIG = 0x98
const MSG_CHANNEL_PRESENCE = 0xAC

export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'error'

export interface SuperChatClientEvents {
  onStateChange: (state: ConnectionState) => void
  onChannelsReceived: (channels: Channel[]) => void
  onJoinResponse?: (response: any) => void
  onMessagesReceived?: (messages: Message[]) => void
  onMessagePosted?: (response: MessagePosted) => void
  onNewMessage?: (message: NewMessage) => void
  onSubscribeOk?: (response: SubscribeOk) => void
  onServerConfig?: (config: ServerConfig) => void
  onProtocolError?: (error: Error_) => void
  onTrafficUpdate?: (bytesSent: number, bytesReceived: number) => void
  onError: (error: string) => void
}

export class SuperChatClient {
  private ws: WebSocket | null = null
  private nickname: string = ''
  private pingInterval: number | null = null
  private frameBuffer: Uint8Array = new Uint8Array(0)
  private expectedFrameLength: number | null = null

  private state: ConnectionState = 'disconnected'
  private events: SuperChatClientEvents

  // Traffic tracking
  private bytesSent: number = 0
  private bytesReceived: number = 0
  private trafficUpdateInterval: number | null = null

  // Reconnection tracking
  private lastUrl: string = ''
  private lastNickname: string = ''
  private reconnectTimeout: number | null = null
  private shouldReconnect: boolean = true

  constructor(events: SuperChatClientEvents) {
    this.events = events
  }

  connect(url: string, nickname: string) {
    this.nickname = nickname
    this.lastUrl = url
    this.lastNickname = nickname
    this.shouldReconnect = true
    this.setState('connecting')

    try {
      this.ws = new WebSocket(url)
      this.ws.binaryType = 'arraybuffer'

      this.ws.onopen = () => {
        console.log('WebSocket connected')
        this.setState('connected')
        this.sendSetNickname(nickname)

        // Start ping interval (30 seconds)
        this.pingInterval = window.setInterval(() => this.sendPing(), 30000)

        // Start traffic update interval (1 second)
        this.trafficUpdateInterval = window.setInterval(() => {
          if (this.events.onTrafficUpdate) {
            this.events.onTrafficUpdate(this.bytesSent, this.bytesReceived)
          }
        }, 1000)
      }

      this.ws.onmessage = (event) => {
        const data = new Uint8Array(event.data)
        this.bytesReceived += data.length
        this.handleFragment(data)
      }

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error)
        this.setState('error')
        this.events.onError('Failed to connect')
      }

      this.ws.onclose = () => {
        console.log('WebSocket closed')
        this.setState('disconnected')
        if (this.pingInterval) {
          clearInterval(this.pingInterval)
          this.pingInterval = null
        }
        if (this.trafficUpdateInterval) {
          clearInterval(this.trafficUpdateInterval)
          this.trafficUpdateInterval = null
        }

        // Attempt to reconnect after 2 seconds if shouldReconnect is true
        if (this.shouldReconnect && this.lastUrl) {
          console.log('Attempting to reconnect in 2 seconds...')
          this.reconnectTimeout = window.setTimeout(() => {
            console.log('Reconnecting to', this.lastUrl)
            this.connect(this.lastUrl, this.lastNickname)
          }, 2000)
        }
      }
    } catch (error) {
      console.error('Failed to connect:', error)
      this.setState('error')
      this.events.onError('Connection failed')
    }
  }

  disconnect() {
    // Prevent automatic reconnection on manual disconnect
    this.shouldReconnect = false

    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout)
      this.reconnectTimeout = null
    }

    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
  }

  private setState(state: ConnectionState) {
    this.state = state
    this.events.onStateChange(state)
  }

  private sendFrame(messageType: number, payloadBytes: Uint8Array) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.error('WebSocket not connected')
      return
    }

    // Encode frame header
    const headerEncoder = new FrameHeaderEncoder()
    const header = headerEncoder.encode({
      length: 3 + payloadBytes.length, // version(1) + type(1) + flags(1) + payload
      version: 1,
      type: messageType,
      flags: 0
    })

    // Combine header + payload
    const frame = new Uint8Array(header.length + payloadBytes.length)
    frame.set(header, 0)
    frame.set(payloadBytes, header.length)

    this.bytesSent += frame.length
    this.ws.send(frame)
  }

  private handleFragment(fragment: Uint8Array) {
    // Append fragment to buffer
    const newBuffer = new Uint8Array(this.frameBuffer.length + fragment.length)
    newBuffer.set(this.frameBuffer, 0)
    newBuffer.set(fragment, this.frameBuffer.length)
    this.frameBuffer = newBuffer

    // Try to extract complete frames from buffer
    while (true) {
      // Need at least 4 bytes to read frame length
      if (this.expectedFrameLength === null && this.frameBuffer.length >= 4) {
        // Read frame length (big-endian uint32)
        const view = new DataView(this.frameBuffer.buffer, this.frameBuffer.byteOffset, 4)
        this.expectedFrameLength = view.getUint32(0, false) // false = big-endian
      }

      // Check if we have a complete frame (length prefix + frame data)
      if (this.expectedFrameLength !== null) {
        const totalFrameSize = 4 + this.expectedFrameLength
        if (this.frameBuffer.length >= totalFrameSize) {
          // Extract complete frame
          const completeFrame = this.frameBuffer.slice(0, totalFrameSize)

          // Remove frame from buffer
          this.frameBuffer = this.frameBuffer.slice(totalFrameSize)
          this.expectedFrameLength = null

          // Process frame
          this.handleMessage(completeFrame)

          // Continue to check for more frames in buffer
          continue
        }
      }

      // Not enough data yet, wait for more fragments
      break
    }
  }

  private handleMessage(data: Uint8Array) {
    try {
      // Check minimum frame size
      if (data.length < 7) {
        console.error(`Frame too short: ${data.length} bytes`)
        return
      }

      // Decode frame header
      const headerDecoder = new FrameHeaderDecoder(data)
      const header = headerDecoder.decode()

      // Extract payload (skip length(4) + version(1) + type(1) + flags(1) = 7 bytes)
      const payloadSlice = data.slice(7)
      const payload = new Uint8Array(new ArrayBuffer(payloadSlice.length))
      payload.set(payloadSlice)

      switch (header.type) {
        case MSG_SERVER_CONFIG:
          this.handleServerConfig(payload)
          break
        case MSG_NICKNAME_RESPONSE:
          this.handleNicknameResponse(payload)
          break
        case MSG_CHANNEL_LIST:
          this.handleChannelList(payload)
          break
        case MSG_JOIN_RESPONSE:
          this.handleJoinResponse(payload)
          break
        case MSG_MESSAGE_LIST:
          this.handleMessageList(payload)
          break
        case MSG_MESSAGE_POSTED:
          this.handleMessagePosted(payload)
          break
        case MSG_NEW_MESSAGE:
          this.handleNewMessage(payload)
          break
        case MSG_SUBSCRIBE_OK:
          this.handleSubscribeOk(payload)
          break
        case MSG_ERROR:
          this.handleProtocolError(payload)
          break
        case MSG_PONG:
          console.log('Received PONG')
          break
        case MSG_CHANNEL_PRESENCE:
          // Channel presence notifications (users joining/leaving) - ignore for now
          console.log('Received CHANNEL_PRESENCE notification')
          break
        default:
          console.warn(`Unhandled message type: 0x${header.type.toString(16)}`)
      }
    } catch (error) {
      console.error('Error handling message:', error)
    }
  }

  private sendSetNickname(nickname: string) {
    const encoder = new SetNicknameEncoder()
    const payload = encoder.encode({ nickname })
    this.sendFrame(MSG_SET_NICKNAME, payload)
  }

  private handleNicknameResponse(payload: Uint8Array) {
    const decoder = new NicknameResponseDecoder(payload)
    const response = decoder.decode()

    if (response.success === 1) {
      console.log('Nickname set successfully:', response.message)
      // Request channel list after successful nickname setup
      this.sendListChannels()
    } else {
      this.events.onError(response.message)
    }
  }

  private sendListChannels() {
    const encoder = new ListChannelsEncoder()
    const payload = encoder.encode({ from_channel_id: 0n, limit: 100 })
    this.sendFrame(MSG_LIST_CHANNELS, payload)
  }

  private handleChannelList(payload: Uint8Array) {
    const decoder = new ChannelListDecoder(payload)
    const channelList = decoder.decode()

    console.log(`Received ${channelList.channel_count} channels`)
    this.events.onChannelsReceived(channelList.channels)
  }

  joinChannel(channelId: bigint) {
    const encoder = new JoinChannelEncoder()
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 }
    })
    this.sendFrame(MSG_JOIN_CHANNEL, payload)
  }

  private handleJoinResponse(payload: Uint8Array) {
    const decoder = new JoinResponseDecoder(payload)
    const response = decoder.decode()

    if (response.success === 1) {
      console.log('Joined channel:', response.channel_id)
    } else {
      this.events.onError(response.message)
    }

    // Emit join response event
    if (this.events.onJoinResponse) {
      this.events.onJoinResponse(response)
    }
  }

  private sendPing() {
    const encoder = new PingEncoder()
    const payload = encoder.encode({ timestamp: BigInt(Date.now()) })
    this.sendFrame(MSG_PING, payload)
  }

  private handleServerConfig(payload: Uint8Array) {
    const decoder = new ServerConfigDecoder(payload)
    const config = decoder.decode()
    console.log('Server config:', config)
    if (this.events.onServerConfig) {
      this.events.onServerConfig(config)
    }
  }

  private handleMessageList(payload: Uint8Array) {
    const decoder = new MessageListDecoder(payload)
    const messageList = decoder.decode()
    console.log(`Received ${messageList.message_count} messages`)
    if (this.events.onMessagesReceived) {
      this.events.onMessagesReceived(messageList.messages)
    }
  }

  private handleMessagePosted(payload: Uint8Array) {
    const decoder = new MessagePostedDecoder(payload)
    const response = decoder.decode()
    console.log('Message posted:', response)
    if (this.events.onMessagePosted) {
      this.events.onMessagePosted(response)
    }
  }

  private handleNewMessage(payload: Uint8Array) {
    const decoder = new NewMessageDecoder(payload)
    const newMessage = decoder.decode()
    console.log('New message received:', newMessage)
    if (this.events.onNewMessage) {
      this.events.onNewMessage(newMessage)
    }
  }

  private handleSubscribeOk(payload: Uint8Array) {
    const decoder = new SubscribeOkDecoder(payload)
    const response = decoder.decode()
    console.log('Subscribe OK:', response)
    if (this.events.onSubscribeOk) {
      this.events.onSubscribeOk(response)
    }
  }

  private handleProtocolError(payload: Uint8Array) {
    const decoder = new Error_Decoder(payload)
    const error = decoder.decode()
    console.error('Protocol error:', error)
    if (this.events.onProtocolError) {
      this.events.onProtocolError(error)
    }
    this.events.onError(`Error ${error.error_code}: ${error.message}`)
  }

  // Public API methods

  listMessages(channelId: bigint, fromMessageId: bigint, limit: number) {
    const encoder = new ListMessagesEncoder()
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 },
      limit,
      before_id: fromMessageId !== 0n ? { present: 1, value: fromMessageId } : { present: 0 },
      parent_id: { present: 0 },
      after_id: { present: 0 }
    })
    this.sendFrame(MSG_LIST_MESSAGES, payload)
  }

  listMessagesForThread(channelId: bigint, threadId: bigint, limit: number) {
    const encoder = new ListMessagesEncoder()
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 },
      limit,
      before_id: { present: 0 },
      parent_id: { present: 1, value: threadId },
      after_id: { present: 0 }
    })
    this.sendFrame(MSG_LIST_MESSAGES, payload)
  }

  postMessage(channelId: bigint, content: string, parentId: bigint | null = null) {
    const encoder = new PostMessageEncoder()
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 },
      parent_id: parentId !== null ? { present: 1, value: parentId } : { present: 0 },
      content
    })
    this.sendFrame(MSG_POST_MESSAGE, payload)
  }

  subscribeChannel(channelId: bigint) {
    const encoder = new SubscribeChannelEncoder()
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 }
    })
    this.sendFrame(MSG_SUBSCRIBE_CHANNEL, payload)
  }

  unsubscribeChannel(channelId: bigint) {
    const encoder = new UnsubscribeChannelEncoder()
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 }
    })
    this.sendFrame(MSG_UNSUBSCRIBE_CHANNEL, payload)
  }

  subscribeThread(messageId: bigint) {
    const encoder = new SubscribeThreadEncoder()
    const payload = encoder.encode({ thread_id: messageId })
    this.sendFrame(MSG_SUBSCRIBE_THREAD, payload)
  }

  unsubscribeThread(messageId: bigint) {
    const encoder = new UnsubscribeThreadEncoder()
    const payload = encoder.encode({ thread_id: messageId })
    this.sendFrame(MSG_UNSUBSCRIBE_THREAD, payload)
  }
}
