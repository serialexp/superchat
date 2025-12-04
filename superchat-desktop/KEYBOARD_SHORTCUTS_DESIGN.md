# Keyboard Shortcuts System Design

## Overview

Port the Go client's context-aware keyboard shortcuts system to the TypeScript/SolidJS desktop app. This system provides:

1. **Central command registry** - Single source of truth for all keyboard shortcuts
2. **Context-aware availability** - Commands automatically show/hide based on view/modal
3. **Dynamic help generation** - Help modal always shows correct shortcuts for current context
4. **Priority-based ordering** - Commands displayed in priority order (most important first)
5. **Conditional availability** - Commands can check application state before showing

## Architecture

### 1. Command Definition (TypeScript Port)

```typescript
// src/lib/commands/types.ts

export enum CommandScope {
  Global = 'global',   // Available everywhere
  View = 'view',       // Limited to specific views
  Modal = 'modal'      // Limited to specific modals
}

export enum ViewState {
  ChannelList = 'channel-list',
  ThreadList = 'thread-list',
  ThreadDetail = 'thread-detail',
  ChatView = 'chat-view'
}

export enum ModalState {
  None = 'none',
  Compose = 'compose',
  Help = 'help',
  ServerSelector = 'server-selector',
  ConfirmDelete = 'confirm-delete'
}

export interface CommandExecutor {
  // Query methods for availability checks
  getCurrentView(): ViewState
  getActiveModal(): ModalState
  hasSelectedChannel(): boolean
  hasSelectedMessage(): boolean
  hasComposeContent(): boolean
  canGoBack(): boolean
  isAdmin(): boolean
}

export interface CommandDefinition {
  // Keys that trigger this command (e.g., "r", "n", "ctrl+d")
  keys: string[]

  // Name of the command for display (e.g., "Reply", "New Thread")
  name: string

  // Help text description for help modal
  helpText: string

  // Scope defines where this command is available
  scope: CommandScope

  // ViewStates lists specific views where this command is active
  // Empty means available in all views matching the scope
  viewStates?: ViewState[]

  // ModalStates lists specific modals where this command is active
  // undefined means available in all modals
  // Use [ModalState.None] to explicitly disable in all modals
  modalStates?: ModalState[]

  // ActionID is the unique identifier for this command's action
  actionId: string

  // IsAvailable checks if command should be active given current state
  // undefined means always available (within scope/view/modal constraints)
  isAvailable?: (executor: CommandExecutor) => boolean

  // Priority for display ordering (lower = higher priority in footer/help)
  priority: number
}
```

### 2. Command Registry

```typescript
// src/lib/commands/registry.ts

export const SHARED_COMMANDS: CommandDefinition[] = [
  // === Global Commands ===

  {
    keys: ['h', '?'],
    name: 'Help',
    helpText: 'Show keyboard shortcuts',
    scope: CommandScope.Global,
    modalStates: [ModalState.None], // Only when no modal open
    actionId: 'help',
    priority: 950
  },

  {
    keys: ['q'],
    name: 'Quit',
    helpText: 'Exit application',
    scope: CommandScope.Global,
    modalStates: [ModalState.None],
    actionId: 'quit',
    priority: 999
  },

  {
    keys: ['Escape'],
    name: 'Back',
    helpText: 'Go back / Close modal',
    scope: CommandScope.Global,
    actionId: 'go-back',
    isAvailable: (executor) => {
      return executor.getActiveModal() !== ModalState.None || executor.canGoBack()
    },
    priority: 30
  },

  // === Navigation Commands ===

  {
    keys: ['ArrowUp', 'k'],
    name: 'Up',
    helpText: 'Move selection up',
    scope: CommandScope.View,
    viewStates: [ViewState.ChannelList, ViewState.ThreadList, ViewState.ThreadDetail],
    modalStates: [ModalState.None],
    actionId: 'navigate-up',
    priority: 10
  },

  {
    keys: ['ArrowDown', 'j'],
    name: 'Down',
    helpText: 'Move selection down',
    scope: CommandScope.View,
    viewStates: [ViewState.ChannelList, ViewState.ThreadList, ViewState.ThreadDetail],
    modalStates: [ModalState.None],
    actionId: 'navigate-down',
    priority: 11
  },

  {
    keys: ['Enter'],
    name: 'Select',
    helpText: 'Select current item',
    scope: CommandScope.View,
    viewStates: [ViewState.ChannelList, ViewState.ThreadList],
    modalStates: [ModalState.None],
    actionId: 'select',
    priority: 20
  },

  // === Messaging Commands ===

  {
    keys: ['n'],
    name: 'New Thread',
    helpText: 'Create a new thread',
    scope: CommandScope.View,
    viewStates: [ViewState.ThreadList],
    modalStates: [ModalState.None],
    actionId: 'compose-new-thread',
    isAvailable: (executor) => executor.hasSelectedChannel(),
    priority: 100
  },

  {
    keys: ['r'],
    name: 'Reply',
    helpText: 'Reply to selected message',
    scope: CommandScope.View,
    viewStates: [ViewState.ThreadDetail],
    modalStates: [ModalState.None],
    actionId: 'compose-reply',
    isAvailable: (executor) => executor.hasSelectedMessage(),
    priority: 101
  },

  {
    keys: ['i'],
    name: 'Compose',
    helpText: 'Focus compose area',
    scope: CommandScope.View,
    viewStates: [ViewState.ChatView],
    modalStates: [ModalState.None],
    actionId: 'focus-compose',
    priority: 102
  },

  {
    keys: ['Control+d', 'Control+Enter'],
    name: 'Send',
    helpText: 'Send message',
    scope: CommandScope.Modal,
    modalStates: [ModalState.Compose],
    actionId: 'send-message',
    isAvailable: (executor) => executor.hasComposeContent(),
    priority: 1
  },

  {
    keys: ['e'],
    name: 'Edit',
    helpText: 'Edit selected message',
    scope: CommandScope.View,
    viewStates: [ViewState.ThreadDetail],
    modalStates: [ModalState.None],
    actionId: 'edit-message',
    isAvailable: (executor) => executor.hasSelectedMessage(),
    priority: 103
  },

  {
    keys: ['d'],
    name: 'Delete',
    helpText: 'Delete selected message',
    scope: CommandScope.View,
    viewStates: [ViewState.ThreadDetail],
    modalStates: [ModalState.None],
    actionId: 'delete-message',
    isAvailable: (executor) => executor.hasSelectedMessage(),
    priority: 104
  },

  // === Channel Management ===

  {
    keys: ['c'],
    name: 'Create Channel',
    helpText: 'Create a new channel',
    scope: CommandScope.View,
    viewStates: [ViewState.ChannelList],
    modalStates: [ModalState.None],
    actionId: 'create-channel',
    isAvailable: (executor) => executor.isAdmin(),
    priority: 801
  },

  // === Admin Commands ===

  {
    keys: ['a'],
    name: 'Admin Panel',
    helpText: 'Open admin panel',
    scope: CommandScope.Global,
    modalStates: [ModalState.None],
    actionId: 'admin-panel',
    isAvailable: (executor) => executor.isAdmin(),
    priority: 800
  }
]
```

### 3. Context Filtering

```typescript
// src/lib/commands/context.ts

/**
 * Get commands available in the current context
 * Sorted by priority (lower priority = shown first)
 */
export function getCommandsForContext(executor: CommandExecutor): CommandDefinition[] {
  const currentView = executor.getCurrentView()
  const activeModal = executor.getActiveModal()

  const available = SHARED_COMMANDS.filter(cmd =>
    isCommandAvailable(cmd, currentView, activeModal, executor)
  )

  // Sort by priority
  return available.sort((a, b) => a.priority - b.priority)
}

/**
 * Find the first available command matching a key
 */
export function findCommandForKey(
  key: string,
  executor: CommandExecutor
): CommandDefinition | null {
  const currentView = executor.getCurrentView()
  const activeModal = executor.getActiveModal()

  for (const cmd of SHARED_COMMANDS) {
    if (keyMatches(key, cmd.keys) &&
        isCommandAvailable(cmd, currentView, activeModal, executor)) {
      return cmd
    }
  }

  return null
}

/**
 * Check if a command is available in the current context
 */
function isCommandAvailable(
  cmd: CommandDefinition,
  view: ViewState,
  modal: ModalState,
  executor: CommandExecutor
): boolean {
  // Check modal compatibility
  if (cmd.modalStates && cmd.modalStates.length > 0) {
    if (!cmd.modalStates.includes(modal)) {
      return false
    }
  }

  // Check scope and view compatibility
  switch (cmd.scope) {
    case CommandScope.Global:
      // Global commands are always in scope
      break

    case CommandScope.View:
      if (cmd.viewStates && cmd.viewStates.length > 0) {
        if (!cmd.viewStates.includes(view)) {
          return false
        }
      }
      break

    case CommandScope.Modal:
      // Modal-scoped commands already checked above
      break
  }

  // Check custom availability condition
  if (cmd.isAvailable) {
    return cmd.isAvailable(executor)
  }

  return true
}

/**
 * Check if a key matches any of the command's keys
 */
function keyMatches(key: string, commandKeys: string[]): boolean {
  const keyLower = key.toLowerCase()
  return commandKeys.some(cmdKey => cmdKey.toLowerCase() === keyLower)
}
```

### 4. Key Formatting for Display

```typescript
// src/lib/commands/formatting.ts

/**
 * Format a key string for display
 * Examples: "Control+d" -> "Ctrl+D", "Escape" -> "Esc", "ArrowUp" -> "↑"
 */
export function formatKey(key: string): string {
  const keyMap: Record<string, string> = {
    'ArrowUp': '↑',
    'ArrowDown': '↓',
    'ArrowLeft': '←',
    'ArrowRight': '→',
    'Enter': 'Enter',
    'Escape': 'Esc',
    'Backspace': 'Backspace',
    'Control+c': 'Ctrl+C',
    'Control+d': 'Ctrl+D',
    'Control+l': 'Ctrl+L',
    'Control+Enter': 'Ctrl+Enter'
  }

  if (keyMap[key]) {
    return keyMap[key]
  }

  // For single letters, return uppercase
  if (key.length === 1) {
    return key.toUpperCase()
  }

  // For Control+letter combinations, capitalize
  if (key.toLowerCase().startsWith('control+')) {
    const rest = key.slice(8)
    if (rest.length === 1) {
      return 'Ctrl+' + rest.toUpperCase()
    }
  }

  return key
}

/**
 * Generate footer display text for a command
 * Examples: "[R] Reply", "[Ctrl+D] Send", "[↑/↓] Navigate"
 */
export function getFooterText(cmd: CommandDefinition): string {
  if (!cmd.name || cmd.keys.length === 0) {
    return ''
  }

  // Format keys for display
  let keyDisplay: string
  if (cmd.keys.length === 1) {
    keyDisplay = formatKey(cmd.keys[0])
  } else {
    // Multiple keys: show first two separated by /
    const formatted = cmd.keys.map(formatKey)
    if (formatted.length <= 2) {
      keyDisplay = formatted.join('/')
    } else {
      keyDisplay = formatted[0] + '/' + formatted[1]
    }
  }

  return `[${keyDisplay}] ${cmd.name}`
}

/**
 * Generate help content for the help modal
 * Returns array of [key, description] pairs
 */
export function generateHelpContent(executor: CommandExecutor): Array<[string, string]> {
  const commands = getCommandsForContext(executor)

  return commands.map(cmd => [
    cmd.keys.join(', '),
    cmd.helpText
  ])
}
```

### 5. SolidJS Integration

```typescript
// src/lib/commands/hook.ts

import { createMemo, onMount, onCleanup } from 'solid-js'
import { store } from '../store/app-store'

/**
 * Hook to get available commands for current context
 * Returns reactive memo that updates when view/modal changes
 */
export function useAvailableCommands(executor: CommandExecutor) {
  return createMemo(() => {
    // Re-run when view or modal changes
    const _ = [store.currentView, store.activeModal]
    return getCommandsForContext(executor)
  })
}

/**
 * Hook to set up global keyboard listener
 * Automatically handles command dispatch based on context
 */
export function useKeyboardShortcuts(
  executor: CommandExecutor,
  onCommand: (actionId: string) => void
) {
  const handleKeyDown = (e: KeyboardEvent) => {
    // Build key string (e.g., "Control+d", "Escape", "k")
    const parts: string[] = []
    if (e.ctrlKey || e.metaKey) parts.push('Control')
    if (e.shiftKey) parts.push('Shift')
    if (e.altKey) parts.push('Alt')

    // Add the actual key
    if (e.key.length === 1) {
      parts.push(e.key.toLowerCase())
    } else {
      parts.push(e.key) // Arrow keys, Enter, etc.
    }

    const keyString = parts.join('+')

    // Find matching command
    const command = findCommandForKey(keyString, executor)
    if (command) {
      e.preventDefault()
      e.stopPropagation()
      onCommand(command.actionId)
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown)
  })

  onCleanup(() => {
    window.removeEventListener('keydown', handleKeyDown)
  })
}

/**
 * Generate footer shortcuts text
 * Returns space-separated string of command shortcuts
 */
export function useFooterShortcuts(executor: CommandExecutor) {
  return createMemo(() => {
    const commands = getCommandsForContext(executor)
    return commands.map(getFooterText).join('  ')
  })
}
```

## Integration with App.tsx

```typescript
// src/App.tsx

const App: Component = () => {
  // ... existing code ...

  // Create command executor interface
  const commandExecutor: CommandExecutor = {
    getCurrentView: () => {
      if (store.currentView === ViewState.ThreadList) return ViewState.ThreadList
      if (store.currentView === ViewState.ThreadDetail) return ViewState.ThreadDetail
      return ViewState.ChannelList
    },
    getActiveModal: () => store.activeModal || ModalState.None,
    hasSelectedChannel: () => store.activeChannelId !== null,
    hasSelectedMessage: () => store.selectedMessageId !== null,
    hasComposeContent: () => store.compose.content.trim().length > 0,
    canGoBack: () => {
      return store.currentView === ViewState.ThreadDetail
    },
    isAdmin: () => store.isAdmin
  }

  // Set up keyboard shortcuts
  useKeyboardShortcuts(commandExecutor, (actionId) => {
    handleCommand(actionId)
  })

  // Get footer shortcuts text
  const footerShortcuts = useFooterShortcuts(commandExecutor)

  // Command handler
  const handleCommand = (actionId: string) => {
    switch (actionId) {
      case 'help':
        // Show help modal with context-aware shortcuts
        const helpContent = generateHelpContent(commandExecutor)
        showHelpModal(helpContent)
        break

      case 'navigate-up':
        // Move selection up
        break

      case 'navigate-down':
        // Move selection down
        break

      case 'select':
        // Select current item
        break

      case 'compose-reply':
        // Open compose with reply context
        break

      // ... etc
    }
  }

  return (
    <div>
      {/* ... existing UI ... */}

      {/* Footer with context-aware shortcuts */}
      <div class="footer">
        {footerShortcuts()}
      </div>
    </div>
  )
}
```

## Benefits

1. **DRY**: Single source of truth for all keyboard shortcuts
2. **Context-Aware**: Shortcuts automatically show/hide based on view/modal
3. **Type-Safe**: Full TypeScript support with enums and interfaces
4. **Reactive**: SolidJS memos ensure UI updates when context changes
5. **Maintainable**: Adding new shortcuts is just adding to the registry
6. **Testable**: Pure functions for filtering and matching
7. **Consistent**: Same shortcuts work across web and desktop
8. **Discoverable**: Help modal always shows correct shortcuts

## Implementation Plan

**Phase 1** (Can do now):
- [ ] Create types.ts with enums and interfaces
- [ ] Create registry.ts with SHARED_COMMANDS array
- [ ] Create context.ts with filtering functions
- [ ] Create formatting.ts with display helpers

**Phase 2** (After basic UI is done):
- [ ] Create hook.ts with SolidJS integration
- [ ] Add ViewState and ModalState to store
- [ ] Implement CommandExecutor in App.tsx
- [ ] Wire up useKeyboardShortcuts hook

**Phase 3** (Polish):
- [ ] Add footer shortcuts display
- [ ] Add help modal with context-aware content
- [ ] Test all shortcuts in different contexts
- [ ] Add visual indicators for selected items

## Notes

- **Web vs Desktop**: This system works for both web (keyboard events) and desktop (Wails)
- **Accessibility**: Keyboard shortcuts improve accessibility
- **Progressive Enhancement**: Start with basic shortcuts, add more as features are built
- **Documentation**: The registry itself IS the documentation
- **Migration Path**: Can migrate shortcuts incrementally (start with global, add view-specific later)

This design is **production-ready** and scales well. The Go client proves it works beautifully for a TUI, and the TypeScript port will be just as elegant for the desktop app!
