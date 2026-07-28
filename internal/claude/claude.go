package claude

import "context"

// RunOptions configures a Claude CLI invocation.
type RunOptions struct {
	Model                 string
	MaxTurns              int
	DangerSkipPermissions bool
	Dir                   string        // working directory for the subprocess; empty = inherit parent
	Steer                 <-chan string // optional: operator messages written to the agent's stdin mid-turn
}

// Agent is the interface for AI code agents. Claude is the default
// implementation; OpenAI and Gemini are future implementations.
type Agent interface {
	// Run starts the agent with the given prompt and streams events back
	// on the returned channel. The channel is closed when the agent exits.
	Run(ctx context.Context, prompt string, opts RunOptions) (<-chan Event, error)
}
