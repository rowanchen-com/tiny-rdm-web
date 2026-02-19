package api

// EmitEvent sends an event to all connected WebSocket clients.
// This replaces Wails' runtime.EventsEmit for the web version.
func EmitEvent(event string, data any) {
	Hub().Emit(event, data)
}
