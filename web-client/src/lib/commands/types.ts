// ABOUTME: Type definitions for the keyboard command system
// ABOUTME: Defines CommandScope, ViewState, ModalState, CommandExecutor, and CommandDefinition

export enum CommandScope {
  Global = 'global',   // Available everywhere
  View = 'view',       // Limited to specific views
  Modal = 'modal'      // Limited to specific modals
}

// View states - matches store ViewState plus additions
export enum ViewState {
  ChannelList = 'channel-list',
  ThreadList = 'thread-list',
  ThreadDetail = 'thread-detail',
  ChatView = 'chat-view'
}

// Modal states
export enum ModalState {
  None = 'none',
  Compose = 'compose',
  Help = 'help',
  ServerSelector = 'server-selector',
  ConfirmDelete = 'confirm-delete'
}

// Interface for querying application state
// Implemented by App.tsx to provide context to commands
export interface CommandExecutor {
  getCurrentView(): ViewState
  getActiveModal(): ModalState
  hasSelectedChannel(): boolean
  hasSelectedMessage(): boolean
  hasSelectedThread(): boolean
  hasComposeContent(): boolean
  canGoBack(): boolean
  isAdmin(): boolean
  isConnected(): boolean
}

// Command definition - single source of truth for keyboard shortcuts
export interface CommandDefinition {
  // Keys that trigger this command (e.g., "r", "n", "Control+d")
  keys: string[]

  // Name of the command for display (e.g., "Reply", "New Thread")
  name: string

  // Help text description for help modal
  helpText: string

  // Scope defines where this command is available
  scope: CommandScope

  // ViewStates lists specific views where this command is active
  // Empty/undefined means available in all views matching the scope
  viewStates?: ViewState[]

  // ModalStates lists specific modals where this command is active
  // undefined means no modal restriction
  // Use [ModalState.None] to only allow when no modal is open
  modalStates?: ModalState[]

  // ActionID is the unique identifier for this command's action
  actionId: string

  // IsAvailable checks if command should be active given current state
  // undefined means always available (within scope/view/modal constraints)
  isAvailable?: (executor: CommandExecutor) => boolean

  // Priority for display ordering (lower = higher priority in footer/help)
  priority: number
}
