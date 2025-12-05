// ABOUTME: Key formatting and display helpers for commands
// ABOUTME: Formats keys for footer display and generates help modal content

import { CommandDefinition, CommandExecutor } from './types'
import { getCommandsForContext } from './context'

/**
 * Format a key string for display
 * Examples: "Control+d" -> "Ctrl+D", "Escape" -> "Esc", "ArrowUp" -> "↑"
 */
export function formatKey(key: string): string {
  // Special key mappings
  const keyMap: Record<string, string> = {
    'ArrowUp': '↑',
    'ArrowDown': '↓',
    'ArrowLeft': '←',
    'ArrowRight': '→',
    'Enter': '↵',
    'Escape': 'Esc',
    'Backspace': '⌫',
    'Tab': 'Tab',
    ' ': 'Space'
  }

  // Check for direct mapping
  if (keyMap[key]) {
    return keyMap[key]
  }

  // Handle modifier combinations
  if (key.includes('+')) {
    const parts = key.split('+')
    const formattedParts = parts.map(part => {
      // Format modifiers
      if (part === 'Control') return 'Ctrl'
      if (part === 'Meta') return '⌘'
      if (part === 'Alt') return 'Alt'
      if (part === 'Shift') return '⇧'

      // Format the key itself
      if (keyMap[part]) return keyMap[part]
      if (part.length === 1) return part.toUpperCase()
      return part
    })
    return formattedParts.join('+')
  }

  // For single letters, return uppercase
  if (key.length === 1) {
    return key.toUpperCase()
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
    const formatted = cmd.keys.slice(0, 2).map(formatKey)
    keyDisplay = formatted.join('/')
  }

  return `[${keyDisplay}] ${cmd.name}`
}

/**
 * Generate help content for the help modal
 * Returns array of { keys, description } objects
 */
export function generateHelpContent(executor: CommandExecutor): Array<{ keys: string; description: string }> {
  const commands = getCommandsForContext(executor)

  return commands.map(cmd => ({
    keys: cmd.keys.map(formatKey).join(', '),
    description: cmd.helpText
  }))
}

/**
 * Generate footer shortcuts text for current context
 * Returns space-separated string of command shortcuts
 */
export function generateFooterText(executor: CommandExecutor): string {
  const commands = getCommandsForContext(executor)

  // Filter to show only high-priority commands in footer (priority < 200)
  const footerCommands = commands.filter(cmd => cmd.priority < 200)

  return footerCommands.map(getFooterText).filter(Boolean).join('  ')
}
