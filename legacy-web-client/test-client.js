#!/usr/bin/env node
// ABOUTME: Node.js test client for SuperChat protocol
// ABOUTME: Tests connection, nickname setup, and channel listing using the generated codec

import net from 'net';
import {
  FrameHeaderDecoder,
  SetNicknameEncoder,
  NicknameResponseDecoder,
  ListChannelsEncoder,
  ChannelListDecoder,
  ServerConfigDecoder,
  Error_Decoder
} from './dist/SuperChatCodec.js';

const SERVER_HOST = 'localhost';
const SERVER_PORT = 6465;

const MSG_SET_NICKNAME = 0x02;
const MSG_NICKNAME_RESPONSE = 0x82;
const MSG_LIST_CHANNELS = 0x04;
const MSG_CHANNEL_LIST = 0x84;
const MSG_SERVER_CONFIG = 0x98;
const MSG_ERROR = 0x91;

class TestClient {
  constructor() {
    this.socket = null;
    this.frameBuffer = Buffer.alloc(0);
    this.expectedFrameLength = null;
  }

  connect() {
    return new Promise((resolve, reject) => {
      console.log(`Connecting to ${SERVER_HOST}:${SERVER_PORT}...`);

      this.socket = net.createConnection(SERVER_PORT, SERVER_HOST, () => {
        console.log('Connected!');
        resolve();
      });

      this.socket.on('data', (data) => this.handleFragment(data));
      this.socket.on('error', (err) => {
        console.error('Socket error:', err);
        reject(err);
      });
      this.socket.on('close', () => {
        console.log('Connection closed');
      });
    });
  }

  handleFragment(fragment) {
    // Append to buffer
    this.frameBuffer = Buffer.concat([this.frameBuffer, fragment]);
    console.log(`Received ${fragment.length} bytes, buffer now ${this.frameBuffer.length} bytes`);

    // Try to extract complete frames
    while (true) {
      // Need at least 4 bytes for length prefix
      if (this.expectedFrameLength === null && this.frameBuffer.length >= 4) {
        this.expectedFrameLength = this.frameBuffer.readUInt32BE(0);
        console.log(`Expecting frame of ${this.expectedFrameLength} bytes (plus 4-byte length prefix)`);
      }

      // Check if we have complete frame
      if (this.expectedFrameLength !== null) {
        const totalFrameSize = 4 + this.expectedFrameLength;
        if (this.frameBuffer.length >= totalFrameSize) {
          // Extract complete frame
          const completeFrame = this.frameBuffer.subarray(0, totalFrameSize);

          // Remove from buffer
          this.frameBuffer = this.frameBuffer.subarray(totalFrameSize);
          this.expectedFrameLength = null;

          // Process frame
          this.handleMessage(completeFrame);

          // Continue checking for more frames
          continue;
        }
      }

      // Not enough data yet
      break;
    }
  }

  handleMessage(data) {
    try {
      console.log(`\n=== RECEIVED FRAME (${data.length} bytes) ===`);
      console.log('First 20 bytes:', Array.from(data.subarray(0, 20)));

      if (data.length < 7) {
        console.error('Frame too short!');
        return;
      }

      // Decode header
      const headerDecoder = new FrameHeaderDecoder(new Uint8Array(data));
      const header = headerDecoder.decode();
      console.log(`Header: length=${header.length}, version=${header.version}, type=0x${header.type.toString(16)}, flags=${header.flags}`);

      // Extract payload (skip 4-byte length + 3-byte header)
      const payloadBuffer = data.subarray(7);
      const payload = new Uint8Array(payloadBuffer);
      console.log(`Payload: ${payload.length} bytes, byteOffset=${payload.byteOffset}`);

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
        case MSG_ERROR:
          this.handleError(payload);
          break;
        default:
          console.warn(`Unhandled message type: 0x${header.type.toString(16)}`);
      }
    } catch (error) {
      console.error('Error handling message:', error);
    }
  }

  handleServerConfig(payload) {
    console.log('\n--- SERVER_CONFIG ---');
    const decoder = new ServerConfigDecoder(payload);
    const config = decoder.decode();
    console.log('Config:', config);
  }

  handleNicknameResponse(payload) {
    console.log('\n--- NICKNAME_RESPONSE ---');
    const decoder = new NicknameResponseDecoder(payload);
    const response = decoder.decode();
    console.log('Response:', response);
  }

  handleChannelList(payload) {
    console.log('\n--- CHANNEL_LIST ---');
    console.log('Payload type:', payload.constructor.name);
    console.log('Payload length:', payload.length);
    console.log('Payload byteOffset:', payload.byteOffset);
    console.log('Payload buffer.byteLength:', payload.buffer.byteLength);
    console.log('Payload instanceof Uint8Array:', payload instanceof Uint8Array);
    console.log('First 50 bytes:', Array.from(payload.subarray(0, 50)));

    // Test direct byte access
    console.log('\nDirect byte access test:');
    for (let i = 0; i < 20; i++) {
      console.log(`  payload[${i}] = ${payload[i]}`);
    }

    try {
      const decoder = new ChannelListDecoder(payload);
      console.log('\nDecoder created');
      console.log('Decoder.bytes type:', decoder.bytes?.constructor?.name);
      console.log('Decoder.bytes length:', decoder.bytes?.length);
      console.log('Decoder.bytes[0]:', decoder.bytes?.[0]);

      // Monkey-patch readUint8 to trace every byte read
      const originalReadUint8 = decoder.readUint8.bind(decoder);
      let readCount = 0;
      decoder.readUint8 = function() {
        const offset = this.byteOffset;
        const bytesLen = this.bytes.length;
        readCount++;
        if (readCount <= 65) {  // Log first 65 reads
          console.log(`  [read#${readCount}] byteOffset=${offset}, bytes.length=${bytesLen}`);
        }
        const result = originalReadUint8();
        if (readCount <= 65) {
          console.log(`    -> byte=${result}`);
        }
        return result;
      };

      console.log('\nCalling decode()...');
      const channelList = decoder.decode();
      console.log(`SUCCESS! Received ${channelList.channel_count} channels:`);
      for (const channel of channelList.channels) {
        console.log(`  - ${channel.name}: ${channel.description} (${channel.user_count} users)`);
      }
    } catch (error) {
      console.error('\nFAILED to decode CHANNEL_LIST:', error.message);
      console.error('Error stack:', error.stack);
    }
  }

  handleError(payload) {
    console.log('\n--- ERROR ---');
    const decoder = new Error_Decoder(payload);
    const error = decoder.decode();
    console.error(`Server error ${error.error_code}: ${error.message}`);
  }

  sendFrame(messageType, payload) {
    // Frame format: [Length(4)][Version(1)][Type(1)][Flags(1)][Payload]
    const length = 3 + payload.length; // version + type + flags + payload
    const frame = Buffer.alloc(4 + length);

    let offset = 0;
    frame.writeUInt32BE(length, offset); offset += 4;
    frame.writeUInt8(1, offset); offset += 1; // version
    frame.writeUInt8(messageType, offset); offset += 1;
    frame.writeUInt8(0, offset); offset += 1; // flags
    frame.set(payload, offset);

    console.log(`\nSending frame: type=0x${messageType.toString(16)}, payload=${payload.length} bytes`);
    this.socket.write(frame);
  }

  async run() {
    try {
      await this.connect();

      // Wait a bit for SERVER_CONFIG
      await new Promise(resolve => setTimeout(resolve, 100));

      // Send SET_NICKNAME
      console.log('\n=== SENDING SET_NICKNAME ===');
      const nicknameEncoder = new SetNicknameEncoder();
      const nicknamePayload = nicknameEncoder.encode({ nickname: 'testuser' });
      this.sendFrame(MSG_SET_NICKNAME, nicknamePayload);

      // Wait for response
      await new Promise(resolve => setTimeout(resolve, 100));

      // Send LIST_CHANNELS
      console.log('\n=== SENDING LIST_CHANNELS ===');
      const channelsEncoder = new ListChannelsEncoder();
      const channelsPayload = channelsEncoder.encode({ from_channel_id: 0n, limit: 100 });
      this.sendFrame(MSG_LIST_CHANNELS, channelsPayload);

      // Wait for response
      await new Promise(resolve => setTimeout(resolve, 500));

      // Close
      console.log('\n=== CLOSING ===');
      this.socket.end();

    } catch (error) {
      console.error('Test failed:', error);
      process.exit(1);
    }
  }
}

// Run the test
const client = new TestClient();
client.run();
