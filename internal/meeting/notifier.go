    package meeting

    // Notifier sends native OS notifications for meeting lifecycle events.
    // The darwin implementation uses UNUserNotificationCenter.
    // The stub (non-darwin) auto-accepts all prompts.
    type Notifier interface {
    	// Setup registers notification categories and requests permission.
    	// Must be called once from the main thread before any other call.
    	Setup()

    	// MeetingDetected fires when side-huddle detects a meeting.
    	// callback receives true if the user chose "Record & Transcribe".
    	MeetingDetected(app string, callback func(record bool))

    	// RecordingReady fires when the WAV file is written.
    	// callback receives true if the user chose "Transcribe".
    	RecordingReady(wavPath string, durationSec int, callback func(transcribe bool))

    	// Transcribing sends an informational banner (no actions).
    	Transcribing()

    	// TranscriptReady fires when the .md file is ready.
    	// Offers an "Open File" action.
    	TranscriptReady(mdPath string)
    }
    