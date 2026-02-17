// X25519 key agreement + AES-256-GCM encryption for SuperChat DMs
// Byte-compatible with pkg/client/crypto/crypto.go

import { x25519 } from '@noble/curves/ed25519.js'
import { hkdf } from '@noble/hashes/hkdf.js'
import { sha512 } from '@noble/hashes/sha2.js'

const X25519_KEY_SIZE = 32
const AES_KEY_SIZE = 32
const NONCE_SIZE = 12
const TAG_SIZE = 16
const HKDF_SALT = new TextEncoder().encode('superchat-dm-v1')

// Key type constants for PROVIDE_PUBLIC_KEY (matches Go)
export const KEY_TYPE_DERIVED_FROM_SSH = 0x00
export const KEY_TYPE_GENERATED = 0x01
export const KEY_TYPE_EPHEMERAL = 0x02

export interface X25519KeyPair {
  publicKey: Uint8Array  // 32 bytes
  privateKey: Uint8Array // 32 bytes
}

// 7 known Curve25519 low-order points (copied exactly from Go)
const LOW_ORDER_POINTS: Uint8Array[] = [
  // Point at infinity (all zeros)
  new Uint8Array(32),
  // Order 2 point
  new Uint8Array([1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),
  // Order 4 points
  new Uint8Array([0xe0, 0xeb, 0x7a, 0x7c, 0x3b, 0x41, 0xb8, 0xae, 0x16, 0x56, 0xe3, 0xfa, 0xf1, 0x9f, 0xc4, 0x6a, 0xda, 0x09, 0x8d, 0xeb, 0x9c, 0x32, 0xb1, 0xfd, 0x86, 0x62, 0x05, 0x16, 0x5f, 0x49, 0xb8, 0x00]),
  new Uint8Array([0x5f, 0x9c, 0x95, 0xbc, 0xa3, 0x50, 0x8c, 0x24, 0xb1, 0xd0, 0xb1, 0x55, 0x9c, 0x83, 0xef, 0x5b, 0x04, 0x44, 0x5c, 0xc4, 0x58, 0x1c, 0x8e, 0x86, 0xd8, 0x22, 0x4e, 0xdd, 0xd0, 0x9f, 0x11, 0x57]),
  // Order 8 points
  new Uint8Array([0xec, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f]),
  new Uint8Array([0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f]),
  new Uint8Array([0xee, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f]),
]

function constantTimeEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false
  let diff = 0
  for (let i = 0; i < a.length; i++) {
    diff |= a[i] ^ b[i]
  }
  return diff === 0
}

export function isLowOrderPoint(publicKey: Uint8Array): boolean {
  if (publicKey.length !== 32) return true
  for (const point of LOW_ORDER_POINTS) {
    if (constantTimeEqual(publicKey, point)) return true
  }
  return false
}

/**
 * Generate a new X25519 key pair.
 * Private key is random + clamped, public key derived via scalarMultBase.
 */
export function generateX25519KeyPair(): X25519KeyPair {
  const privateKey = new Uint8Array(X25519_KEY_SIZE)
  crypto.getRandomValues(privateKey)

  // Standard X25519 clamping (matches Go)
  privateKey[0] &= 248
  privateKey[31] &= 127
  privateKey[31] |= 64

  const publicKey = x25519.scalarMultBase(privateKey)

  return { publicKey, privateKey }
}

/**
 * Compute X25519 shared secret. Validates against low-order points.
 */
export function computeSharedSecret(myPrivateKey: Uint8Array, theirPublicKey: Uint8Array): Uint8Array {
  if (myPrivateKey.length !== X25519_KEY_SIZE) {
    throw new Error(`Invalid private key size: ${myPrivateKey.length}`)
  }
  if (theirPublicKey.length !== X25519_KEY_SIZE) {
    throw new Error(`Invalid public key size: ${theirPublicKey.length}`)
  }
  if (isLowOrderPoint(theirPublicKey)) {
    throw new Error('Invalid public key: low-order point')
  }

  return x25519.scalarMult(myPrivateKey, theirPublicKey)
}

/**
 * Derive a unique AES-256 key for a DM channel using HKDF-SHA512.
 * Salt: "superchat-dm-v1", info: channelId as 8-byte big-endian.
 */
export function deriveChannelKey(sharedSecret: Uint8Array, channelId: bigint): Uint8Array {
  if (sharedSecret.length !== X25519_KEY_SIZE) {
    throw new Error(`Invalid shared secret size: ${sharedSecret.length}`)
  }

  // Convert channelId to 8-byte big-endian (matches Go's binary.BigEndian.PutUint64)
  const info = new Uint8Array(8)
  const view = new DataView(info.buffer)
  view.setBigUint64(0, channelId, false) // false = big-endian

  return hkdf(sha512, sharedSecret, HKDF_SALT, info, AES_KEY_SIZE)
}

/**
 * Encrypt plaintext with AES-256-GCM.
 * Returns: nonce(12) || ciphertext || tag(16)
 * Uses Web Crypto API for AES-GCM.
 */
export async function encryptMessage(key: Uint8Array, plaintext: Uint8Array): Promise<Uint8Array> {
  if (key.length !== AES_KEY_SIZE) {
    throw new Error(`Invalid key size: ${key.length}`)
  }

  const nonce = new Uint8Array(NONCE_SIZE)
  crypto.getRandomValues(nonce)

  const cryptoKey = await crypto.subtle.importKey(
    'raw', key, { name: 'AES-GCM' }, false, ['encrypt']
  )

  // Web Crypto AES-GCM appends the tag to the ciphertext
  const encrypted = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: nonce, tagLength: TAG_SIZE * 8 },
    cryptoKey,
    plaintext
  )

  // Output format: nonce || ciphertext || tag (matches Go's gcm.Seal(nonce, nonce, plaintext, nil))
  const result = new Uint8Array(NONCE_SIZE + encrypted.byteLength)
  result.set(nonce, 0)
  result.set(new Uint8Array(encrypted), NONCE_SIZE)
  return result
}

/**
 * Decrypt ciphertext encrypted with encryptMessage.
 * Expects: nonce(12) || ciphertext || tag(16)
 */
export async function decryptMessage(key: Uint8Array, ciphertext: Uint8Array): Promise<Uint8Array> {
  if (key.length !== AES_KEY_SIZE) {
    throw new Error(`Invalid key size: ${key.length}`)
  }
  if (ciphertext.length < NONCE_SIZE + TAG_SIZE) {
    throw new Error('Ciphertext too short')
  }

  const nonce = ciphertext.slice(0, NONCE_SIZE)
  const encrypted = ciphertext.slice(NONCE_SIZE)

  const cryptoKey = await crypto.subtle.importKey(
    'raw', key, { name: 'AES-GCM' }, false, ['decrypt']
  )

  const plaintext = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: nonce, tagLength: TAG_SIZE * 8 },
    cryptoKey,
    encrypted
  )

  return new Uint8Array(plaintext)
}
