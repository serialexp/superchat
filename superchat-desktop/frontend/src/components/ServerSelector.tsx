// Server Selector Component
// Full-featured server selection screen with status probing and throttling

import { Component, createSignal, For, Show, onMount, createEffect } from 'solid-js'

export interface Server {
  name: string
  wsUrl: string
  wssUrl: string
  status: 'checking' | 'online' | 'offline'
  isSecure: boolean
}

interface ServerSelectorProps {
  onConnect: (url: string, nickname: string, throttleBps: number) => void
}

const THROTTLE_OPTIONS = [
  { value: 0, label: 'No limit' },
  { value: 1800, label: '14.4k modem' },
  { value: 3600, label: '28.8k modem' },
  { value: 4200, label: '33.6k modem' },
  { value: 7000, label: '56k modem' },
  { value: 16000, label: '128k ISDN' },
  { value: 32000, label: '256k DSL' },
  { value: 64000, label: '512k' },
  { value: 128000, label: '1Mbps' },
]

const ServerSelector: Component<ServerSelectorProps> = (props) => {
  const [servers, setServers] = createSignal<Server[]>([])
  const [selectedIndex, setSelectedIndex] = createSignal(-1)
  const [nickname, setNickname] = createSignal('')
  const [customUrl, setCustomUrl] = createSignal('')
  const [throttleSpeed, setThrottleSpeed] = createSignal(0)
  const [errorMessage, setErrorMessage] = createSignal('')

  onMount(() => {
    initializeServers()
  })

  const initializeServers = () => {
    const hostname = window.location.hostname || 'localhost'
    const initialServers: Server[] = []

    // Skip "Current Server" for Wails, localhost, wails.localhost, and empty hostnames
    const skipHostnames = ['wails', 'localhost', 'wails.localhost', '']
    if (!skipHostnames.includes(hostname)) {
      initialServers.push({
        name: 'Current Server',
        wsUrl: `ws://${hostname}:8080/ws`,
        wssUrl: `wss://${hostname}:8080/ws`,
        status: 'checking',
        isSecure: window.location.protocol === 'https:'
      })
    }

    // Only add superchat.win if we're not already on it
    if (hostname !== 'superchat.win') {
      initialServers.push({
        name: 'superchat.win',
        wsUrl: 'ws://superchat.win:8080/ws',
        wssUrl: 'wss://superchat.win:8080/ws',
        status: 'checking',
        isSecure: false
      })
    }

    // Always add Custom Server last
    initialServers.push({
      name: 'Custom Server',
      wsUrl: '',
      wssUrl: '',
      status: 'offline',
      isSecure: false
    })

    setServers(initialServers)
    loadFromLocalStorage()
    checkServerStatus()
  }

  const checkServerStatus = async () => {
    const serverList = servers()
    for (let i = 0; i < serverList.length - 1; i++) { // Skip custom server
      const server = serverList[i]

      // Try secure first, then insecure (matching web-client behavior)
      const urlsToTry = [server.wssUrl, server.wsUrl]
      let isOnline = false
      let secureWorks = false

      for (const url of urlsToTry) {
        try {
          const testWs = new WebSocket(url)

          await new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
              testWs.close()
              reject(new Error('timeout'))
            }, 3000)

            testWs.onopen = () => {
              clearTimeout(timeout)
              testWs.close()
              resolve(true)
            }

            testWs.onerror = () => {
              clearTimeout(timeout)
              reject(new Error('connection failed'))
            }
          })

          // Connection succeeded
          isOnline = true
          secureWorks = url === server.wssUrl
          break
        } catch (error) {
          // Try next URL
          continue
        }
      }

      // Update server status based on probe results
      setServers(prev => {
        const updated = [...prev]
        updated[i] = {
          ...updated[i],
          status: isOnline ? 'online' : 'offline',
          isSecure: secureWorks
        }
        return updated
      })
    }
  }

  const loadFromLocalStorage = () => {
    const savedNickname = localStorage.getItem('superchat_nickname')
    if (savedNickname) {
      setNickname(savedNickname)
    }

    const savedServerIndex = localStorage.getItem('superchat_server_index')
    if (savedServerIndex !== null) {
      const index = parseInt(savedServerIndex, 10)
      if (index >= 0 && index < servers().length) {
        setSelectedIndex(index)

        // Restore isSecure flag
        const savedServerSecure = localStorage.getItem('superchat_server_secure')
        if (savedServerSecure !== null) {
          setServers(prev => {
            const updated = [...prev]
            updated[index] = { ...updated[index], isSecure: savedServerSecure === 'true' }
            return updated
          })
        }

        // Restore custom URL if it was custom server
        if (index === servers().length - 1) {
          const savedCustomUrl = localStorage.getItem('superchat_custom_url')
          if (savedCustomUrl) {
            setCustomUrl(savedCustomUrl)
          }
        }
      }
    }

    const savedThrottle = localStorage.getItem('superchat_throttle_speed')
    if (savedThrottle !== null) {
      setThrottleSpeed(parseInt(savedThrottle, 10))
    }
  }

  const handleServerClick = (index: number) => {
    setSelectedIndex(index)
    const server = servers()[index]

    // For non-custom servers, populate the URL
    if (index < servers().length - 1) {
      const url = server.isSecure ? server.wssUrl : server.wsUrl
      setCustomUrl(url)
    } else {
      // Custom server - clear URL or use saved one
      const savedCustomUrl = localStorage.getItem('superchat_custom_url')
      setCustomUrl(savedCustomUrl || '')
    }
  }

  const toggleSecure = (index: number) => {
    setServers(prev => {
      const updated = [...prev]
      updated[index] = { ...updated[index], isSecure: !updated[index].isSecure }
      return updated
    })

    // Update URL display
    const server = servers()[index]
    if (index < servers().length - 1) {
      const url = server.isSecure ? server.wssUrl : server.wsUrl
      setCustomUrl(url)
    }
  }

  const handleConnect = (e: Event) => {
    e.preventDefault()

    if (!nickname().trim()) {
      setErrorMessage('Please enter a nickname')
      return
    }

    if (selectedIndex() === -1) {
      setErrorMessage('Please select a server')
      return
    }

    const server = servers()[selectedIndex()]
    let finalUrl = ''

    if (selectedIndex() === servers().length - 1) {
      // Custom server
      finalUrl = customUrl().trim()
      if (!finalUrl) {
        setErrorMessage('Please enter a server URL')
        return
      }
    } else {
      // Predefined server
      finalUrl = server.isSecure ? server.wssUrl : server.wsUrl
    }

    // Save to localStorage
    localStorage.setItem('superchat_nickname', nickname())
    localStorage.setItem('superchat_server_index', selectedIndex().toString())
    localStorage.setItem('superchat_server_secure', server.isSecure.toString())
    localStorage.setItem('superchat_throttle_speed', throttleSpeed().toString())

    if (selectedIndex() === servers().length - 1) {
      localStorage.setItem('superchat_custom_url', customUrl())
    }

    props.onConnect(finalUrl, nickname(), throttleSpeed())
  }

  // Update URL display when server or secure toggle changes
  createEffect(() => {
    const index = selectedIndex()
    if (index >= 0 && index < servers().length - 1) {
      const server = servers()[index]
      const url = server.isSecure ? server.wssUrl : server.wsUrl
      setCustomUrl(url)
    }
  })

  return (
    <div class="fixed inset-0 bg-black/95 flex items-center justify-center p-8 z-50">
      <div class="card bg-base-200 shadow-xl w-full max-w-lg">
        <div class="card-body">
          <h2 class="card-title text-2xl mb-4">Connect to SuperChat</h2>

          {/* Error Display */}
          <Show when={errorMessage()}>
            <div class="alert alert-error mb-4">
              <span>{errorMessage()}</span>
            </div>
          </Show>

          {/* Server List */}
          <fieldset class="fieldset mb-4">
            <legend class="fieldset-legend">Available Servers</legend>
            <div class="space-y-2">
              <For each={servers()}>
                {(server, index) => (
                  <div
                    onClick={() => handleServerClick(index())}
                    class={`flex items-center p-3 rounded-lg border-2 cursor-pointer transition-all ${
                      selectedIndex() === index()
                        ? 'border-primary bg-primary/10'
                        : 'border-base-300 hover:border-primary/50 hover:bg-base-300'
                    }`}
                  >
                    {/* Status Indicator */}
                    <div
                      class={`w-3 h-3 rounded-full mr-3 ${
                        server.status === 'online'
                          ? 'bg-success shadow-lg shadow-success/50'
                          : server.status === 'offline'
                          ? 'bg-error'
                          : 'bg-base-content/30 animate-pulse'
                      }`}
                    />

                    {/* Server Info */}
                    <div class="flex-1">
                      <div class="font-semibold">{server.name}</div>
                      <div class="text-xs text-base-content/60 font-mono">
                        {server.name === 'Custom Server'
                          ? 'Enter custom URL below'
                          : server.isSecure ? server.wssUrl : server.wsUrl}
                      </div>
                    </div>

                    {/* WS/WSS Toggle (for non-custom servers) */}
                    <Show when={index() < servers().length - 1}>
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          toggleSecure(index())
                        }}
                        class={`badge badge-sm ml-2 ${
                          server.isSecure ? 'badge-success' : 'badge-warning'
                        }`}
                      >
                        {server.isSecure ? 'WSS' : 'WS'}
                      </button>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </fieldset>

          {/* Connection Form */}
          <form onSubmit={handleConnect}>
            {/* Custom URL (only visible for custom server) */}
            <Show when={selectedIndex() === servers().length - 1}>
              <fieldset class="fieldset mb-4">
                <legend class="fieldset-legend">Server URL</legend>
                <input
                  type="text"
                  value={customUrl()}
                  onInput={(e) => setCustomUrl(e.currentTarget.value)}
                  placeholder="ws://localhost:8080/ws or wss://localhost:8080/ws"
                  class="input w-full"
                />
              </fieldset>
            </Show>

            {/* Nickname */}
            <fieldset class="fieldset mb-4">
              <legend class="fieldset-legend">Nickname</legend>
              <input
                type="text"
                value={nickname()}
                onInput={(e) => setNickname(e.currentTarget.value)}
                placeholder="Enter your nickname"
                class="input w-full"
                minLength={3}
                maxLength={32}
              />
            </fieldset>

            {/* Throttling */}
            <fieldset class="fieldset mb-6">
              <legend class="fieldset-legend">Connection Speed (optional)</legend>
              <select
                value={throttleSpeed()}
                onChange={(e) => setThrottleSpeed(parseInt(e.currentTarget.value, 10))}
                class="select w-full"
              >
                <For each={THROTTLE_OPTIONS}>
                  {(option) => (
                    <option value={option.value}>{option.label}</option>
                  )}
                </For>
              </select>
            </fieldset>

            {/* Connect Button */}
            <button
              type="submit"
              class="btn btn-primary w-full"
              disabled={!nickname().trim() || selectedIndex() === -1}
            >
              Connect
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}

export default ServerSelector
