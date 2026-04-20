package meeting

    // State represents the current phase of the meeting Service.
    type State int

    const (
    	StateIdle        State = iota
    	StateMonitoring
    	StateRecording
    	StateTranscribing
    )

    func (s State) String() string {
    	switch s {
    	case StateIdle:
    		return "idle"
    	case StateMonitoring:
    		return "monitoring"
    	case StateRecording:
    		return "recording"
    	case StateTranscribing:
    		return "transcribing"
    	}
    	return "unknown"
    }
    