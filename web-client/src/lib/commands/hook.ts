// ABOUTME: SolidJS hooks for keyboard shortcut integration
// ABOUTME: Provides useKeyboardShortcuts hook for global keyboard handling

import { createMemo, onMount, onCleanup } from 'solid-js'
import { CommandExecutor } from './types'
import { findCommandForKey, buildKeyString, getCommandsForContext } from './context'
import { generateFooterText } from './formatting'

/**
 * Hook to get available commands for current context
 * Returns reactive memo that updates when view/modal changes
 */
export function useAvailableCommands(executor: CommandExecutor) {
  return createMemo(() => {
    // The executor methods access reactive signals, so this memo
    // will automatically re-run when those signals change
    return getCommandsForContext(executor)
  })
}

/**
 * Hook to set up global keyboard listener
 * Automatically handles command dispatch based on context
 *
 * @param executor - Object implementing CommandExecutor interface
 * @param onCommand - Callback when a command is matched
 * @param isEnabled - Optional function to check if shortcuts should be active
 */
export function useKeyboardShortcuts(
  executor: CommandExecutor,
  onCommand: (actionId: string) => void,
  isEnabled?: () => boolean
) {
  const handleKeyDown = (e: KeyboardEvent) => {
    // Check if shortcuts are enabled
    if (isEnabled && !isEnabled()) {
      return
    }

    // Skip if focus is in an input/textarea (unless it's a modifier combo)
    const target = e.target as HTMLElement
    const isInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA'
    const hasModifier = e.ctrlKey || e.metaKey || e.altKey

    // For inputs, only process modifier shortcuts (Ctrl+D, etc.) and Escape
    if (isInput && !hasModifier && e.key !== 'Escape') {
      return
    }

    // Build the key string
    const keyString = buildKeyString(e)

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
 * Hook to generate footer shortcuts text for current context
 * Returns reactive memo that updates when context changes
 */
export function useFooterShortcuts(executor: CommandExecutor) {
  return createMemo(() => {
    return generateFooterText(executor)
  })
}
