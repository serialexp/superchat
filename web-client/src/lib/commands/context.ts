// ABOUTME: Context-aware command filtering and key matching
// ABOUTME: Determines which commands are available based on current view/modal state

import {
  CommandDefinition,
  CommandScope,
  ViewState,
  ModalState,
  CommandExecutor
} from './types'
import { SHARED_COMMANDS } from './registry'

/**
 * Get commands available in the current context
 * Sorted by priority (lower priority number = shown first)
 */
export function getCommandsForContext(executor: CommandExecutor): CommandDefinition[] {
  const currentView = executor.getCurrentView()
  const activeModal = executor.getActiveModal()

  const available = SHARED_COMMANDS.filter(cmd =>
    isCommandAvailable(cmd, currentView, activeModal, executor)
  )

  // Sort by priority (lower = higher priority)
  return available.sort((a, b) => a.priority - b.priority)
}

/**
 * Find the first available command matching a key
 * Returns null if no matching command is available
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
      // Global commands are always in scope (if modal check passed)
      break

    case CommandScope.View:
      if (cmd.viewStates && cmd.viewStates.length > 0) {
        if (!cmd.viewStates.includes(view)) {
          return false
        }
      }
      break

    case CommandScope.Modal:
      // Modal-scoped commands already checked above via modalStates
      // If no modalStates defined, it's available in any modal
      if (modal === ModalState.None) {
        return false // Modal-scoped commands require a modal to be open
      }
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
 * Handles case-insensitive matching for letters and exact matching for modifiers
 */
function keyMatches(key: string, commandKeys: string[]): boolean {
  const keyLower = key.toLowerCase()

  return commandKeys.some(cmdKey => {
    const cmdKeyLower = cmdKey.toLowerCase()

    // For modifier combinations (Control+x, Meta+x), compare case-insensitively
    if (cmdKeyLower.includes('+')) {
      return cmdKeyLower === keyLower
    }

    // For single keys, compare case-insensitively
    return cmdKeyLower === keyLower
  })
}

/**
 * Build a key string from a KeyboardEvent
 * Returns strings like "Control+d", "Meta+d", "Escape", "k", "ArrowUp"
 */
export function buildKeyString(e: KeyboardEvent): string {
  const parts: string[] = []

  // Add modifiers
  if (e.ctrlKey) parts.push('Control')
  if (e.metaKey) parts.push('Meta')
  if (e.altKey) parts.push('Alt')
  if (e.shiftKey) parts.push('Shift')

  // Add the actual key
  // For single printable characters, use the key directly
  // For special keys (Enter, Escape, Arrow*), use the key name
  if (e.key.length === 1) {
    parts.push(e.key.toLowerCase())
  } else {
    parts.push(e.key)
  }

  return parts.join('+')
}
