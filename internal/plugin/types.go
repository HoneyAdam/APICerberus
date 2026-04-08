package plugin

// Phase identifies plugin execution stage.
type Phase string

const (
	PhasePreAuth   Phase = "pre-auth"
	PhaseAuth      Phase = "auth"
	PhasePreProxy  Phase = "pre-proxy"
	PhaseProxy     Phase = "proxy"
	PhasePostProxy Phase = "post-proxy"
)
