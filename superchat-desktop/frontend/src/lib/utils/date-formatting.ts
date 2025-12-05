// Date and time formatting utilities for messages

/**
 * Format a Unix timestamp (milliseconds) to a readable time string
 * - If today: "3:45 PM"
 * - If this year: "Jan 15, 3:45 PM"
 * - If older: "Jan 15, 2024 3:45 PM"
 */
export function formatMessageTime(timestampMs: bigint): string {
  const date = new Date(Number(timestampMs))
  const now = new Date()

  const isToday = isSameDay(date, now)
  const isThisYear = date.getFullYear() === now.getFullYear()

  if (isToday) {
    // Just show time
    return date.toLocaleTimeString('en-US', {
      hour: 'numeric',
      minute: '2-digit',
      hour12: true
    })
  } else if (isThisYear) {
    // Show date + time, omit year
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      hour12: true
    })
  } else {
    // Show full date + time
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      hour12: true
    })
  }
}

/**
 * Format a date for a separator
 * - If today: "Today"
 * - If yesterday: "Yesterday"
 * - If this year: "Monday, January 15"
 * - If older: "Monday, January 15, 2024"
 */
export function formatDateSeparator(timestampMs: bigint): string {
  const date = new Date(Number(timestampMs))
  const now = new Date()

  if (isSameDay(date, now)) {
    return 'Today'
  }

  const yesterday = new Date(now)
  yesterday.setDate(yesterday.getDate() - 1)
  if (isSameDay(date, yesterday)) {
    return 'Yesterday'
  }

  const isThisYear = date.getFullYear() === now.getFullYear()

  if (isThisYear) {
    return date.toLocaleDateString('en-US', {
      weekday: 'long',
      month: 'long',
      day: 'numeric'
    })
  } else {
    return date.toLocaleDateString('en-US', {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
      year: 'numeric'
    })
  }
}

/**
 * Check if two dates are on the same day
 */
export function isSameDay(date1: Date, date2: Date): boolean {
  return (
    date1.getFullYear() === date2.getFullYear() &&
    date1.getMonth() === date2.getMonth() &&
    date1.getDate() === date2.getDate()
  )
}

/**
 * Check if a date separator should be shown between two messages
 */
export function shouldShowDateSeparator(
  prevTimestampMs: bigint | null,
  currentTimestampMs: bigint
): boolean {
  if (prevTimestampMs === null) {
    return true // Always show separator for first message
  }

  const prevDate = new Date(Number(prevTimestampMs))
  const currentDate = new Date(Number(currentTimestampMs))

  return !isSameDay(prevDate, currentDate)
}
