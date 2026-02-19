/**
 * WebSocket client for real-time communication with the Go backend.
 * Replaces Wails event system in web mode.
 */

let ws = null
let reconnectTimer = null
const listeners = new Map() // event -> Set<callback>

function getWsUrl() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${location.host}/ws`
}

export function connectWebSocket() {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
        return
    }

    ws = new WebSocket(getWsUrl())

    ws.onopen = () => {
        console.log('[ws] connected')
        if (reconnectTimer) {
            clearTimeout(reconnectTimer)
            reconnectTimer = null
        }
    }

    ws.onmessage = (evt) => {
        try {
            const msg = JSON.parse(evt.data)
            if (msg.event) {
                dispatch(msg.event, msg.data)
            }
        } catch (e) {
            console.warn('[ws] parse error:', e)
        }
    }

    ws.onclose = () => {
        console.log('[ws] disconnected, reconnecting in 3s...')
        scheduleReconnect()
    }

    ws.onerror = () => {
        ws.close()
    }
}

function scheduleReconnect() {
    if (!reconnectTimer) {
        reconnectTimer = setTimeout(() => {
            reconnectTimer = null
            connectWebSocket()
        }, 3000)
    }
}

function dispatch(event, data) {
    const cbs = listeners.get(event)
    if (cbs) {
        for (const cb of cbs) {
            try {
                cb(data)
            } catch (e) {
                console.error(`[ws] handler error for "${event}":`, e)
            }
        }
    }
}

export function onWsEvent(event, callback) {
    if (!listeners.has(event)) {
        listeners.set(event, new Set())
    }
    listeners.get(event).add(callback)
}

export function offWsEvent(event, callback) {
    if (!callback) {
        listeners.delete(event)
    } else {
        const cbs = listeners.get(event)
        if (cbs) {
            cbs.delete(callback)
        }
    }
}

export function sendWsMessage(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(msg))
    }
}
