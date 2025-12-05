// ABOUTME: Public exports for the keyboard command system
// ABOUTME: Re-exports types, registry, context, formatting, and hooks

export * from './types'
export { SHARED_COMMANDS } from './registry'
export { getCommandsForContext, findCommandForKey, buildKeyString } from './context'
export { formatKey, getFooterText, generateHelpContent, generateFooterText } from './formatting'
export { useKeyboardShortcuts, useAvailableCommands, useFooterShortcuts } from './hook'
