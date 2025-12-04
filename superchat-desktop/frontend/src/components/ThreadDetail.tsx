// Thread Detail Component
// Displays a thread with all nested replies

import { Component, For, Show, createSignal } from 'solid-js'
import type { Message } from '../SuperChatCodec'
import { formatMessageTime } from '../lib/utils/date-formatting'
import arrowBendUpLeftIcon from '@phosphor-icons/core/bold/arrow-bend-up-left-bold.svg'

// Message with nested replies (from selector)
interface MessageWithReplies extends Message {
  replies: MessageWithReplies[]
}

interface ThreadDetailProps {
  thread: MessageWithReplies | null
  onReply: (messageId: bigint, message: Message) => void
  onBack: () => void
}

const ThreadDetail: Component<ThreadDetailProps> = (props) => {
  return (
    <div class="flex-1 overflow-y-auto p-4 bg-base-100">
      <Show when={props.thread}>
        {(thread) => (
          <div class="space-y-4">
            {/* Root Message (highlighted) */}
            <div class="card bg-gradient-to-br from-primary/10 to-primary/5 shadow-lg border-l-4 border-primary relative">
              <div class="card-body p-4">
                {/* Reply button in top-right corner */}
                <button
                  onClick={() => props.onReply(thread().message_id, thread())}
                  class="btn btn-xs btn-ghost absolute top-2 right-2"
                  title="Reply to this message"
                >
                  <img src={arrowBendUpLeftIcon} alt="Reply" width="16" height="16" class="inline-block" />
                </button>

                <div class="flex items-baseline gap-2 mb-2">
                  <span class="font-semibold text-xs text-secondary">{thread().author_nickname}</span>
                  <span class="text-xs text-base-content/50">
                    {formatMessageTime(thread().created_at)}
                  </span>
                </div>
                <div class="text-base whitespace-pre-wrap pr-12">
                  {thread().content}
                </div>
              </div>
            </div>

            {/* Nested Replies */}
            <Show when={thread().replies.length > 0}>
              <div class="space-y-3">
                <For each={thread().replies}>
                  {(reply) => <MessageWithRepliesComponent message={reply} depth={1} onReply={props.onReply} />}
                </For>
              </div>
            </Show>
          </div>
        )}
      </Show>
    </div>
  )
}

// Recursive component for rendering nested replies
interface MessageWithRepliesComponentProps {
  message: MessageWithReplies
  depth: number
  onReply: (messageId: bigint, message: Message) => void
}

const MessageWithRepliesComponent: Component<MessageWithRepliesComponentProps> = (props) => {
  return (
    <div class={props.depth > 0 ? `ml-6 border-l-2 border-base-300 pl-4` : ''}>
      <div class="card bg-base-200 hover:bg-base-300 transition-colors relative">
        <div class="card-body p-3">
          {/* Reply button in top-right corner */}
          <button
            onClick={() => props.onReply(props.message.message_id, props.message)}
            class="btn btn-xs btn-ghost absolute top-2 right-2"
            title="Reply to this message"
          >
            <img src={arrowBendUpLeftIcon} alt="Reply" width="16" height="16" class="inline-block" />
          </button>

          <div class="flex items-baseline gap-2 mb-1">
            <span class="font-semibold text-xs text-secondary">{props.message.author_nickname}</span>
            <span class="text-xs text-base-content/50">
              {formatMessageTime(props.message.created_at)}
            </span>
          </div>
          <div class="text-base whitespace-pre-wrap pr-12">
            {props.message.content}
          </div>
        </div>
      </div>

      {/* Nested replies */}
      <Show when={props.message.replies.length > 0}>
        <div class="mt-3 space-y-3">
          <For each={props.message.replies}>
            {(reply) => (
              <MessageWithRepliesComponent
                message={reply}
                depth={props.depth + 1}
                onReply={props.onReply}
              />
            )}
          </For>
        </div>
      </Show>
    </div>
  )
}

export default ThreadDetail
