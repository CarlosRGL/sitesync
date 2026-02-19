package sync

// EventType identifies what kind of engine event was emitted.
type EventType uint8

const (
	// EvStepStart signals that a step has begun.
	EvStepStart EventType = iota
	// EvStepDone signals that a step completed successfully.
	EvStepDone
	// EvStepFail signals that a step failed; Message contains the error.
	// The engine blocks on ReplyCh waiting for an ErrorAction response.
	EvStepFail
	// EvLog is a single line of output from a subprocess.
	EvLog
	// EvProgress is a progress update (Progress field is 0.0–1.0).
	EvProgress
	// EvDone signals that the entire sync run has finished.
	EvDone
)

// ErrorAction tells the engine what to do after a step failure.
type ErrorAction uint8

const (
	ActionRetry    ErrorAction = iota // Re-run the failed step
	ActionContinue                    // Skip the failed step, continue to next
	ActionQuit                        // Abort the entire sync
)

// Event is the message type passed from the sync engine to the TUI.
type Event struct {
	Type     EventType
	Step     int     // 1–7, or 0 for EvDone
	Message  string  // log text for EvLog; error text for EvStepFail
	Progress float64 // 0.0–1.0 for EvProgress

	// ReplyCh is set on EvStepFail events. The consumer must send exactly
	// one ErrorAction to tell the engine how to proceed.
	ReplyCh chan<- ErrorAction
}

// Op describes which parts of the sync to run.
type Op uint8

const (
	OpAll   Op = iota // SQL + files
	OpSQL             // database only
	OpFiles           // files only
)

// StepName returns
// a human-readable name for each step (1-indexed).
func StepName(step int) string {
	names := [...]string{
		"",
		"Fetch SQL dump",
		"Find / Replace",
		"Before hooks",
		"Import SQL",
		"Between hooks",
		"Sync files",
		"After hooks",
	}
	if step < 1 || step > 7 {
		return "Unknown"
	}
	return names[step]
}
