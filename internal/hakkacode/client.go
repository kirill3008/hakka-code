package hakkacode

import "hakka-code/internal/hakkacode/backend"

// Re-export Client and Dial so old references still compile. New code
// should import backend directly.

// Client is the WebSocket transport.
type Client = backend.Client

// Dial connects to the Hakka WebSocket server.
var Dial = backend.Dial
