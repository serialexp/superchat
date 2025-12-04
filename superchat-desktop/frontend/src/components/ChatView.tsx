// Chat View Component
// Displays flat chronological messages for chat channels (type=0)

import { Component, Show, Index } from 'solid-js'
import type { Message } from '../SuperChatCodec'
import { formatMessageTime, formatDateSeparator, shouldShowDateSeparator } from '../lib/utils/date-formatting'

interface ChatViewProps {
  messages: Message[]
}

const ChatView: Component<ChatViewProps> = (props) => {
  return (
    <div
      class="flex-1 p-4 bg-base-100"
    >
      <Show
        when={props.messages.length > 0}
        fallback={
          <div class="text-center text-base-content/50 py-8">
            <p>No messages yet. Start a conversation!</p>
          </div>
        }
      >
        <div class="space-y-2">
          <Index each={props.messages}>
            {(message, index) => {
              const prevMessage = index > 0 ? props.messages[index - 1] : null
              const showDateSeparator = shouldShowDateSeparator(
                prevMessage?.created_at || null,
                message().created_at
              )

              return (
                <>
                  <Show when={showDateSeparator}>
                    <div class="flex items-center gap-4 my-4">
                      <div class="flex-1 border-t border-base-300"></div>
                      <div class="text-xs font-semibold text-base-content/50 uppercase tracking-wide">
                        {formatDateSeparator(message().created_at)}
                      </div>
                      <div class="flex-1 border-t border-base-300"></div>
                    </div>
                  </Show>
                  <div class="p-3 bg-base-200 rounded-lg hover:bg-base-300 transition-colors">
                    <div class="flex items-baseline gap-2 mb-1">
                      <span class="font-semibold text-xs text-secondary">{message().author_nickname}</span>
                      <span class="text-xs text-base-content/50">
                        {formatMessageTime(message().created_at)}
                      </span>
                    </div>
                    <div class="text-base whitespace-pre-wrap">{message().content}</div>
                  </div>
                </>
              )
            }}
          </Index>
        </div>
      </Show>
    </div>
  )
}

export default ChatView
