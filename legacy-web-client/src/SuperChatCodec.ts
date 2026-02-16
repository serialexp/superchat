import { BitStreamEncoder, BitStreamDecoder, Endianness } from "./BitStream.js";

/**
 * Length-prefixed UTF-8 string
 */
export type String = string;

export class StringEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: String): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    const value_bytes = new TextEncoder().encode(value);
    this.writeUint16(value_bytes.length, "big_endian");
    for (const byte of value_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class StringDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[]) {
    super(bytes, "msb_first");
  }

  decode(): String {
    let value: any = {};
    const result_length = this.readUint16("big_endian");
    const result_bytes: number[] = [];
    for (let i = 0; i < result_length; i++) {
      result_bytes.push(this.readUint8());
    }
    value.result = new TextDecoder().decode(new Uint8Array(result_bytes));
    return value.result;
  }
}

/**
 * All messages use this frame format
 */
export interface FrameHeader {
  length: number;
  version: number;
  type: number;
  flags: number;
}

export class FrameHeaderEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: FrameHeader): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint32(value.length, "big_endian");
    this.writeUint8(value.version);
    this.writeUint8(value.type);
    this.writeUint8(value.flags);
    return this.finish();
  }
}

export class FrameHeaderDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): FrameHeader {
    const value: any = {};

    value.length = this.readUint32("big_endian");
    value.version = this.readUint8();
    value.type = this.readUint8();
    value.flags = this.readUint8();
    return value;
  }
}

/**
 * Authentication request with password
 */
export interface AuthRequest {
  nickname: String;
  password: String;
}

export class AuthRequestEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: AuthRequest): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    const value_nickname_bytes = new TextEncoder().encode(value.nickname);
    this.writeUint16(value_nickname_bytes.length, "big_endian");
    for (const byte of value_nickname_bytes) {
      this.writeUint8(byte);
    }
    const value_password_bytes = new TextEncoder().encode(value.password);
    this.writeUint16(value_password_bytes.length, "big_endian");
    for (const byte of value_password_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class AuthRequestDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): AuthRequest {
    const value: any = {};

    const nickname_length = this.readUint16("big_endian");
    const nickname_bytes: number[] = [];
    for (let i = 0; i < nickname_length; i++) {
      nickname_bytes.push(this.readUint8());
    }
    value.nickname = new TextDecoder().decode(new Uint8Array(nickname_bytes));
    const password_length = this.readUint16("big_endian");
    const password_bytes: number[] = [];
    for (let i = 0; i < password_length; i++) {
      password_bytes.push(this.readUint8());
    }
    value.password = new TextDecoder().decode(new Uint8Array(password_bytes));
    return value;
  }
}

/**
 * Authentication response
 */
export interface AuthResponse {
  success: number;
  user_id: { present: number, value?: bigint };
  nickname: { present: number, value?: String };
  message: String;
}

export class AuthResponseEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: AuthResponse): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.success);
    this.writeUint8(value.user_id.present);
    if (value.user_id.present == 1 && value.user_id.value !== undefined) {
      this.writeUint64(value.user_id.value, "big_endian");
    }
    this.writeUint8(value.nickname.present);
    if (value.nickname.present == 1 && value.nickname.value !== undefined) {
      const value_nickname_value_bytes = new TextEncoder().encode(value.nickname.value);
      this.writeUint16(value_nickname_value_bytes.length, "big_endian");
      for (const byte of value_nickname_value_bytes) {
        this.writeUint8(byte);
      }
    }
    const value_message_bytes = new TextEncoder().encode(value.message);
    this.writeUint16(value_message_bytes.length, "big_endian");
    for (const byte of value_message_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class AuthResponseDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): AuthResponse {
    const value: any = {};

    value.success = this.readUint8();
    value.user_id = {};
    value.user_id.present = this.readUint8();
    if (value.user_id.present == 1) {
      value.user_id.value = this.readUint64("big_endian");
    }
    value.nickname = {};
    value.nickname.present = this.readUint8();
    if (value.nickname.present == 1) {
      const nickname_value_length = this.readUint16("big_endian");
      const nickname_value_bytes: number[] = [];
      for (let i = 0; i < nickname_value_length; i++) {
        nickname_value_bytes.push(this.readUint8());
      }
      value.nickname.value = new TextDecoder().decode(new Uint8Array(nickname_value_bytes));
    }
    const message_length = this.readUint16("big_endian");
    const message_bytes: number[] = [];
    for (let i = 0; i < message_length; i++) {
      message_bytes.push(this.readUint8());
    }
    value.message = new TextDecoder().decode(new Uint8Array(message_bytes));
    return value;
  }
}

/**
 * Set or change nickname
 */
export interface SetNickname {
  nickname: String;
}

export class SetNicknameEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: SetNickname): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    const value_nickname_bytes = new TextEncoder().encode(value.nickname);
    this.writeUint16(value_nickname_bytes.length, "big_endian");
    for (const byte of value_nickname_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class SetNicknameDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): SetNickname {
    const value: any = {};

    const nickname_length = this.readUint16("big_endian");
    const nickname_bytes: number[] = [];
    for (let i = 0; i < nickname_length; i++) {
      nickname_bytes.push(this.readUint8());
    }
    value.nickname = new TextDecoder().decode(new Uint8Array(nickname_bytes));
    return value;
  }
}

/**
 * Nickname change result
 */
export interface NicknameResponse {
  success: number;
  message: String;
}

export class NicknameResponseEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: NicknameResponse): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.success);
    const value_message_bytes = new TextEncoder().encode(value.message);
    this.writeUint16(value_message_bytes.length, "big_endian");
    for (const byte of value_message_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class NicknameResponseDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): NicknameResponse {
    const value: any = {};

    value.success = this.readUint8();
    const message_length = this.readUint16("big_endian");
    const message_bytes: number[] = [];
    for (let i = 0; i < message_length; i++) {
      message_bytes.push(this.readUint8());
    }
    value.message = new TextDecoder().decode(new Uint8Array(message_bytes));
    return value;
  }
}

/**
 * Post a new message
 */
export interface PostMessage {
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
  parent_id: { present: number, value?: bigint };
  content: String;
}

export class PostMessageEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: PostMessage): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    this.writeUint8(value.parent_id.present);
    if (value.parent_id.present == 1 && value.parent_id.value !== undefined) {
      this.writeUint64(value.parent_id.value, "big_endian");
    }
    const value_content_bytes = new TextEncoder().encode(value.content);
    this.writeUint16(value_content_bytes.length, "big_endian");
    for (const byte of value_content_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class PostMessageDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): PostMessage {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    value.parent_id = {};
    value.parent_id.present = this.readUint8();
    if (value.parent_id.present == 1) {
      value.parent_id.value = this.readUint64("big_endian");
    }
    const content_length = this.readUint16("big_endian");
    const content_bytes: number[] = [];
    for (let i = 0; i < content_length; i++) {
      content_bytes.push(this.readUint8());
    }
    value.content = new TextDecoder().decode(new Uint8Array(content_bytes));
    return value;
  }
}

/**
 * Message post confirmation
 */
export interface MessagePosted {
  success: number;
  message_id: bigint;
  message: String;
}

export class MessagePostedEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: MessagePosted): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.success);
    this.writeUint64(value.message_id, "big_endian");
    const value_message_bytes = new TextEncoder().encode(value.message);
    this.writeUint16(value_message_bytes.length, "big_endian");
    for (const byte of value_message_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class MessagePostedDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): MessagePosted {
    const value: any = {};

    value.success = this.readUint8();
    value.message_id = this.readUint64("big_endian");
    const message_length = this.readUint16("big_endian");
    const message_bytes: number[] = [];
    for (let i = 0; i < message_length; i++) {
      message_bytes.push(this.readUint8());
    }
    value.message = new TextDecoder().decode(new Uint8Array(message_bytes));
    return value;
  }
}

/**
 * Real-time message notification
 */
export interface NewMessage {
  message_id: bigint;
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
  parent_id: { present: number, value?: bigint };
  author_user_id: { present: number, value?: bigint };
  author_nickname: String;
  content: String;
  created_at: bigint;
  edited_at: { present: number, value?: bigint };
  reply_count: number;
}

export class NewMessageEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: NewMessage): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.message_id, "big_endian");
    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    this.writeUint8(value.parent_id.present);
    if (value.parent_id.present == 1 && value.parent_id.value !== undefined) {
      this.writeUint64(value.parent_id.value, "big_endian");
    }
    this.writeUint8(value.author_user_id.present);
    if (value.author_user_id.present == 1 && value.author_user_id.value !== undefined) {
      this.writeUint64(value.author_user_id.value, "big_endian");
    }
    const value_author_nickname_bytes = new TextEncoder().encode(value.author_nickname);
    this.writeUint16(value_author_nickname_bytes.length, "big_endian");
    for (const byte of value_author_nickname_bytes) {
      this.writeUint8(byte);
    }
    const value_content_bytes = new TextEncoder().encode(value.content);
    this.writeUint16(value_content_bytes.length, "big_endian");
    for (const byte of value_content_bytes) {
      this.writeUint8(byte);
    }
    this.writeInt64(value.created_at, "big_endian");
    this.writeUint8(value.edited_at.present);
    if (value.edited_at.present == 1 && value.edited_at.value !== undefined) {
      this.writeInt64(value.edited_at.value, "big_endian");
    }
    this.writeUint32(value.reply_count, "big_endian");
    return this.finish();
  }
}

export class NewMessageDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): NewMessage {
    const value: any = {};

    value.message_id = this.readUint64("big_endian");
    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    value.parent_id = {};
    value.parent_id.present = this.readUint8();
    if (value.parent_id.present == 1) {
      value.parent_id.value = this.readUint64("big_endian");
    }
    value.author_user_id = {};
    value.author_user_id.present = this.readUint8();
    if (value.author_user_id.present == 1) {
      value.author_user_id.value = this.readUint64("big_endian");
    }
    const author_nickname_length = this.readUint16("big_endian");
    const author_nickname_bytes: number[] = [];
    for (let i = 0; i < author_nickname_length; i++) {
      author_nickname_bytes.push(this.readUint8());
    }
    value.author_nickname = new TextDecoder().decode(new Uint8Array(author_nickname_bytes));
    const content_length = this.readUint16("big_endian");
    const content_bytes: number[] = [];
    for (let i = 0; i < content_length; i++) {
      content_bytes.push(this.readUint8());
    }
    value.content = new TextDecoder().decode(new Uint8Array(content_bytes));
    value.created_at = this.readInt64("big_endian");
    value.edited_at = {};
    value.edited_at.present = this.readUint8();
    if (value.edited_at.present == 1) {
      value.edited_at.value = this.readInt64("big_endian");
    }
    value.reply_count = this.readUint32("big_endian");
    return value;
  }
}

/**
 * Register current nickname with password
 */
export interface RegisterUser {
  password_hash: String;
}

export class RegisterUserEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: RegisterUser): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    const value_password_hash_bytes = new TextEncoder().encode(value.password_hash);
    this.writeUint16(value_password_hash_bytes.length, "big_endian");
    for (const byte of value_password_hash_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class RegisterUserDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): RegisterUser {
    const value: any = {};

    const password_hash_length = this.readUint16("big_endian");
    const password_hash_bytes: number[] = [];
    for (let i = 0; i < password_hash_length; i++) {
      password_hash_bytes.push(this.readUint8());
    }
    value.password_hash = new TextDecoder().decode(new Uint8Array(password_hash_bytes));
    return value;
  }
}

/**
 * Registration result
 */
export interface RegisterResponse {
  success: number;
  user_id: { present: number, value?: bigint };
}

export class RegisterResponseEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: RegisterResponse): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.success);
    this.writeUint8(value.user_id.present);
    if (value.user_id.present == 1 && value.user_id.value !== undefined) {
      this.writeUint64(value.user_id.value, "big_endian");
    }
    return this.finish();
  }
}

export class RegisterResponseDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): RegisterResponse {
    const value: any = {};

    value.success = this.readUint8();
    value.user_id = {};
    value.user_id.present = this.readUint8();
    if (value.user_id.present == 1) {
      value.user_id.value = this.readUint64("big_endian");
    }
    return value;
  }
}

/**
 * Request channel list
 */
export interface ListChannels {
  from_channel_id: bigint;
  limit: number;
}

export class ListChannelsEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: ListChannels): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.from_channel_id, "big_endian");
    this.writeUint16(value.limit, "big_endian");
    return this.finish();
  }
}

export class ListChannelsDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): ListChannels {
    const value: any = {};

    value.from_channel_id = this.readUint64("big_endian");
    value.limit = this.readUint16("big_endian");
    return value;
  }
}

/**
 * Channel information in list
 */
export interface Channel {
  channel_id: bigint;
  name: String;
  description: String;
  user_count: number;
  is_operator: number;
  type: number;
  retention_hours: number;
}

export class ChannelEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: Channel): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    const value_name_bytes = new TextEncoder().encode(value.name);
    this.writeUint16(value_name_bytes.length, "big_endian");
    for (const byte of value_name_bytes) {
      this.writeUint8(byte);
    }
    const value_description_bytes = new TextEncoder().encode(value.description);
    this.writeUint16(value_description_bytes.length, "big_endian");
    for (const byte of value_description_bytes) {
      this.writeUint8(byte);
    }
    this.writeUint32(value.user_count, "big_endian");
    this.writeUint8(value.is_operator);
    this.writeUint8(value.type);
    this.writeUint32(value.retention_hours, "big_endian");
    return this.finish();
  }
}

export class ChannelDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): Channel {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    const name_length = this.readUint16("big_endian");
    const name_bytes: number[] = [];
    for (let i = 0; i < name_length; i++) {
      name_bytes.push(this.readUint8());
    }
    value.name = new TextDecoder().decode(new Uint8Array(name_bytes));
    const description_length = this.readUint16("big_endian");
    const description_bytes: number[] = [];
    for (let i = 0; i < description_length; i++) {
      description_bytes.push(this.readUint8());
    }
    value.description = new TextDecoder().decode(new Uint8Array(description_bytes));
    value.user_count = this.readUint32("big_endian");
    value.is_operator = this.readUint8();
    value.type = this.readUint8();
    value.retention_hours = this.readUint32("big_endian");
    return value;
  }
}

/**
 * List of channels
 */
export interface ChannelList {
  channel_count: number;
  channels: Channel[];
}

export class ChannelListEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: ChannelList): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint16(value.channel_count, "big_endian");
    for (const value_channels_item of value.channels) {
      this.writeUint64(value_channels_item.channel_id, "big_endian");
      const value_channels_item_name_bytes = new TextEncoder().encode(value_channels_item.name);
      this.writeUint16(value_channels_item_name_bytes.length, "big_endian");
      for (const byte of value_channels_item_name_bytes) {
        this.writeUint8(byte);
      }
      const value_channels_item_description_bytes = new TextEncoder().encode(value_channels_item.description);
      this.writeUint16(value_channels_item_description_bytes.length, "big_endian");
      for (const byte of value_channels_item_description_bytes) {
        this.writeUint8(byte);
      }
      this.writeUint32(value_channels_item.user_count, "big_endian");
      this.writeUint8(value_channels_item.is_operator);
      this.writeUint8(value_channels_item.type);
      this.writeUint32(value_channels_item.retention_hours, "big_endian");
    }
    return this.finish();
  }
}

export class ChannelListDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): ChannelList {
    const value: any = {};

    value.channel_count = this.readUint16("big_endian");
    value.channels = [];
    const channels_length = value.channel_count ?? this.context?.channel_count;
    if (channels_length === undefined) {
      throw new Error('Field-referenced array length field "channel_count" not found in value or context');
    }
    for (let i = 0; i < channels_length; i++) {
      let channels_item: any;
      channels_item = {};
      channels_item.channel_id = this.readUint64("big_endian");
      const channels_item_name_length = this.readUint16("big_endian");
      const channels_item_name_bytes: number[] = [];
      for (let i = 0; i < channels_item_name_length; i++) {
        channels_item_name_bytes.push(this.readUint8());
      }
      channels_item.name = new TextDecoder().decode(new Uint8Array(channels_item_name_bytes));
      const channels_item_description_length = this.readUint16("big_endian");
      const channels_item_description_bytes: number[] = [];
      for (let i = 0; i < channels_item_description_length; i++) {
        channels_item_description_bytes.push(this.readUint8());
      }
      channels_item.description = new TextDecoder().decode(new Uint8Array(channels_item_description_bytes));
      channels_item.user_count = this.readUint32("big_endian");
      channels_item.is_operator = this.readUint8();
      channels_item.type = this.readUint8();
      channels_item.retention_hours = this.readUint32("big_endian");
      value.channels.push(channels_item);
    }
    return value;
  }
}

/**
 * Join a channel
 */
export interface JoinChannel {
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
}

export class JoinChannelEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: JoinChannel): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    return this.finish();
  }
}

export class JoinChannelDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): JoinChannel {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    return value;
  }
}

/**
 * Join result
 */
export interface JoinResponse {
  success: number;
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
  message: String;
}

export class JoinResponseEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: JoinResponse): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.success);
    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    const value_message_bytes = new TextEncoder().encode(value.message);
    this.writeUint16(value_message_bytes.length, "big_endian");
    for (const byte of value_message_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class JoinResponseDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): JoinResponse {
    const value: any = {};

    value.success = this.readUint8();
    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    const message_length = this.readUint16("big_endian");
    const message_bytes: number[] = [];
    for (let i = 0; i < message_length; i++) {
      message_bytes.push(this.readUint8());
    }
    value.message = new TextDecoder().decode(new Uint8Array(message_bytes));
    return value;
  }
}

/**
 * Request messages from channel
 */
export interface ListMessages {
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
  limit: number;
  before_id: { present: number, value?: bigint };
  parent_id: { present: number, value?: bigint };
  after_id: { present: number, value?: bigint };
}

export class ListMessagesEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: ListMessages): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    this.writeUint16(value.limit, "big_endian");
    this.writeUint8(value.before_id.present);
    if (value.before_id.present == 1 && value.before_id.value !== undefined) {
      this.writeUint64(value.before_id.value, "big_endian");
    }
    this.writeUint8(value.parent_id.present);
    if (value.parent_id.present == 1 && value.parent_id.value !== undefined) {
      this.writeUint64(value.parent_id.value, "big_endian");
    }
    this.writeUint8(value.after_id.present);
    if (value.after_id.present == 1 && value.after_id.value !== undefined) {
      this.writeUint64(value.after_id.value, "big_endian");
    }
    return this.finish();
  }
}

export class ListMessagesDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): ListMessages {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    value.limit = this.readUint16("big_endian");
    value.before_id = {};
    value.before_id.present = this.readUint8();
    if (value.before_id.present == 1) {
      value.before_id.value = this.readUint64("big_endian");
    }
    value.parent_id = {};
    value.parent_id.present = this.readUint8();
    if (value.parent_id.present == 1) {
      value.parent_id.value = this.readUint64("big_endian");
    }
    value.after_id = {};
    value.after_id.present = this.readUint8();
    if (value.after_id.present == 1) {
      value.after_id.value = this.readUint64("big_endian");
    }
    return value;
  }
}

/**
 * Message in list
 */
export interface Message {
  message_id: bigint;
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
  parent_id: { present: number, value?: bigint };
  author_user_id: { present: number, value?: bigint };
  author_nickname: String;
  content: String;
  created_at: bigint;
  edited_at: { present: number, value?: bigint };
  reply_count: number;
}

export class MessageEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: Message): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.message_id, "big_endian");
    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    this.writeUint8(value.parent_id.present);
    if (value.parent_id.present == 1 && value.parent_id.value !== undefined) {
      this.writeUint64(value.parent_id.value, "big_endian");
    }
    this.writeUint8(value.author_user_id.present);
    if (value.author_user_id.present == 1 && value.author_user_id.value !== undefined) {
      this.writeUint64(value.author_user_id.value, "big_endian");
    }
    const value_author_nickname_bytes = new TextEncoder().encode(value.author_nickname);
    this.writeUint16(value_author_nickname_bytes.length, "big_endian");
    for (const byte of value_author_nickname_bytes) {
      this.writeUint8(byte);
    }
    const value_content_bytes = new TextEncoder().encode(value.content);
    this.writeUint16(value_content_bytes.length, "big_endian");
    for (const byte of value_content_bytes) {
      this.writeUint8(byte);
    }
    this.writeInt64(value.created_at, "big_endian");
    this.writeUint8(value.edited_at.present);
    if (value.edited_at.present == 1 && value.edited_at.value !== undefined) {
      this.writeInt64(value.edited_at.value, "big_endian");
    }
    this.writeUint32(value.reply_count, "big_endian");
    return this.finish();
  }
}

export class MessageDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): Message {
    const value: any = {};

    value.message_id = this.readUint64("big_endian");
    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    value.parent_id = {};
    value.parent_id.present = this.readUint8();
    if (value.parent_id.present == 1) {
      value.parent_id.value = this.readUint64("big_endian");
    }
    value.author_user_id = {};
    value.author_user_id.present = this.readUint8();
    if (value.author_user_id.present == 1) {
      value.author_user_id.value = this.readUint64("big_endian");
    }
    const author_nickname_length = this.readUint16("big_endian");
    const author_nickname_bytes: number[] = [];
    for (let i = 0; i < author_nickname_length; i++) {
      author_nickname_bytes.push(this.readUint8());
    }
    value.author_nickname = new TextDecoder().decode(new Uint8Array(author_nickname_bytes));
    const content_length = this.readUint16("big_endian");
    const content_bytes: number[] = [];
    for (let i = 0; i < content_length; i++) {
      content_bytes.push(this.readUint8());
    }
    value.content = new TextDecoder().decode(new Uint8Array(content_bytes));
    value.created_at = this.readInt64("big_endian");
    value.edited_at = {};
    value.edited_at.present = this.readUint8();
    if (value.edited_at.present == 1) {
      value.edited_at.value = this.readInt64("big_endian");
    }
    value.reply_count = this.readUint32("big_endian");
    return value;
  }
}

/**
 * List of messages
 */
export interface MessageList {
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
  parent_id: { present: number, value?: bigint };
  message_count: number;
  messages: Message[];
}

export class MessageListEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: MessageList): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    this.writeUint8(value.parent_id.present);
    if (value.parent_id.present == 1 && value.parent_id.value !== undefined) {
      this.writeUint64(value.parent_id.value, "big_endian");
    }
    this.writeUint16(value.message_count, "big_endian");
    for (const value_messages_item of value.messages) {
      this.writeUint64(value_messages_item.message_id, "big_endian");
      this.writeUint64(value_messages_item.channel_id, "big_endian");
      this.writeUint8(value_messages_item.subchannel_id.present);
      if (value_messages_item.subchannel_id.present == 1 && value_messages_item.subchannel_id.value !== undefined) {
        this.writeUint64(value_messages_item.subchannel_id.value, "big_endian");
      }
      this.writeUint8(value_messages_item.parent_id.present);
      if (value_messages_item.parent_id.present == 1 && value_messages_item.parent_id.value !== undefined) {
        this.writeUint64(value_messages_item.parent_id.value, "big_endian");
      }
      this.writeUint8(value_messages_item.author_user_id.present);
      if (value_messages_item.author_user_id.present == 1 && value_messages_item.author_user_id.value !== undefined) {
        this.writeUint64(value_messages_item.author_user_id.value, "big_endian");
      }
      const value_messages_item_author_nickname_bytes = new TextEncoder().encode(value_messages_item.author_nickname);
      this.writeUint16(value_messages_item_author_nickname_bytes.length, "big_endian");
      for (const byte of value_messages_item_author_nickname_bytes) {
        this.writeUint8(byte);
      }
      const value_messages_item_content_bytes = new TextEncoder().encode(value_messages_item.content);
      this.writeUint16(value_messages_item_content_bytes.length, "big_endian");
      for (const byte of value_messages_item_content_bytes) {
        this.writeUint8(byte);
      }
      this.writeInt64(value_messages_item.created_at, "big_endian");
      this.writeUint8(value_messages_item.edited_at.present);
      if (value_messages_item.edited_at.present == 1 && value_messages_item.edited_at.value !== undefined) {
        this.writeInt64(value_messages_item.edited_at.value, "big_endian");
      }
      this.writeUint32(value_messages_item.reply_count, "big_endian");
    }
    return this.finish();
  }
}

export class MessageListDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): MessageList {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    value.parent_id = {};
    value.parent_id.present = this.readUint8();
    if (value.parent_id.present == 1) {
      value.parent_id.value = this.readUint64("big_endian");
    }
    value.message_count = this.readUint16("big_endian");
    value.messages = [];
    const messages_length = value.message_count ?? this.context?.message_count;
    if (messages_length === undefined) {
      throw new Error('Field-referenced array length field "message_count" not found in value or context');
    }
    for (let i = 0; i < messages_length; i++) {
      let messages_item: any;
      messages_item = {};
      messages_item.message_id = this.readUint64("big_endian");
      messages_item.channel_id = this.readUint64("big_endian");
      messages_item.subchannel_id = {};
      messages_item.subchannel_id.present = this.readUint8();
      if (messages_item.subchannel_id.present == 1) {
        messages_item.subchannel_id.value = this.readUint64("big_endian");
      }
      messages_item.parent_id = {};
      messages_item.parent_id.present = this.readUint8();
      if (messages_item.parent_id.present == 1) {
        messages_item.parent_id.value = this.readUint64("big_endian");
      }
      messages_item.author_user_id = {};
      messages_item.author_user_id.present = this.readUint8();
      if (messages_item.author_user_id.present == 1) {
        messages_item.author_user_id.value = this.readUint64("big_endian");
      }
      const messages_item_author_nickname_length = this.readUint16("big_endian");
      const messages_item_author_nickname_bytes: number[] = [];
      for (let i = 0; i < messages_item_author_nickname_length; i++) {
        messages_item_author_nickname_bytes.push(this.readUint8());
      }
      messages_item.author_nickname = new TextDecoder().decode(new Uint8Array(messages_item_author_nickname_bytes));
      const messages_item_content_length = this.readUint16("big_endian");
      const messages_item_content_bytes: number[] = [];
      for (let i = 0; i < messages_item_content_length; i++) {
        messages_item_content_bytes.push(this.readUint8());
      }
      messages_item.content = new TextDecoder().decode(new Uint8Array(messages_item_content_bytes));
      messages_item.created_at = this.readInt64("big_endian");
      messages_item.edited_at = {};
      messages_item.edited_at.present = this.readUint8();
      if (messages_item.edited_at.present == 1) {
        messages_item.edited_at.value = this.readInt64("big_endian");
      }
      messages_item.reply_count = this.readUint32("big_endian");
      value.messages.push(messages_item);
    }
    return value;
  }
}

/**
 * Keepalive heartbeat
 */
export interface Ping {
  timestamp: bigint;
}

export class PingEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: Ping): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeInt64(value.timestamp, "big_endian");
    return this.finish();
  }
}

export class PingDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): Ping {
    const value: any = {};

    value.timestamp = this.readInt64("big_endian");
    return value;
  }
}

/**
 * Ping response
 */
export interface Pong {
  client_timestamp: bigint;
}

export class PongEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: Pong): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeInt64(value.client_timestamp, "big_endian");
    return this.finish();
  }
}

export class PongDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): Pong {
    const value: any = {};

    value.client_timestamp = this.readInt64("big_endian");
    return value;
  }
}

/**
 * Subscribe to thread updates
 */
export interface SubscribeThread {
  thread_id: bigint;
}

export class SubscribeThreadEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: SubscribeThread): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.thread_id, "big_endian");
    return this.finish();
  }
}

export class SubscribeThreadDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): SubscribeThread {
    const value: any = {};

    value.thread_id = this.readUint64("big_endian");
    return value;
  }
}

/**
 * Unsubscribe from thread updates
 */
export interface UnsubscribeThread {
  thread_id: bigint;
}

export class UnsubscribeThreadEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: UnsubscribeThread): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.thread_id, "big_endian");
    return this.finish();
  }
}

export class UnsubscribeThreadDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): UnsubscribeThread {
    const value: any = {};

    value.thread_id = this.readUint64("big_endian");
    return value;
  }
}

/**
 * Subscribe to new threads in channel
 */
export interface SubscribeChannel {
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
}

export class SubscribeChannelEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: SubscribeChannel): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    return this.finish();
  }
}

export class SubscribeChannelDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): SubscribeChannel {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    return value;
  }
}

/**
 * Unsubscribe from channel updates
 */
export interface UnsubscribeChannel {
  channel_id: bigint;
  subchannel_id: { present: number, value?: bigint };
}

export class UnsubscribeChannelEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: UnsubscribeChannel): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint64(value.channel_id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    return this.finish();
  }
}

export class UnsubscribeChannelDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): UnsubscribeChannel {
    const value: any = {};

    value.channel_id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    return value;
  }
}

/**
 * Subscription confirmation
 */
export interface SubscribeOk {
  type: number;
  id: bigint;
  subchannel_id: { present: number, value?: bigint };
}

export class SubscribeOkEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: SubscribeOk): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.type);
    this.writeUint64(value.id, "big_endian");
    this.writeUint8(value.subchannel_id.present);
    if (value.subchannel_id.present == 1 && value.subchannel_id.value !== undefined) {
      this.writeUint64(value.subchannel_id.value, "big_endian");
    }
    return this.finish();
  }
}

export class SubscribeOkDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): SubscribeOk {
    const value: any = {};

    value.type = this.readUint8();
    value.id = this.readUint64("big_endian");
    value.subchannel_id = {};
    value.subchannel_id.present = this.readUint8();
    if (value.subchannel_id.present == 1) {
      value.subchannel_id.value = this.readUint64("big_endian");
    }
    return value;
  }
}

/**
 * Generic error response
 */
export interface Error_ {
  error_code: number;
  message: String;
}

export class Error_Encoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: Error_): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint16(value.error_code, "big_endian");
    const value_message_bytes = new TextEncoder().encode(value.message);
    this.writeUint16(value_message_bytes.length, "big_endian");
    for (const byte of value_message_bytes) {
      this.writeUint8(byte);
    }
    return this.finish();
  }
}

export class Error_Decoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): Error_ {
    const value: any = {};

    value.error_code = this.readUint16("big_endian");
    const message_length = this.readUint16("big_endian");
    const message_bytes: number[] = [];
    for (let i = 0; i < message_length; i++) {
      message_bytes.push(this.readUint8());
    }
    value.message = new TextDecoder().decode(new Uint8Array(message_bytes));
    return value;
  }
}

/**
 * Server configuration and limits
 */
export interface ServerConfig {
  protocol_version: number;
  max_message_rate: number;
  max_channel_creates: number;
  inactive_cleanup_days: number;
  max_connections_per_ip: number;
  max_message_length: number;
  max_thread_subs: number;
  max_channel_subs: number;
  directory_enabled: number;
}

export class ServerConfigEncoder extends BitStreamEncoder {
  private compressionDict: Map<string, number> = new Map();

  constructor() {
    super("msb_first");
  }

  encode(value: ServerConfig): Uint8Array {
    // Reset compression dictionary for each encode
    this.compressionDict.clear();

    this.writeUint8(value.protocol_version);
    this.writeUint16(value.max_message_rate, "big_endian");
    this.writeUint16(value.max_channel_creates, "big_endian");
    this.writeUint16(value.inactive_cleanup_days, "big_endian");
    this.writeUint8(value.max_connections_per_ip);
    this.writeUint32(value.max_message_length, "big_endian");
    this.writeUint16(value.max_thread_subs, "big_endian");
    this.writeUint16(value.max_channel_subs, "big_endian");
    this.writeUint8(value.directory_enabled);
    return this.finish();
  }
}

export class ServerConfigDecoder extends BitStreamDecoder {
  constructor(bytes: Uint8Array | number[], private context?: any) {
    super(bytes, "msb_first");
  }

  decode(): ServerConfig {
    const value: any = {};

    value.protocol_version = this.readUint8();
    value.max_message_rate = this.readUint16("big_endian");
    value.max_channel_creates = this.readUint16("big_endian");
    value.inactive_cleanup_days = this.readUint16("big_endian");
    value.max_connections_per_ip = this.readUint8();
    value.max_message_length = this.readUint32("big_endian");
    value.max_thread_subs = this.readUint16("big_endian");
    value.max_channel_subs = this.readUint16("big_endian");
    value.directory_enabled = this.readUint8();
    return value;
  }
}

