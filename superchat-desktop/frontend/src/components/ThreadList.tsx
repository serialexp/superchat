// Thread List Component
// Displays root messages only (for forum channels)

import { Component, For, Show } from 'solid-js'
import type { Message } from '../SuperChatCodec'
import { formatMessageTime } from '../lib/utils/date-formatting'
import { selectors } from '../store/selectors'

interface ThreadListProps {
  threads: Message[]
  onThreadClick: (threadId: bigint) => void
}

const ThreadList: Component<ThreadListProps> = (props) => {
  return (
    <div class="flex-1 overflow-y-auto p-4 bg-base-100">
      <Show
        when={props.threads.length > 0}
        fallback={
          <div class="text-center text-base-content/50 py-8">
            <p>No threads yet. Start a conversation!</p>
          </div>
        }
      >
        <div class="space-y-3">
          <For each={props.threads}>
            {(thread) => {
              const replyCount = selectors.getReplyCount(thread.message_id)

              return (
                <div
                  onClick={() => props.onThreadClick(thread.message_id)}
                  class="card bg-gradient-to-br from-base-200 to-base-300 shadow-md hover:shadow-lg hover:-translate-y-0.5 transition-all duration-200 cursor-pointer border border-base-300"
                >
                  <div class="card-body p-4">
                    {/* Thread Header */}
                    <div class="flex items-start justify-between gap-4 mb-2">
                      <div class="flex items-baseline gap-2">
                        <span class="font-semibold text-xs text-secondary">{thread.author_nickname}</span>
                        <span class="text-xs text-base-content/50">
                          {formatMessageTime(thread.created_at)}
                        </span>
                      </div>
                      <Show when={replyCount > 0}>
                        <div class="badge badge-primary badge-sm">
                          {replyCount} {replyCount === 1 ? 'reply' : 'replies'}
                        </div>
                      </Show>
                    </div>

                    {/* Thread Preview */}
                    <div class="text-base line-clamp-3">
                      {thread.content}
                    </div>
                  </div>
                </div>
              )
            }}
          </For>
        </div>
      </Show>
    </div>
  )
}

export default ThreadList
