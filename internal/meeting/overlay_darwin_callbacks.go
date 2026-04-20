    //go:build darwin && cgo

    package meeting

    import "C"

    import "sync"

    // overlayMu guards the callback slots below.
    var overlayMu sync.Mutex

    // At most one overlay prompt is active at a time.
    var (
    	overlayRecordCB    func(bool)  // set by MeetingDetected
    	overlayTranscribeCB func(bool) // set by RecordingReady
    	overlayOpenPath    string       // set by TranscriptReady
    	overlayCurrentApp  string       // app name during recording
    )

    //export overlayGoAction
    func overlayGoAction(actionCStr *C.char) {
    	action := C.GoString(actionCStr)

    	overlayMu.Lock()
    	recordCB    := overlayRecordCB
    	transcribeCB := overlayTranscribeCB
    	openPath    := overlayOpenPath
    	overlayMu.Unlock()

    	switch action {
    	case "record":
    		if recordCB != nil {
    			overlayMu.Lock(); overlayRecordCB = nil; overlayMu.Unlock()
    			recordCB(true)
    		}
    	case "dismiss":
    		if recordCB != nil {
    			overlayMu.Lock(); overlayRecordCB = nil; overlayMu.Unlock()
    			recordCB(false)
    		} else if transcribeCB != nil {
    			overlayMu.Lock(); overlayTranscribeCB = nil; overlayMu.Unlock()
    			transcribeCB(false)
    		}
    		go overlayHide()
    	case "transcribe":
    		if transcribeCB != nil {
    			overlayMu.Lock(); overlayTranscribeCB = nil; overlayMu.Unlock()
    			transcribeCB(true)
    		}
    	case "later":
    		if transcribeCB != nil {
    			overlayMu.Lock(); overlayTranscribeCB = nil; overlayMu.Unlock()
    			transcribeCB(false)
    		}
    		go overlayHide()
    	case "open":
    		if openPath != "" {
    			p := openPath
    			overlayMu.Lock(); overlayOpenPath = ""; overlayMu.Unlock()
    			go overlayOpenFile(p)
    		}
    		go overlayHide()
    	case "open_settings":
    		go overlayOpenFile("x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")
    	}
    }
    