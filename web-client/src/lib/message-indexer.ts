// Message indexer for building efficient lookup structures
// Builds threadIndex (channel -> root messages) and replyIndex (parent -> children)

import type { Message } from '../SuperChatCodec'

/**
 * Build thread index: channelId -> array of root message IDs
 * Root messages have parent_id.present === 0
 */
export function buildThreadIndex(messages: Map<bigint, Message>): Map<bigint, bigint[]> {
  const index = new Map<bigint, bigint[]>()

  for (const message of messages.values()) {
    // Only index root messages (parent_id.present === 0)
    if (message.parent_id.present === 0) {
      const channelId = message.channel_id
      const existing = index.get(channelId) || []
      existing.push(message.message_id)
      index.set(channelId, existing)
    }
  }

  return index
}

/**
 * Build reply index: parentId -> array of child message IDs
 * Child messages have parent_id.present === 1
 */
export function buildReplyIndex(messages: Map<bigint, Message>): Map<bigint, bigint[]> {
  const index = new Map<bigint, bigint[]>()

  for (const message of messages.values()) {
    // Only index replies (parent_id.present === 1)
    if (message.parent_id.present === 1) {
      const parentId = message.parent_id.value
      const existing = index.get(parentId) || []
      existing.push(message.message_id)
      index.set(parentId, existing)
    }
  }

  return index
}

/**
 * Add a single message to the thread index
 * Returns updated index (creates new Map for immutability)
 */
export function addMessageToThreadIndex(
  index: Map<bigint, bigint[]>,
  message: Message
): Map<bigint, bigint[]> {
  // Only add root messages
  if (message.parent_id.present !== 0) return index

  const newIndex = new Map(index)
  const channelId = message.channel_id
  const existing = newIndex.get(channelId) || []

  // Check if message is already in index
  if (existing.includes(message.message_id)) {
    return index // No change
  }

  existing.push(message.message_id)
  newIndex.set(channelId, existing)
  return newIndex
}

/**
 * Add a single message to the reply index
 * Returns updated index (creates new Map for immutability)
 */
export function addMessageToReplyIndex(
  index: Map<bigint, bigint[]>,
  message: Message
): Map<bigint, bigint[]> {
  // Only add replies
  if (message.parent_id.present !== 1) return index

  const newIndex = new Map(index)
  const parentId = message.parent_id.value
  const existing = newIndex.get(parentId) || []

  // Check if message is already in index
  if (existing.includes(message.message_id)) {
    return index // No change
  }

  existing.push(message.message_id)
  newIndex.set(parentId, existing)
  return newIndex
}

/**
 * Rebuild both indexes from scratch
 * Call this when receiving MESSAGE_LIST or after clearing messages
 */
export function rebuildIndexes(messages: Map<bigint, Message>): {
  threadIndex: Map<bigint, bigint[]>
  replyIndex: Map<bigint, bigint[]>
} {
  return {
    threadIndex: buildThreadIndex(messages),
    replyIndex: buildReplyIndex(messages)
  }
}

/**
 * Incrementally add a message to indexes
 * Call this when receiving NEW_MESSAGE broadcasts
 */
export function addMessageToIndexes(
  threadIndex: Map<bigint, bigint[]>,
  replyIndex: Map<bigint, bigint[]>,
  message: Message
): {
  threadIndex: Map<bigint, bigint[]>
  replyIndex: Map<bigint, bigint[]>
} {
  return {
    threadIndex: addMessageToThreadIndex(threadIndex, message),
    replyIndex: addMessageToReplyIndex(replyIndex, message)
  }
}

/**
 * Get all message IDs in a thread (root + all nested replies)
 * Useful for subscription management
 */
export function getThreadMessageIds(
  rootId: bigint,
  replyIndex: Map<bigint, bigint[]>
): bigint[] {
  const ids: bigint[] = [rootId]

  function addReplies(parentId: bigint) {
    const childIds = replyIndex.get(parentId) || []
    for (const childId of childIds) {
      ids.push(childId)
      addReplies(childId) // Recurse for nested replies
    }
  }

  addReplies(rootId)
  return ids
}

/**
 * Check if a message is a root message (thread starter)
 */
export function isRootMessage(message: Message): boolean {
  return message.parent_id.present === 0
}

/**
 * Check if a message is a reply
 */
export function isReply(message: Message): boolean {
  return message.parent_id.present === 1
}

/**
 * Get the root message ID for any message in a thread
 * If the message is already a root, returns its own ID
 */
export function getRootMessageId(
  messageId: bigint,
  messages: Map<bigint, Message>
): bigint | null {
  const message = messages.get(messageId)
  if (!message) return null

  // Already a root
  if (message.parent_id.present === 0) {
    return message.message_id
  }

  // Walk up the parent chain
  let current = message
  while (current.parent_id.present === 1) {
    const parent = messages.get(current.parent_id.value)
    if (!parent) break // Parent not found, shouldn't happen
    current = parent
  }

  return current.message_id
}
