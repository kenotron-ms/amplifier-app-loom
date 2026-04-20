    //go:build !darwin

    package meeting

    // NewNotifier returns a stub notifier that auto-accepts all prompts.
    func NewNotifier() Notifier { return &stubNotifier{} }

    type stubNotifier struct{}

    func (n *stubNotifier) Setup() {}

    func (n *stubNotifier) MeetingDetected(_ string, callback func(bool)) {
    	go callback(true)
    }

    func (n *stubNotifier) RecordingReady(_ string, _ int, callback func(bool)) {
    	go callback(true)
    }

    func (n *stubNotifier) Transcribing() {}

    func (n *stubNotifier) TranscriptReady(_ string) {}
    