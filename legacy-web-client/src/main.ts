// ABOUTME: SuperChat web client main entry point
// ABOUTME: Handles WebSocket connection, protocol encoding/decoding, and UI updates

import {
  FrameHeaderEncoder, FrameHeaderDecoder,
  SetNicknameEncoder, NicknameResponseDecoder,
  ListChannelsEncoder, ChannelListDecoder,
  JoinChannelEncoder, JoinResponseDecoder,
  ListMessagesEncoder, MessageListDecoder,
  PostMessageEncoder, MessagePostedDecoder,
  NewMessageDecoder,
  SubscribeThreadEncoder, UnsubscribeThreadEncoder,
  SubscribeChannelEncoder, UnsubscribeChannelEncoder,
  SubscribeOkDecoder,
  ServerConfigDecoder,
  Error_Decoder,
  PingEncoder,
  type SetNickname, type ListChannels, type JoinChannel,
  type ListMessages, type PostMessage, type Ping,
  type Channel, type Message, type NewMessage, type SubscribeOk
} from './SuperChatCodec.js';

import { BitStreamDecoder } from './BitStream.js';

// View states
enum ViewState {
  ThreadList,
  ThreadDetail
}

// Protocol message type codes
const MSG_SET_NICKNAME = 0x02;
const MSG_NICKNAME_RESPONSE = 0x82;
const MSG_LIST_CHANNELS = 0x04;
const MSG_CHANNEL_LIST = 0x84;
const MSG_JOIN_CHANNEL = 0x05;
const MSG_JOIN_RESPONSE = 0x85;
const MSG_LIST_MESSAGES = 0x09;
const MSG_MESSAGE_LIST = 0x89;
const MSG_POST_MESSAGE = 0x0A;
const MSG_MESSAGE_POSTED = 0x8A;
const MSG_NEW_MESSAGE = 0x8D;
const MSG_PING = 0x10;
const MSG_PONG = 0x90;
const MSG_SUBSCRIBE_THREAD = 0x51;
const MSG_UNSUBSCRIBE_THREAD = 0x52;
const MSG_SUBSCRIBE_CHANNEL = 0x53;
const MSG_UNSUBSCRIBE_CHANNEL = 0x54;
const MSG_SUBSCRIBE_OK = 0x99;
const MSG_CHANNEL_PRESENCE = 0xAC;
const MSG_ERROR = 0x91;
const MSG_SERVER_CONFIG = 0x98;

// SUBSCRIBE_OK type field values (not message types!)
const SUBSCRIBE_TYPE_THREAD = 1;
const SUBSCRIBE_TYPE_CHANNEL = 2;

// Application state
class SuperChatClient {
  private ws: WebSocket | null = null;
  private nickname: string = '';
  private isRegistered: boolean = false; // Track if we're a registered user
  private currentChannel: Channel | null = null;
  private channels: Map<bigint, Channel> = new Map();
  private pingInterval: number | null = null;
  private frameBuffer: Uint8Array = new Uint8Array(0);
  private expectedFrameLength: number | null = null;

  // Traffic tracking
  private bytesSent: number = 0;
  private bytesReceived: number = 0;
  private bytesReceivedThrottled: number = 0; // Bytes we've "received" according to throttle
  private trafficUpdateInterval: number | null = null;
  private throttleBytesPerSecond: number = 0; // 0 = no throttle
  private pendingSends: Array<{data: Uint8Array, timestamp: number}> = [];

  // Receive throttling (buffer complete frames, not fragments)
  private frameReceiveBuffer: Array<{frame: Uint8Array, arrivedAt: number, size: number}> = [];
  private receiveProcessInterval: number | null = null;
  private lastReceiveProcessTime: number = 0;

  // Threading state
  private currentView: ViewState = ViewState.ThreadList;
  private currentThread: Message | null = null;
  private threads: Message[] = []; // Root messages only (parent_id.present === 0)
  private threadReplies: Map<bigint, Message[]> = new Map(); // Replies by thread root ID
  private replyToMessageId: bigint | null = null; // When composing a reply
  private replyingToMessage: Message | null = null; // Full message being replied to

  // Subscription tracking
  private subscribedChannelId: bigint | null = null;
  private subscribedThreadId: bigint | null = null;

  // Server list
  private servers: Array<{name: string, wsUrl: string, wssUrl: string, status: 'checking' | 'online' | 'offline', isSecure: boolean}> = [];
  private selectedServerUrl: string = '';
  private selectedServerIndex: number = -1;

  constructor() {
    this.setupEventListeners();
    this.initializeServers();
  }

  private initializeServers() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const hostname = window.location.hostname || 'localhost';

    this.servers = [];

    // Skip "Current Server" for Wails and other non-server hostnames
    const skipHostnames = ['wails', 'localhost', ''];
    if (!skipHostnames.includes(hostname)) {
      this.servers.push({
        name: 'Current Server',
        wsUrl: `ws://${hostname}:8080/ws`,
        wssUrl: `wss://${hostname}:8080/ws`,
        status: 'checking',
        isSecure: protocol === 'wss:'
      });
    }

    // Only add superchat.win if we're not already on it
    if (hostname !== 'superchat.win') {
      this.servers.push({
        name: 'superchat.win',
        wsUrl: 'ws://superchat.win:8080/ws',
        wssUrl: 'wss://superchat.win:8080/ws',
        status: 'checking',
        isSecure: false
      });
    }

    // Always add Custom Server last
    this.servers.push({
      name: 'Custom Server',
      wsUrl: '',
      wssUrl: '',
      status: 'offline',
      isSecure: false
    });

    // Restore last nickname
    const savedNickname = localStorage.getItem('superchat_nickname');
    if (savedNickname) {
      const nicknameInput = document.getElementById('nickname') as HTMLInputElement;
      if (nicknameInput) {
        nicknameInput.value = savedNickname;
      }
    }

    // Restore last selected server
    const savedServerIndex = localStorage.getItem('superchat_server_index');
    const savedCustomUrl = localStorage.getItem('superchat_custom_url');
    const savedServerSecure = localStorage.getItem('superchat_server_secure');

    console.log('Restoring from localStorage:', {
      savedServerIndex,
      savedCustomUrl,
      savedServerSecure
    });

    this.renderServerList();
    this.checkServerStatus();

    // Select saved server after rendering
    if (savedServerIndex !== null) {
      const index = parseInt(savedServerIndex, 10);
      if (index >= 0 && index < this.servers.length) {
        // Restore the isSecure flag before selecting
        if (savedServerSecure !== null) {
          this.servers[index].isSecure = savedServerSecure === 'true';
          console.log(`Restored server ${index} with isSecure=${this.servers[index].isSecure}`);
        }

        this.selectServer(index);

        // Restore custom URL if it was custom server
        if (index === this.servers.length - 1 && savedCustomUrl) {
          const serverUrlInput = document.getElementById('server-url') as HTMLInputElement;
          if (serverUrlInput) {
            serverUrlInput.value = savedCustomUrl;
          }
        }
      }
    }
  }

  private setupEventListeners() {
    // Connect form
    const form = document.getElementById('connect-form') as HTMLFormElement;
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const url = (document.getElementById('server-url') as HTMLInputElement).value;
      const nickname = (document.getElementById('nickname') as HTMLInputElement).value;
      const throttle = parseInt((document.getElementById('throttle-speed') as HTMLSelectElement).value, 10);
      this.throttleBytesPerSecond = throttle;

      // Save nickname and server selection to localStorage
      console.log('Saving to localStorage:', {
        nickname,
        serverIndex: this.selectedServerIndex,
        isSecure: this.servers[this.selectedServerIndex]?.isSecure,
        url
      });
      localStorage.setItem('superchat_nickname', nickname);
      localStorage.setItem('superchat_server_index', this.selectedServerIndex.toString());

      // Save which protocol (ws/wss) was used
      const selectedServer = this.servers[this.selectedServerIndex];
      if (selectedServer) {
        localStorage.setItem('superchat_server_secure', selectedServer.isSecure.toString());
      }

      // Save custom URL if custom server is selected
      if (this.selectedServerIndex === this.servers.length - 1) {
        localStorage.setItem('superchat_custom_url', url);
      }

      this.connect(url, nickname);
    });

    // Mobile menu toggle
    document.getElementById('mobile-menu-toggle')?.addEventListener('click', () => {
      this.toggleMobileSidebar();
    });

    // Send message button
    document.getElementById('send-button')?.addEventListener('click', () => {
      this.sendMessage();
    });

    // Send message on Enter (Ctrl+Enter for newline)
    document.getElementById('message-input')?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey && !e.ctrlKey) {
        e.preventDefault();
        this.sendMessage();
      }
      // Escape to cancel reply
      if (e.key === 'Escape' && this.replyToMessageId !== null) {
        this.cancelReply();
      }
    });

    // Back button
    document.addEventListener('click', (e) => {
      const target = e.target as HTMLElement;
      if (target.id === 'back-button') {
        this.backToThreadList();
      }
    });

    // Cancel reply button
    document.getElementById('cancel-reply-button')?.addEventListener('click', () => {
      this.cancelReply();
    });
  }

  private cancelReply() {
    this.replyToMessageId = null;
    this.replyingToMessage = null;
    this.updateComposeArea();
    this.updateReplyContext();
    this.showStatus('Reply cancelled', 'info');
  }

  private backToThreadList() {
    // Unsubscribe from current thread
    if (this.subscribedThreadId !== null) {
      this.unsubscribeFromThread(this.subscribedThreadId);
    }

    this.currentThread = null;
    this.currentView = ViewState.ThreadList;
    this.renderMessages();
    this.updateBackButton();
    this.updateComposeArea();
  }

  private updateBackButton() {
    const backButton = document.getElementById('back-button');
    if (backButton) {
      backButton.style.display = this.currentView === ViewState.ThreadDetail ? 'inline-block' : 'none';
    }
  }

  private updateComposeArea() {
    const composeArea = document.getElementById('compose-area');
    const input = document.getElementById('message-input') as HTMLTextAreaElement;

    if (!composeArea || !input) return;

    // Chat channels (type 0) always show compose area
    if (this.currentChannel && this.currentChannel.type === 0) {
      composeArea.style.display = 'block';
      input.placeholder = 'Type a message...';
    } else if (this.currentView === ViewState.ThreadList) {
      // Forum thread list: show compose area for creating new threads
      composeArea.style.display = 'block';
      input.placeholder = 'Start a new conversation...';
    } else {
      // Forum thread detail: only show when user clicked reply
      const shouldShow = this.replyToMessageId !== null;
      composeArea.style.display = shouldShow ? 'block' : 'none';

      if (shouldShow) {
        input.placeholder = 'Type your reply...';
      } else {
        input.value = '';
      }
    }
  }

  private updateReplyContext() {
    const replyContext = document.getElementById('reply-context');
    const replyToAuthor = document.getElementById('reply-to-author');
    const replyToPreview = document.getElementById('reply-to-preview');

    if (!replyContext || !replyToAuthor || !replyToPreview) return;

    // Remove highlight from any previously highlighted message
    document.querySelectorAll('.reply-target').forEach(el => {
      el.classList.remove('reply-target');
    });

    if (this.replyingToMessage) {
      // Show reply context
      replyContext.style.display = 'block';
      replyToAuthor.textContent = this.replyingToMessage.author_nickname;

      // Show preview of message content (first 50 chars)
      const preview = this.replyingToMessage.content.length > 50
        ? this.replyingToMessage.content.substring(0, 50) + '...'
        : this.replyingToMessage.content;
      replyToPreview.textContent = `"${preview}"`;

      // Highlight the message being replied to
      const messageElement = document.querySelector(`[data-message-id="${this.replyingToMessage.message_id}"]`);
      if (messageElement) {
        messageElement.classList.add('reply-target');
      }
    } else {
      // Hide reply context
      replyContext.style.display = 'none';
      replyToAuthor.textContent = '';
      replyToPreview.textContent = '';
    }
  }

  private toggleMobileSidebar() {
    const sidebar = document.getElementById('sidebar');
    const toggle = document.getElementById('mobile-menu-toggle');
    sidebar?.classList.toggle('mobile-open');
    toggle?.classList.toggle('active');
  }

  private closeMobileSidebar() {
    const sidebar = document.getElementById('sidebar');
    const toggle = document.getElementById('mobile-menu-toggle');
    sidebar?.classList.remove('mobile-open');
    toggle?.classList.remove('active');
  }

  async connect(url: string, nickname: string) {
    this.nickname = nickname;

    try {
      this.ws = new WebSocket(url);
      this.ws.binaryType = 'arraybuffer';

      this.ws.onopen = () => {
        console.log('WebSocket connected');
        this.sendSetNickname(nickname);
        // Start ping interval (30 seconds)
        this.pingInterval = window.setInterval(() => this.sendPing(), 30000);
        // Start traffic stats update interval (1 second)
        this.trafficUpdateInterval = window.setInterval(() => this.updateTrafficStats(), 1000);
        // Start receive processing interval if throttling is enabled
        if (this.throttleBytesPerSecond > 0) {
          this.lastReceiveProcessTime = Date.now();
          // Process receive buffer every 100ms
          this.receiveProcessInterval = window.setInterval(() => this.processReceiveBuffer(), 100);
        }
      };

      this.ws.onmessage = (event) => {
        const data = new Uint8Array(event.data);
        this.bytesReceived += data.length;
        // Always process fragments immediately to assemble frames
        this.handleFragment(data);
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        const errorMsg = error instanceof ErrorEvent && error.message ? error.message : error.toString();
        this.showStatus(`Failed to connect to ${url}: ${errorMsg}`, 'error');
      };

      this.ws.onclose = (event) => {
        console.log('WebSocket closed', event);
        if (event.wasClean) {
          this.showStatus('Disconnected from server', 'error');
        } else {
          this.showStatus(`Connection lost (code: ${event.code}). Server may be offline or unreachable.`, 'error');
        }
        if (this.pingInterval) {
          clearInterval(this.pingInterval);
          this.pingInterval = null;
        }
        if (this.trafficUpdateInterval) {
          clearInterval(this.trafficUpdateInterval);
          this.trafficUpdateInterval = null;
        }
        if (this.receiveProcessInterval) {
          clearInterval(this.receiveProcessInterval);
          this.receiveProcessInterval = null;
        }
      };
    } catch (error) {
      console.error('Failed to connect:', error);
      this.showStatus('Failed to connect', 'error');
    }
  }

  private sendFrame(messageType: number, payloadBytes: Uint8Array) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.error('WebSocket not connected');
      return;
    }

    // Encode frame header
    const headerEncoder = new FrameHeaderEncoder();
    const header = headerEncoder.encode({
      length: 3 + payloadBytes.length, // version(1) + type(1) + flags(1) + payload
      version: 1,
      type: messageType,
      flags: 0
    });

    // Combine header + payload
    const frame = new Uint8Array(header.length + payloadBytes.length);
    frame.set(header, 0);
    frame.set(payloadBytes, header.length);

    // Track bytes sent
    this.bytesSent += frame.length;

    // If throttling is enabled, queue the send
    if (this.throttleBytesPerSecond > 0) {
      this.pendingSends.push({ data: frame, timestamp: Date.now() });
      this.processPendingSends();
    } else {
      this.ws.send(frame);
    }
  }

  private processPendingSends() {
    if (this.pendingSends.length === 0 || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }

    // Simple rate limiting: send one frame per interval based on throttle rate
    // This is a rough approximation - real throttling would need more sophisticated flow control
    const bytesPerInterval = Math.max(100, this.throttleBytesPerSecond / 10); // 100ms intervals

    while (this.pendingSends.length > 0) {
      const pending = this.pendingSends[0];

      // Check if we can send based on throttle
      if (pending.data.length <= bytesPerInterval) {
        this.pendingSends.shift();
        this.ws.send(pending.data);
      } else {
        // Wait for next interval
        setTimeout(() => this.processPendingSends(), 100);
        break;
      }
    }
  }

  private processReceiveBuffer() {
    if (this.frameReceiveBuffer.length === 0) {
      return;
    }

    const now = Date.now();
    const elapsedMs = now - this.lastReceiveProcessTime;
    const bytesAllowed = (this.throttleBytesPerSecond * elapsedMs) / 1000;

    let bytesProcessed = 0;
    const toProcess: Uint8Array[] = [];

    // Process as many buffered frames as the throttle allows
    while (this.frameReceiveBuffer.length > 0 && bytesProcessed < bytesAllowed) {
      const buffered = this.frameReceiveBuffer[0];

      // Check if we have enough "bandwidth" to process this frame
      if (bytesProcessed + buffered.size <= bytesAllowed) {
        this.frameReceiveBuffer.shift();
        toProcess.push(buffered.frame);
        bytesProcessed += buffered.size;
        this.bytesReceivedThrottled += buffered.size;
      } else {
        // Not enough bandwidth for this frame yet
        break;
      }
    }

    // Update last process time
    this.lastReceiveProcessTime = now;

    // Process all the frames we're allowed to
    for (const frame of toProcess) {
      console.log(`Processing frame ${frame.length} bytes from buffer (${this.frameReceiveBuffer.length} remaining)`);
      this.handleCompleteFrame(frame);
    }
  }

  private handleFragment(fragment: Uint8Array) {
    // Append fragment to buffer
    const newBuffer = new Uint8Array(this.frameBuffer.length + fragment.length);
    newBuffer.set(this.frameBuffer, 0);
    newBuffer.set(fragment, this.frameBuffer.length);
    this.frameBuffer = newBuffer;

    // Try to extract complete frames from buffer
    while (true) {
      // Need at least 4 bytes to read frame length
      if (this.expectedFrameLength === null && this.frameBuffer.length >= 4) {
        // Read frame length (big-endian uint32)
        const view = new DataView(this.frameBuffer.buffer, this.frameBuffer.byteOffset, 4);
        this.expectedFrameLength = view.getUint32(0, false); // false = big-endian
        console.log(`Expecting frame of ${this.expectedFrameLength} bytes (plus 4-byte length prefix)`);
      }

      // Check if we have a complete frame (length prefix + frame data)
      if (this.expectedFrameLength !== null) {
        const totalFrameSize = 4 + this.expectedFrameLength;
        if (this.frameBuffer.length >= totalFrameSize) {
          // Extract complete frame
          const completeFrame = this.frameBuffer.slice(0, totalFrameSize);

          // Remove frame from buffer
          this.frameBuffer = this.frameBuffer.slice(totalFrameSize);
          this.expectedFrameLength = null;

          // If throttling is enabled, buffer the complete frame
          if (this.throttleBytesPerSecond > 0) {
            this.frameReceiveBuffer.push({
              frame: completeFrame,
              arrivedAt: Date.now(),
              size: totalFrameSize
            });
            console.log(`Buffered frame ${totalFrameSize} bytes (buffer: ${this.frameReceiveBuffer.length} frames)`);
          } else {
            // No throttling - process immediately
            this.handleCompleteFrame(completeFrame);
          }

          // Continue to check for more frames in buffer
          continue;
        }
      }

      // Not enough data yet, wait for more fragments
      break;
    }
  }

  private handleCompleteFrame(data: Uint8Array) {
    this.handleMessage(data);
  }

  private handleMessage(data: Uint8Array) {
    try {
      // Debug: log raw frame data
      console.log(`Received frame: ${data.length} bytes, first 20:`, Array.from(data.slice(0, 20)));

      // Check minimum frame size
      if (data.length < 7) {
        console.error(`Frame too short: ${data.length} bytes (need at least 7 for header)`);
        return;
      }

      // Decode frame header
      const headerDecoder = new FrameHeaderDecoder(data);
      const header = headerDecoder.decode();

      console.log(`Decoded header: length=${header.length}, version=${header.version}, type=0x${header.type.toString(16)}, flags=${header.flags}`);

      // Extract payload (skip length(4) + version(1) + type(1) + flags(1) = 7 bytes)
      // Create a completely fresh copy with a new ArrayBuffer at offset 0
      const payloadSlice = data.slice(7);
      const payload = new Uint8Array(new ArrayBuffer(payloadSlice.length));
      payload.set(payloadSlice);
      console.log(`Payload: ${payload.length} bytes, byteOffset: ${payload.byteOffset}`);

      switch (header.type) {
        case MSG_SERVER_CONFIG:
          this.handleServerConfig(payload);
          break;
        case MSG_NICKNAME_RESPONSE:
          this.handleNicknameResponse(payload);
          break;
        case MSG_CHANNEL_LIST:
          this.handleChannelList(payload);
          break;
        case MSG_JOIN_RESPONSE:
          this.handleJoinResponse(payload);
          break;
        case MSG_MESSAGE_LIST:
          this.handleMessageList(payload);
          break;
        case MSG_MESSAGE_POSTED:
          this.handleMessagePosted(payload);
          break;
        case MSG_NEW_MESSAGE:
          this.handleNewMessage(payload);
          break;
        case MSG_PONG:
          console.log('Received PONG');
          break;
        case MSG_SUBSCRIBE_OK:
          this.handleSubscribeOk(payload);
          break;
        case MSG_CHANNEL_PRESENCE:
          // Channel presence notifications (users joining/leaving) - ignore for now
          console.log('Received CHANNEL_PRESENCE notification');
          break;
        case MSG_ERROR:
          this.handleError(payload);
          break;
        default:
          console.warn(`Unhandled message type: 0x${header.type.toString(16)} (decimal ${header.type})`);
      }
    } catch (error) {
      console.error('Error handling message:', error);
      console.error('Frame data:', Array.from(data));
    }
  }

  private sendSetNickname(nickname: string) {
    const encoder = new SetNicknameEncoder();
    const payload = encoder.encode({ nickname });
    this.sendFrame(MSG_SET_NICKNAME, payload);
  }

  private handleServerConfig(payload: Uint8Array) {
    try {
      const decoder = new ServerConfigDecoder(payload);
      const config = decoder.decode();
      console.log('Received SERVER_CONFIG:', config);
      // Store config if needed in the future
    } catch (error) {
      console.error('Error decoding SERVER_CONFIG:', error);
    }
  }

  private handleError(payload: Uint8Array) {
    try {
      const decoder = new Error_Decoder(payload);
      const error = decoder.decode();
      console.error(`Server error ${error.error_code}: ${error.message}`);
      this.showStatus(`Server error: ${error.message}`, 'error');
    } catch (error) {
      console.error('Error decoding ERROR message:', error);
    }
  }

  private handleNicknameResponse(payload: Uint8Array) {
    const decoder = new NicknameResponseDecoder(payload);
    const response = decoder.decode();

    if (response.success === 1) {
      console.log('Nickname set successfully:', response.message);
      this.showStatus(response.message, 'success');

      // Server echoes back the actual nickname (with ~ if anonymous)
      // We can extract it from the success message or just check our original nickname
      // If server accepted it without ~, we're registered
      this.isRegistered = !this.nickname.startsWith('~');

      // Hide connect panel and show app
      document.getElementById('connect-panel')?.classList.add('hidden');
      document.getElementById('app')!.style.display = 'flex';

      // Request channel list
      this.sendListChannels();
    } else {
      this.showStatus(response.message, 'error');
    }
  }

  private sendListChannels() {
    const encoder = new ListChannelsEncoder();
    const payload = encoder.encode({ from_channel_id: 0n, limit: 100 });
    this.sendFrame(MSG_LIST_CHANNELS, payload);
  }

  private handleChannelList(payload: Uint8Array) {
    try {
      const decoder = new ChannelListDecoder(payload);
      const channelList = decoder.decode();

      console.log(`Received ${channelList.channel_count} channels`);

      // Store channels
      this.channels.clear();
      for (const channel of channelList.channels) {
        this.channels.set(channel.channel_id, channel);
      }

      // Update UI
      this.renderChannels();
    } catch (error) {
      console.error('Error decoding CHANNEL_LIST:', error);
      throw error;
    }
  }

  private renderChannels() {
    const list = document.getElementById('channel-list')!;
    list.innerHTML = '';

    for (const channel of this.channels.values()) {
      const item = document.createElement('div');
      item.className = 'channel-item';
      if (this.currentChannel?.channel_id === channel.channel_id) {
        item.classList.add('active');
      }

      const prefix = channel.type === 0 ? '>' : '#';
      item.innerHTML = `
        <div class="channel-name">${prefix} ${channel.name}</div>
        <div class="channel-info">${channel.type === 0 ? 'Chat' : 'Forum'}</div>
      `;

      item.addEventListener('click', () => {
        this.joinChannel(channel);
        this.closeMobileSidebar();
      });
      list.appendChild(item);
    }
  }

  private joinChannel(channel: Channel) {
    const encoder = new JoinChannelEncoder();
    const payload = encoder.encode({
      channel_id: channel.channel_id,
      subchannel_id: { present: 0 }
    });
    this.sendFrame(MSG_JOIN_CHANNEL, payload);
  }

  private handleJoinResponse(payload: Uint8Array) {
    const decoder = new JoinResponseDecoder(payload);
    const response = decoder.decode();

    if (response.success === 1) {
      const channel = this.channels.get(response.channel_id);
      if (channel) {
        this.currentChannel = channel;
        console.log('Joined channel:', channel.name);

        // Reset view state when switching channels
        if (this.subscribedThreadId !== null) {
          this.unsubscribeFromThread(this.subscribedThreadId);
        }
        this.currentView = ViewState.ThreadList;
        this.currentThread = null;
        this.replyToMessageId = null;
        this.replyingToMessage = null;
        this.threads = [];
        this.threadReplies.clear();

        // Update UI
        this.updateChannelTitle();
        this.renderChannels();
        this.updateBackButton();
        this.updateComposeArea();

        // Subscribe to channel for real-time updates
        this.subscribeToChannel(channel.channel_id);

        // Request messages
        this.sendListMessages(channel.channel_id);
      }
    } else {
      this.showStatus(response.message, 'error');
    }
  }

  private subscribeToChannel(channelId: bigint) {
    // Always unsubscribe from previous channel if we think we're subscribed to one
    if (this.subscribedChannelId !== null) {
      this.unsubscribeFromChannel(this.subscribedChannelId);
    }

    const encoder = new SubscribeChannelEncoder();
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 }
    });
    this.sendFrame(MSG_SUBSCRIBE_CHANNEL, payload);
    console.log(`Subscribing to channel ${channelId}...`);

    // Optimistically set the subscribed channel immediately
    this.subscribedChannelId = channelId;
  }

  private unsubscribeFromChannel(channelId: bigint) {
    const encoder = new UnsubscribeChannelEncoder();
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 }
    });
    this.sendFrame(MSG_UNSUBSCRIBE_CHANNEL, payload);
    console.log(`Unsubscribing from channel ${channelId}...`);

    // Clear local state immediately
    this.subscribedChannelId = null;
  }

  private subscribeToThread(threadId: bigint) {
    // Always unsubscribe from previous thread if we think we're subscribed to one
    if (this.subscribedThreadId !== null) {
      this.unsubscribeFromThread(this.subscribedThreadId);
    }

    const encoder = new SubscribeThreadEncoder();
    const payload = encoder.encode({ thread_id: threadId });
    this.sendFrame(MSG_SUBSCRIBE_THREAD, payload);
    console.log(`Subscribing to thread ${threadId}...`);

    // Optimistically set the subscribed thread immediately
    this.subscribedThreadId = threadId;
  }

  private unsubscribeFromThread(threadId: bigint) {
    const encoder = new UnsubscribeThreadEncoder();
    const payload = encoder.encode({ thread_id: threadId });
    this.sendFrame(MSG_UNSUBSCRIBE_THREAD, payload);
    console.log(`Unsubscribing from thread ${threadId}...`);

    // Clear local state immediately
    this.subscribedThreadId = null;
  }

  private handleSubscribeOk(payload: Uint8Array) {
    try {
      const decoder = new SubscribeOkDecoder(payload);
      const response = decoder.decode();

      if (response.type === SUBSCRIBE_TYPE_CHANNEL) {
        console.log(`Subscription confirmed for channel ${response.id}`);
        this.updateChannelTitle(); // Update title to show connection indicator
      } else if (response.type === SUBSCRIBE_TYPE_THREAD) {
        console.log(`Subscription confirmed for thread ${response.id}`);
      }
    } catch (error) {
      console.error('Error decoding SUBSCRIBE_OK:', error);
    }
  }

  private sendListMessages(channelId: bigint) {
    const encoder = new ListMessagesEncoder();
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 },
      limit: 50,
      before_id: { present: 0 },
      parent_id: { present: 0 },
      after_id: { present: 0 }
    });
    this.sendFrame(MSG_LIST_MESSAGES, payload);
  }

  private handleMessageList(payload: Uint8Array) {
    const decoder = new MessageListDecoder(payload);
    const messageList = decoder.decode();

    console.log(`Received ${messageList.message_count} messages for channel ${messageList.channel_id}`);

    // Check if this is a response for thread replies (parent_id was set in request)
    const isThreadReplies = messageList.parent_id.present === 1;

    if (isThreadReplies) {
      // These are replies to a specific thread
      const threadId = messageList.parent_id.value!;
      this.threadReplies.set(threadId, messageList.messages);
      console.log(`Loaded ${messageList.messages.length} replies for thread ${threadId}`);
    } else {
      // These are root-level messages (threads)
      // Filter to only include messages with no parent
      this.threads = messageList.messages.filter(msg => msg.parent_id.present === 0);
      console.log(`Loaded ${this.threads.length} threads`);
    }

    // Update UI if this is the current channel
    if (this.currentChannel?.channel_id === messageList.channel_id) {
      this.renderMessages();

      // Auto-scroll to bottom when loading thread replies in chat channels (type 0)
      console.log(`handleMessageList: isThreadReplies=${isThreadReplies}, currentView=${this.currentView}, channel.type=${this.currentChannel.type}`);
      if (isThreadReplies && this.currentView === ViewState.ThreadDetail && this.currentChannel.type === 0) {
        console.log('Auto-scrolling after loading thread replies');
        setTimeout(() => {
          const container = document.getElementById('messages');
          if (container) {
            console.log(`Scrolling: scrollTop=${container.scrollTop} -> scrollHeight=${container.scrollHeight}`);
            container.scrollTop = container.scrollHeight;
          }
        }, 50); // Small delay to ensure render is complete
      }
    }
  }

  private renderMessages() {
    // Chat channels (type 0) show a flat list of all messages
    if (this.currentChannel && this.currentChannel.type === 0) {
      this.renderChatMessages();
    } else if (this.currentView === ViewState.ThreadList) {
      this.renderThreadList();
    } else {
      this.renderThreadDetail();
    }
  }

  private renderChatMessages() {
    const container = document.getElementById('messages')!;

    if (!this.currentChannel) {
      container.innerHTML = '<div class="empty-state"><h3>Select a channel</h3></div>';
      return;
    }

    if (this.threads.length === 0) {
      container.innerHTML = '<div class="empty-state"><h3>No messages yet</h3><p>Start chatting!</p></div>';
      return;
    }

    container.innerHTML = '';

    let lastDate: string | null = null;

    // Render all messages in chronological order
    for (const message of this.threads) {
      const date = new Date(Number(message.created_at));
      const dateStr = date.toLocaleDateString();
      const timeStr = date.toLocaleTimeString();

      // Show date separator if date changed
      if (lastDate !== dateStr) {
        const dateSeparator = document.createElement('div');
        dateSeparator.className = 'date-separator';
        dateSeparator.textContent = dateStr;
        container.appendChild(dateSeparator);
        lastDate = dateStr;
      }

      const div = document.createElement('div');
      div.className = 'chat-message';
      div.setAttribute('data-message-id', message.message_id.toString());

      div.innerHTML = `
        <span class="chat-time">${timeStr}</span>
        <span class="chat-author">${this.escapeHtml(message.author_nickname)}</span>
        <span class="chat-content">${this.escapeHtml(message.content)}</span>
      `;

      container.appendChild(div);
    }

    // Auto-scroll to bottom for chat channels
    console.log(`renderChatMessages: scrolling to bottom, scrollHeight=${container.scrollHeight}`);
    container.scrollTop = container.scrollHeight;
  }

  private renderThreadList() {
    const container = document.getElementById('messages')!;

    if (!this.currentChannel) {
      container.innerHTML = '<div class="empty-state"><h3>Select a channel</h3></div>';
      return;
    }

    if (this.threads.length === 0) {
      container.innerHTML = '<div class="empty-state"><h3>No threads yet</h3><p>Start a conversation!</p></div>';
      return;
    }

    container.innerHTML = '';

    for (const thread of this.threads) {
      const div = document.createElement('div');
      div.className = 'thread-item';

      const date = new Date(Number(thread.created_at));
      const timeStr = date.toLocaleTimeString();

      // Create preview (first 80 chars of content)
      const preview = thread.content.length > 80
        ? thread.content.substring(0, 80) + '...'
        : thread.content;

      // Reply count (show next to time in header)
      const replyCount = thread.reply_count > 0
        ? ` • ${thread.reply_count} ${thread.reply_count === 1 ? 'reply' : 'replies'}`
        : '';

      div.innerHTML = `
        <div class="thread-header">
          <span class="thread-author">${this.escapeHtml(thread.author_nickname)}</span>
          <span class="thread-time">${timeStr}${replyCount}</span>
        </div>
        <div class="thread-preview">${this.escapeHtml(preview)}</div>
      `;

      // Click to open thread
      div.addEventListener('click', () => {
        this.openThread(thread);
      });

      container.appendChild(div);
    }

    // Scroll to top when showing thread list
    container.scrollTop = 0;
  }

  private renderThreadDetail() {
    const container = document.getElementById('messages')!;

    if (!this.currentThread) {
      container.innerHTML = '<div class="empty-state"><h3>No thread selected</h3></div>';
      return;
    }

    container.innerHTML = '';

    // Show root message
    this.renderMessage(container, this.currentThread, 0, true);

    // Build tree and render replies
    const replies = this.threadReplies.get(this.currentThread.message_id) || [];
    const messageTree = this.buildMessageTree(replies, this.currentThread.message_id);
    this.renderMessageTree(container, messageTree, 1);

    // Add click handlers for reply buttons
    container.querySelectorAll('.reply-button').forEach(button => {
      button.addEventListener('click', (e) => {
        e.stopPropagation();
        const messageId = BigInt((e.target as HTMLElement).getAttribute('data-message-id')!);
        this.replyToMessage(messageId);
      });
    });

    // Scroll to bottom only for chat channels (type 0)
    console.log(`renderThreadDetail: channel.type=${this.currentChannel?.type}`);
    if (this.currentChannel && this.currentChannel.type === 0) {
      console.log(`Auto-scrolling in renderThreadDetail: scrollHeight=${container.scrollHeight}`);
      container.scrollTop = container.scrollHeight;
    }
  }

  private buildMessageTree(messages: Message[], rootId: bigint): Message[] {
    // Build a map of parent_id -> children
    const childrenMap = new Map<bigint, Message[]>();

    for (const msg of messages) {
      if (msg.parent_id.present === 1) {
        const parentId = msg.parent_id.value!;
        if (!childrenMap.has(parentId)) {
          childrenMap.set(parentId, []);
        }
        childrenMap.get(parentId)!.push(msg);
      }
    }

    // Get direct children of the root
    return childrenMap.get(rootId) || [];
  }

  private renderMessageTree(container: HTMLElement, messages: Message[], depth: number) {
    for (const msg of messages) {
      this.renderMessage(container, msg, depth, false);

      // Recursively render children
      const replies = this.threadReplies.get(this.currentThread!.message_id) || [];
      const children = this.buildMessageTree(replies, msg.message_id);
      if (children.length > 0) {
        this.renderMessageTree(container, children, depth + 1);
      }
    }
  }

  private renderMessage(container: HTMLElement, message: Message, depth: number, isRoot: boolean) {
    const div = document.createElement('div');
    div.className = isRoot ? 'message thread-root' : 'message thread-reply';
    div.setAttribute('data-message-id', message.message_id.toString());

    // Add indentation based on depth (but not for root)
    if (!isRoot && depth > 0) {
      div.style.marginLeft = `${depth * 2}rem`;
    }

    const date = new Date(Number(message.created_at));
    const timeStr = date.toLocaleTimeString();

    div.innerHTML = `
      <div class="message-header">
        <span class="message-author">${this.escapeHtml(message.author_nickname)}</span>
        <span class="message-time">${timeStr}</span>
      </div>
      <div class="message-content">${this.escapeHtml(message.content)}</div>
      <div class="message-actions">
        <button class="reply-button" data-message-id="${message.message_id}">Reply</button>
      </div>
    `;

    container.appendChild(div);
  }

  private openThread(thread: Message) {
    this.currentThread = thread;
    this.currentView = ViewState.ThreadDetail;
    this.updateBackButton();
    this.updateComposeArea();

    // Subscribe to thread for real-time reply updates
    this.subscribeToThread(thread.message_id);

    // Load replies for this thread
    this.loadThreadReplies(thread.message_id);
  }

  private loadThreadReplies(threadId: bigint) {
    const encoder = new ListMessagesEncoder();
    const payload = encoder.encode({
      channel_id: this.currentChannel!.channel_id,
      subchannel_id: { present: 0 },
      limit: 100,
      before_id: { present: 0 },
      parent_id: { present: 1, value: threadId },
      after_id: { present: 0 }
    });
    this.sendFrame(MSG_LIST_MESSAGES, payload);
  }

  private replyToMessage(messageId: bigint) {
    this.replyToMessageId = messageId;

    // Find the message being replied to
    if (this.currentThread && this.currentThread.message_id === messageId) {
      this.replyingToMessage = this.currentThread;
    } else {
      const replies = this.threadReplies.get(this.currentThread!.message_id) || [];
      this.replyingToMessage = replies.find(m => m.message_id === messageId) || null;
    }

    this.updateComposeArea();
    this.updateReplyContext();

    // Focus the input
    const input = document.getElementById('message-input') as HTMLTextAreaElement;
    input.focus();

    this.showStatus('Reply mode active - press Escape to cancel', 'info');
  }

  private handleNewMessage(payload: Uint8Array) {
    const decoder = new NewMessageDecoder(payload);
    const newMsg = decoder.decode();

    console.log('Received new message:', newMsg);

    const message: Message = {
      message_id: newMsg.message_id,
      channel_id: newMsg.channel_id,
      subchannel_id: newMsg.subchannel_id,
      parent_id: newMsg.parent_id,
      author_user_id: newMsg.author_user_id,
      author_nickname: newMsg.author_nickname,
      content: newMsg.content,
      created_at: newMsg.created_at,
      edited_at: newMsg.edited_at,
      reply_count: newMsg.reply_count
    };

    // Check if this is our own message by comparing nicknames
    const isOurMessage = this.isOwnMessage(message.author_nickname);

    // Determine if this is a root message (thread) or a reply
    if (message.parent_id.present === 0) {
      // Root message - add to threads list
      this.threads.push(message);
      console.log('Added new thread to list');
    } else {
      // Reply - we need to find the ROOT of the thread to add it there
      // The message's parent_id might be the root OR another reply
      const parentId = message.parent_id.value!;

      // Find which thread this belongs to (could be root or need to traverse up)
      let rootThreadId = parentId;

      // Check if parentId is a root thread
      const isRootThread = this.threads.some(t => t.message_id === parentId);
      if (!isRootThread) {
        // Parent is a reply, find its root by checking currentThread
        if (this.currentThread) {
          rootThreadId = this.currentThread.message_id;
        }
      }

      // Add to the root thread's replies
      const replies = this.threadReplies.get(rootThreadId) || [];
      replies.push(message);
      this.threadReplies.set(rootThreadId, replies);
      console.log(`Added reply to thread ${rootThreadId}`);

      // Update reply count in threads list (only increment for the root thread)
      const thread = this.threads.find(t => t.message_id === rootThreadId);
      if (thread) {
        thread.reply_count++;
      }
    }

    // Update UI if this is the current channel
    if (this.currentChannel?.channel_id === newMsg.channel_id) {
      this.renderMessages();

      // Auto-scroll to bottom for chat channels (type 0) or when viewing thread detail
      const shouldScroll = this.currentChannel.type === 0 || this.currentView === ViewState.ThreadDetail;
      if (shouldScroll) {
        setTimeout(() => {
          const container = document.getElementById('messages');
          if (container) {
            container.scrollTop = container.scrollHeight;
          }
        }, 50); // Small delay to ensure render is complete
      }
    }
  }

  private sendMessage() {
    const input = document.getElementById('message-input') as HTMLTextAreaElement;
    const content = input.value.trim();

    if (!content || !this.currentChannel) {
      return;
    }

    const encoder = new PostMessageEncoder();
    const payload = encoder.encode({
      channel_id: this.currentChannel.channel_id,
      subchannel_id: { present: 0 },
      parent_id: this.replyToMessageId !== null
        ? { present: 1, value: this.replyToMessageId }
        : { present: 0 },
      content
    });
    this.sendFrame(MSG_POST_MESSAGE, payload);

    // Clear input and reply context
    input.value = '';
    this.replyToMessageId = null;
    this.replyingToMessage = null;
    this.updateComposeArea();
    this.updateReplyContext();
  }

  private handleMessagePosted(payload: Uint8Array) {
    const decoder = new MessagePostedDecoder(payload);
    const response = decoder.decode();

    if (response.success === 1) {
      console.log('Message posted successfully:', response.message_id);
    } else {
      this.showStatus(response.message, 'error');
    }
  }

  private sendPing() {
    const encoder = new PingEncoder();
    const payload = encoder.encode({ timestamp: BigInt(Date.now()) });
    this.sendFrame(MSG_PING, payload);
  }

  private showStatus(message: string, type: 'success' | 'error' | 'info' = 'info') {
    // Create status element if it doesn't exist
    let status = document.getElementById('status-bar');
    if (!status) {
      status = document.createElement('div');
      status.id = 'status-bar';
      status.className = 'status';
      document.body.prepend(status);
    }

    status.textContent = message;
    status.className = `status ${type}`;

    // Auto-hide after 3 seconds
    setTimeout(() => {
      status?.remove();
    }, 3000);
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  private isOwnMessage(authorNickname: string): boolean {
    // If we're registered, check for exact match
    if (this.isRegistered) {
      return authorNickname === this.nickname;
    }
    // If we're anonymous, check for ~nickname
    return authorNickname === `~${this.nickname}`;
  }

  private formatBytes(bytes: number): string {
    const unit = 1024;
    if (bytes < unit) {
      return `${bytes}B`;
    }
    let div = unit;
    let exp = 0;
    for (let n = Math.floor(bytes / unit); n >= unit; n = Math.floor(n / unit)) {
      div *= unit;
      exp++;
    }
    const units = 'KMGTPE';
    return `${(bytes / div).toFixed(1)}${units[exp]}B`;
  }

  private renderServerList() {
    const serverList = document.getElementById('server-list');
    if (!serverList) return;

    serverList.innerHTML = '';

    this.servers.forEach((server, index) => {
      const url = server.isSecure ? server.wssUrl : server.wsUrl;
      const isCustom = server.name === 'Custom Server';
      const isSelected = this.selectedServerIndex === index;

      const serverItem = document.createElement('div');
      serverItem.className = `server-item ${isSelected ? 'selected' : ''}`;
      serverItem.onclick = () => this.selectServer(index);

      const serverInfo = document.createElement('div');
      serverInfo.className = 'server-info';

      const serverName = document.createElement('div');
      serverName.className = 'server-name';
      serverName.textContent = server.name;

      serverInfo.appendChild(serverName);

      // Only show URL and badge for non-custom servers
      if (!isCustom) {
        const statusDot = document.createElement('div');
        statusDot.className = `server-status ${server.status}`;
        serverItem.appendChild(statusDot);

        const serverUrl = document.createElement('div');
        serverUrl.className = 'server-url';
        serverUrl.textContent = url;
        serverInfo.appendChild(serverUrl);

        serverItem.appendChild(serverInfo);

        const badge = document.createElement('span');
        badge.className = `server-badge ${server.isSecure ? 'secure' : 'insecure'}`;
        badge.textContent = server.isSecure ? 'WSS' : 'WS';
        serverItem.appendChild(badge);
      } else {
        const serverDescription = document.createElement('div');
        serverDescription.className = 'server-url';
        serverDescription.textContent = 'Enter your own server URL';
        serverInfo.appendChild(serverDescription);

        serverItem.appendChild(serverInfo);
      }

      serverList.appendChild(serverItem);
    });

    // Select first server by default if none selected
    if (this.selectedServerIndex === -1 && this.servers.length > 0) {
      this.selectServer(0);
    }
  }

  private selectServer(index: number) {
    const server = this.servers[index];
    const isCustom = server.name === 'Custom Server';

    this.selectedServerIndex = index;
    this.selectedServerUrl = server.isSecure ? server.wssUrl : server.wsUrl;

    const serverUrlInput = document.getElementById('server-url') as HTMLInputElement;
    const serverUrlGroup = serverUrlInput?.closest('.form-group') as HTMLElement;

    if (serverUrlInput) {
      serverUrlInput.value = this.selectedServerUrl;
      serverUrlInput.readOnly = !isCustom;
      serverUrlInput.placeholder = isCustom ? 'Enter server URL (ws:// or wss://)' : '';

      // Show/hide the server URL field
      if (serverUrlGroup) {
        serverUrlGroup.style.display = isCustom ? 'block' : 'none';
      }

      // Focus the input if custom server is selected
      if (isCustom) {
        serverUrlInput.focus();
      }
    }

    this.renderServerList();
  }

  private async checkServerStatus() {
    for (let i = 0; i < this.servers.length; i++) {
      const server = this.servers[i];

      // Try secure first, then insecure
      const urlsToTry = [server.wssUrl, server.wsUrl];
      let isOnline = false;
      let secureWorks = false;

      for (const url of urlsToTry) {
        try {
          const online = await this.probeServer(url);
          if (online) {
            isOnline = true;
            secureWorks = url === server.wssUrl;
            break;
          }
        } catch (error) {
          // Try next URL
        }
      }

      this.servers[i].status = isOnline ? 'online' : 'offline';
      this.servers[i].isSecure = secureWorks;
      this.renderServerList();
    }
  }

  private probeServer(url: string): Promise<boolean> {
    return new Promise((resolve) => {
      const ws = new WebSocket(url);
      const timeout = setTimeout(() => {
        ws.close();
        resolve(false);
      }, 3000);

      ws.onopen = () => {
        clearTimeout(timeout);
        ws.close();
        resolve(true);
      };

      ws.onerror = () => {
        clearTimeout(timeout);
        resolve(false);
      };
    });
  }

  private updateChannelTitle() {
    if (!this.currentChannel) return;

    const titleEl = document.getElementById('channel-title');
    if (!titleEl) {
      console.warn('channel-title element not found');
      return;
    }

    const prefix = this.currentChannel.type === 0 ? '>' : '#';

    // Create title with connection indicator using flexbox for alignment
    titleEl.innerHTML = '';
    titleEl.style.display = 'flex';
    titleEl.style.alignItems = 'center';
    titleEl.style.gap = '8px';

    // Show green indicator when we have an active channel
    const indicator = document.createElement('span');
    indicator.style.cssText = 'display: block; width: 8px; height: 8px; border-radius: 50%; background: #10b981; box-shadow: 0 0 6px rgba(16, 185, 129, 0.6); flex-shrink: 0;';
    titleEl.appendChild(indicator);

    const text = document.createTextNode(`${prefix} ${this.currentChannel.name}`);
    titleEl.appendChild(text);
  }

  private formatBandwidth(bytesPerSec: number): string {
    // Convert bytes/sec to bits/sec (multiply by 8)
    const bitsPerSec = bytesPerSec * 8;

    // Common modem speeds in bits/sec
    if (bitsPerSec <= 14400) return '14.4k';
    if (bitsPerSec <= 28800) return '28.8k';
    if (bitsPerSec <= 33600) return '33.6k';
    if (bitsPerSec <= 56000) return '56k';
    if (bitsPerSec <= 128000) return '128k';
    if (bitsPerSec <= 256000) return '256k';
    if (bitsPerSec <= 512000) return '512k';
    if (bitsPerSec <= 1024000) return '1Mbps';
    if (bitsPerSec <= 10240000) return `${(bitsPerSec / 1000000).toFixed(1)}Mbps`;
    return `${(bitsPerSec / 1000000).toFixed(1)}Mbps`;
  }

  private updateTrafficStats() {
    // Update the traffic display in the header
    const trafficElement = document.getElementById('traffic-stats');
    if (trafficElement && this.ws && this.ws.readyState === WebSocket.OPEN) {
      const sent = this.formatBytes(this.bytesSent);

      // Show throttled receive count if throttling is enabled, otherwise show actual
      const recvBytes = this.throttleBytesPerSecond > 0 ? this.bytesReceivedThrottled : this.bytesReceived;
      const recv = this.formatBytes(recvBytes);

      let html = `<span style="color: #9ca3af;">↑${sent} ↓${recv}</span>`;

      // Add throttle indicator and buffer status if enabled
      if (this.throttleBytesPerSecond > 0) {
        const speed = this.formatBandwidth(this.throttleBytesPerSecond);
        html += ` <span style="color: #9ca3af;">⏱ ${speed}</span>`;

        // Show buffered data if any
        if (this.frameReceiveBuffer.length > 0) {
          const bufferedBytes = this.frameReceiveBuffer.reduce((sum, item) => sum + item.size, 0);
          const buffered = this.formatBytes(bufferedBytes);
          html += ` <span style="color: #f59e0b;">(${buffered} buffered)</span>`;
        }
      }

      trafficElement.innerHTML = html;
    }
  }
}

// Initialize client when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => new SuperChatClient());
} else {
  new SuperChatClient();
}
