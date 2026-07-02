package hakkacode

import "hakka-code/internal/hakkacode/protocol"

// Re-export protocol types for backward compatibility. New code should
// import protocol directly.

type (
	Command       = protocol.Command
	RequestFrame  = protocol.RequestFrame
	TurnStats     = protocol.TurnStats
	ResponseFrame = protocol.ResponseFrame
	SessionSummary = protocol.SessionSummary
)

var sessionSummaryFromMap = protocol.SessionSummaryFromMap
