# Web Client Threading Implementation Plan

## Current Status
✅ Schema synchronized with Go server (subchannel_id, thread_depth removed, message_id fixed)
✅ Basic message display working
✅ Channel joining working
✅ Threaded view implemented (two-level navigation: thread list → thread detail)
✅ Subscriptions implemented for real-time updates (channel and thread subscriptions)
⏳ Ready for testing

## Threading Model (from Go Client Analysis)

The Go client uses a two-view threading system:

### View 1: Thread List (ViewThreadList)
- Shows only **root-level messages** (messages with `parent_id.present === 0`)
- Each thread item shows:
  - Author nickname
  - Message preview (first line of content)
  - Time posted
  - Reply count (if > 0)
- User can:
  - Navigate with up/down arrows
  - Press Enter to open full thread
  - Press 'n' to create new thread
  - Press Escape to return to channel list

### View 2: Thread Detail (ViewThreadView)
- Shows full thread: root message + all replies
- Displays message hierarchy with indentation
- Each message shows:
  - Author nickname
  - Full message content
  - Timestamp
  - Reply count
- User can:
  - Navigate with up/down arrows (selects individual messages)
  - Press 'r' to reply to selected message
  - Press 'e' to edit own message
  - Press 'd' to delete own message
  - Press Escape to return to thread list

## Implementation Phases

### Phase 1: Add State Management ✅

**File:** `web-client/src/main.ts`

**Changes needed:**
- [x] Add view state enum:
  ```typescript
  enum ViewState {
    ThreadList,
    ThreadDetail
  }
  ```
- [x] Add to SuperChatClient class:
  ```typescript
  private currentView: ViewState = ViewState.ThreadList;
  private currentThread: Message | null = null;
  private threads: Message[] = []; // Root messages only
  private threadReplies: Map<bigint, Message[]> = new Map(); // Replies by thread ID
  ```
- [x] Modify `handleMessageList()` to separate roots from replies:
  ```typescript
  // Filter messages:
  // - Roots: parent_id.present === 0
  // - Replies: parent_id.present === 1
  // Store roots in this.threads
  // Store replies in this.threadReplies[parent_id]
  ```

### Phase 2: Update UI - Thread List View ✅

**File:** `web-client/src/main.ts`

**Changes needed:**
- [x] Modify `renderMessages()` to check `currentView`
- [x] Implement `renderThreadList()`:
  ```typescript
  private renderThreadList() {
    // Show list of root messages with:
    // - Author
    // - Content preview (first 50 chars)
    // - Time
    // - Reply count badge if > 0
    // - Click handler to open thread
  }
  ```
- [ ] Add click handlers to open thread:
  ```typescript
  threadItem.addEventListener('click', () => {
    this.currentThread = thread;
    this.currentView = ViewState.ThreadDetail;
    this.loadThreadReplies(thread.message_id);
  });
  ```

**File:** `web-client/index.html`

**CSS additions needed:**
- [x] `.thread-item` - Style for thread list items
- [x] `.thread-item:hover` - Hover state
- [x] `.thread-preview` - Style for content preview
- [x] `.reply-count-badge` - Badge showing reply count

### Phase 3: Update UI - Thread Detail View ✅

**File:** `web-client/src/main.ts`

**Changes needed:**
- [x] Implement `renderThreadDetail()`:
  ```typescript
  private renderThreadDetail() {
    // Show root message at top
    // Show all replies below with indentation
    // Add "Reply" button next to each message
    // Add "Back to threads" button at top
  }
  ```
- [x] Add back button handler:
  ```typescript
  backButton.addEventListener('click', () => {
    this.currentThread = null;
    this.currentView = ViewState.ThreadList;
    this.renderMessages();
  });
  ```
- [x] Add reply button handlers:
  ```typescript
  replyButton.addEventListener('click', () => {
    this.replyToMessage(message.message_id);
  });
  ```
- [x] Implement `replyToMessage()`:
  ```typescript
  private replyToMessage(parentId: bigint) {
    // Set parent_id in compose area
    // Show indicator of who you're replying to
    // Focus textarea
  }
  ```

**File:** `web-client/index.html`

**HTML structure changes:**
- [x] Add back button to header:
  ```html
  <button id="back-button" style="display: none;">← Back to Threads</button>
  ```
- [ ] Modify compose area to show reply context:
  ```html
  <div id="reply-context" style="display: none;">
    Replying to: <span id="reply-to-author"></span>
    <button id="cancel-reply">Cancel</button>
  </div>
  ```

**CSS additions:**
- [ ] `.thread-detail` - Container for full thread view
- [ ] `.thread-root` - Style for root message (highlighted)
- [ ] `.thread-reply` - Style for reply messages
- [ ] `.reply-indent-1`, `.reply-indent-2`, etc. - Indentation levels
- [ ] `.reply-button` - Style for reply buttons
- [ ] `.back-button` - Style for back button
- [ ] `.reply-context` - Style for reply indicator in compose

### Phase 4: Load Thread Replies on Demand ✅

**File:** `web-client/src/main.ts`

**Changes needed:**
- [x] Implement `loadThreadReplies()`:
  ```typescript
  private loadThreadReplies(threadId: bigint) {
    // Send LIST_MESSAGES with parent_id = threadId
    // This loads all replies to this thread
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
  ```
- [x] Update `handleMessageList()` to handle thread-specific replies:
  ```typescript
  // If parent_id was set in request, these are replies to a specific thread
  // Store in this.threadReplies[parent_id]
  ```

### Phase 5: Implement Subscriptions ✅

**Background:** The Go client uses subscriptions for real-time updates

**Protocol messages needed:**
```
SUBSCRIBE_CHANNEL (0x14) - Subscribe to channel updates
SUBSCRIBE_THREAD (0x15) - Subscribe to specific thread updates
UNSUBSCRIBE_CHANNEL (0x16) - Unsubscribe from channel
UNSUBSCRIBE_THREAD (0x17) - Unsubscribe from thread
SUBSCRIBE_OK (0x99) - Server confirms subscription
```

**File:** `tools/binschema/examples/superchat.schema.json`

**Schema additions needed:**
- [x] Check if SUBSCRIBE_CHANNEL/THREAD messages exist in schema
- [x] Add if missing:
  ```json
  "SubscribeChannel": {
    "description": "Subscribe to channel for real-time updates",
    "sequence": [
      { "name": "channel_id", "type": "uint64" },
      { "name": "subchannel_id", "type": "Optional<uint64>" }
    ]
  }
  ```
- [x] Similar for SUBSCRIBE_THREAD, UNSUBSCRIBE_*, SUBSCRIBE_OK

**File:** `web-client/src/main.ts`

**Implementation steps:**
- [x] Add message type constants (check Go protocol for correct codes):
  ```typescript
  const MSG_SUBSCRIBE_CHANNEL = 0x14;
  const MSG_SUBSCRIBE_THREAD = 0x15;
  const MSG_UNSUBSCRIBE_CHANNEL = 0x16;
  const MSG_UNSUBSCRIBE_THREAD = 0x17;
  const MSG_SUBSCRIBE_OK = 0x99;
  ```
- [x] Implement subscription methods:
  ```typescript
  private subscribeToChannel(channelId: bigint) {
    const encoder = new SubscribeChannelEncoder();
    const payload = encoder.encode({
      channel_id: channelId,
      subchannel_id: { present: 0 }
    });
    this.sendFrame(MSG_SUBSCRIBE_CHANNEL, payload);
  }

  private subscribeToThread(threadId: bigint) {
    const encoder = new SubscribeThreadEncoder();
    const payload = encoder.encode({
      message_id: threadId
    });
    this.sendFrame(MSG_SUBSCRIBE_THREAD, payload);
  }
  ```
- [x] Call subscriptions at the right times:
  - Subscribe to channel when joining: After JOIN_RESPONSE success
  - Subscribe to thread when opening: After loading thread replies
  - Unsubscribe from thread when going back: Before switching to thread list
  - Unsubscribe from channel when leaving: Before joining another channel

**File:** `web-client/src/main.ts` - Handle NEW_MESSAGE properly

**Changes needed:**
- [x] Update `handleNewMessage()` to handle thread context:
  ```typescript
  private handleNewMessage(payload: Uint8Array) {
    const newMsg = decoder.decode();

    // If it's a root message (parent_id.present === 0)
    if (newMsg.parent_id.present === 0) {
      // Add to threads list
      this.threads.push(newMsg);
      // Update thread list if in that view
      if (this.currentView === ViewState.ThreadList) {
        this.renderThreadList();
      }
    } else {
      // It's a reply - add to threadReplies map
      const parentId = newMsg.parent_id.value!;
      const replies = this.threadReplies.get(parentId) || [];
      replies.push(newMsg);
      this.threadReplies.set(parentId, replies);

      // Update thread detail if viewing this thread
      if (this.currentView === ViewState.ThreadDetail &&
          this.currentThread?.message_id === parentId) {
        this.renderThreadDetail();
      }
    }

    // Update reply counts in thread list
    this.updateThreadReplyCounts();
  }
  ```

### Phase 6: Polish & Testing ⏳

**Testing checklist:**
- [ ] Thread list shows only root messages
- [ ] Thread list updates reply counts in real-time
- [ ] Clicking thread opens detail view
- [ ] Thread detail shows root + all replies
- [ ] Reply button sets parent_id correctly
- [ ] Sending reply updates thread immediately
- [ ] NEW_MESSAGE updates correct view
- [ ] Back button returns to thread list
- [ ] Subscriptions prevent missed messages
- [ ] Multiple users see each other's messages in real-time
- [ ] Reply hierarchy is clear (indentation)
- [ ] Cancel reply clears parent_id

**Performance considerations:**
- [ ] Don't reload entire thread on every NEW_MESSAGE
- [ ] Limit initial thread list to 50 threads
- [ ] Load more threads on scroll (infinite scroll)
- [ ] Limit replies per thread to 100 initially

## Key Differences from Current Implementation

**Current:**
- Single flat view showing all messages with simple indentation
- No concept of "opening" a thread
- No separation between roots and replies

**Target:**
- Two-level navigation: Thread list → Thread detail
- Clear visual hierarchy
- Proper subscription management
- Reply counts on thread list items
- Better use of screen space (thread list more compact)

## Files That Need Changes

1. ✅ `tools/binschema/examples/superchat.schema.json` - Already has message types
2. ⏳ `web-client/src/main.ts` - Main implementation (bulk of work)
3. ⏳ `web-client/index.html` - HTML structure changes
4. ⏳ `web-client/index.html` - CSS additions (inline styles)

## Protocol Message Flow

### Opening a Forum Channel
```
Client → Server: JOIN_CHANNEL (channel_id)
Server → Client: JOIN_RESPONSE (success)
Client → Server: SUBSCRIBE_CHANNEL (channel_id)
Server → Client: SUBSCRIBE_OK
Client → Server: LIST_MESSAGES (channel_id, parent_id=null, limit=50)
Server → Client: MESSAGE_LIST (roots only)
[Client displays thread list]
```

### Opening a Thread
```
Client → Server: SUBSCRIBE_THREAD (message_id)
Server → Client: SUBSCRIBE_OK
Client → Server: LIST_MESSAGES (channel_id, parent_id=thread_id, limit=100)
Server → Client: MESSAGE_LIST (replies)
[Client displays thread detail]
```

### Real-Time Update (New Root Message)
```
Server → Client: NEW_MESSAGE (parent_id=null)
[Client adds to thread list if subscribed to channel]
```

### Real-Time Update (New Reply)
```
Server → Client: NEW_MESSAGE (parent_id=thread_id)
[Client adds to thread detail if viewing that thread]
[Client increments reply count in thread list]
```

## Next Steps When Resuming

1. Start with Phase 1: Add state management to main.ts
2. Test that messages are correctly separated into threads vs replies
3. Implement Phase 2: Thread list rendering
4. Test thread list displays correctly
5. Implement Phase 3: Thread detail rendering
6. Test navigation between views
7. Implement Phase 4: Load replies on demand
8. Finally tackle Phase 5: Subscriptions (most complex)

## Notes
- The Go client code is in `pkg/client/ui/view.go` (line 196+) and `pkg/client/ui/update.go`
- Subscription logic is in `pkg/client/ui/update.go` around lines 2137-2147
- Message formatting is in `pkg/client/ui/view.go` starting line 535
