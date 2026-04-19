//go:build darwin && cgo

    package meeting

    /*
    #cgo CFLAGS: -x objective-c -fobjc-arc
    #cgo LDFLAGS: -framework Cocoa -framework WebKit

    #import <Cocoa/Cocoa.h>
    #import <WebKit/WebKit.h>

    extern void overlayGoAction(const char *action);

    // ── Delegate ─────────────────────────────────────────────────────────────────

    @interface _OverlayDelegate : NSObject <WKScriptMessageHandler>
    @end

    @implementation _OverlayDelegate
    - (void)userContentController:(WKUserContentController *)ucc
          didReceiveScriptMessage:(WKScriptMessage *)msg {
        NSString *action = (NSString *)msg.body ?: @"";
        overlayGoAction(action.UTF8String);
    }
    @end

    // ── Module state ──────────────────────────────────────────────────────────────

    static NSPanel          *_gOverlay  = nil;
    static WKWebView        *_gWebView  = nil;
    static _OverlayDelegate *_gODel     = nil;

    // ── HTML ──────────────────────────────────────────────────────────────────────

    static NSString *overlayHTML = @"<!DOCTYPE html>"
    "<html><head><meta charset='utf-8'><style>"
    "*{margin:0;padding:0;box-sizing:border-box}"
    "html,body{background:transparent;width:100%;height:100%;overflow:hidden;"
    "font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text',sans-serif;"
    "-webkit-font-smoothing:antialiased}"
    "#card{position:absolute;top:12px;right:20px;left:20px;"
    "background:rgba(28,28,30,0.93);"
    "backdrop-filter:blur(24px) saturate(180%);"
    "-webkit-backdrop-filter:blur(24px) saturate(180%);"
    "border-radius:14px;border:0.5px solid rgba(255,255,255,0.1);"
    "box-shadow:0 4px 20px rgba(0,0,0,0.45);"
    "padding:13px 15px;"
    "transform:translateX(120%) scale(0.95);opacity:0;"
    "transition:transform 0.3s cubic-bezier(0.34,1.56,0.64,1),opacity 0.25s ease}"
    "#card.on{transform:translateX(0) scale(1);opacity:1}"
    ".row{display:flex;align-items:center;gap:9px}"
    ".dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}"
    ".green{background:#30d158}.red{background:#ff453a;animation:p 1.2s ease-in-out infinite}"
    ".yellow{background:#ffd60a}.blue{background:#0a84ff}"
    "@keyframes p{0%,100%{opacity:1;transform:scale(1)}50%{opacity:0.55;transform:scale(0.8)}}"
    ".txt{flex:1;min-width:0}"
    ".t{color:rgba(255,255,255,0.95);font-size:13px;font-weight:600;line-height:1.3;"
    "white-space:nowrap;overflow:hidden;text-overflow:ellipsis}"
    ".s{color:rgba(255,255,255,0.42);font-size:11px;margin-top:2px}"
    ".timer{font-variant-numeric:tabular-nums;color:rgba(255,255,255,0.45);font-size:12px;font-weight:500}"
    ".btns{display:flex;gap:6px;margin-top:10px}"
    "button{flex:1;padding:6px 10px;border-radius:8px;font-size:12px;font-weight:500;"
    "cursor:pointer;border:none;font-family:inherit;transition:opacity 0.1s}"
    "button:active{opacity:0.7}"
    ".p{background:rgba(10,132,255,0.9);color:#fff}"
    ".sec{background:rgba(255,255,255,0.1);color:rgba(255,255,255,0.72)}"
    "</style></head><body>"
    "<div id='card'>"
    "<div class='row'><div class='dot' id='dot'></div>"
    "<div class='txt'><div class='t' id='ti'></div><div class='s' id='su'></div></div>"
    "<div class='timer' id='tm'></div></div>"
    "<div class='btns' id='bt'></div></div>"
    "<script>"
    "var iv=null,sec=0;"
    "function s(a){window.webkit.messageHandlers.loom.postMessage(a)}"
    "function setState(d){"
    "clearInterval(iv);iv=null;sec=0;"
    "document.getElementById('tm').textContent='';"
    "document.getElementById('bt').innerHTML='';"
    "if(!d){document.getElementById('card').classList.remove('on');return}"
    "document.getElementById('ti').textContent=d.title||'';"
    "document.getElementById('su').textContent=d.sub||'';"
    "document.getElementById('dot').className='dot '+(d.dot||'green');"
    "if(d.timer){iv=setInterval(function(){"
    "sec++;var m=Math.floor(sec/60),s=sec%60;"
    "document.getElementById('tm').textContent=m+':'+(s<10?'0':'')+s"
    "},1000)}"
    "(d.buttons||[]).forEach(function(b){"
    "var el=document.createElement('button');"
    "el.textContent=b.label;el.className=b.p?'p':'sec';"
    "el.onclick=function(){s(b.a)};document.getElementById('bt').appendChild(el)"
    "});"
    "document.getElementById('card').classList.add('on')"
    "}"
    "</script></body></html>";

    // ── C API ─────────────────────────────────────────────────────────────────────

    void overlay_ensure_created(void) {
        if (_gOverlay) return;

        NSRect screen = [NSScreen mainScreen].visibleFrame;
        CGFloat w = 440, h = 152;
        NSRect frame = NSMakeRect(
            NSMaxX(screen) - w - 8,
            NSMaxY(screen) - h - 8,
            w, h
        );

        _gOverlay = [[NSPanel alloc]
            initWithContentRect:frame
            styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
            backing:NSBackingStoreBuffered
            defer:NO];

        _gOverlay.level               = NSFloatingWindowLevel;
        _gOverlay.opaque              = NO;
        _gOverlay.backgroundColor     = [NSColor clearColor];
        _gOverlay.hasShadow           = NO;
        // ignoresMouseEvents intentionally NOT set — NSNonactivatingPanel handles focus correctly
        _gOverlay.collectionBehavior  =
            NSWindowCollectionBehaviorCanJoinAllSpaces |
            NSWindowCollectionBehaviorStationary       |
            NSWindowCollectionBehaviorIgnoresCycle;
        [_gOverlay setAnimationBehavior:NSWindowAnimationBehaviorNone];

        // WKWebView
        WKWebViewConfiguration *cfg = [[WKWebViewConfiguration alloc] init];
        [cfg.userContentController addScriptMessageHandler:
            (_gODel = [[_OverlayDelegate alloc] init]) name:@"loom"];

        _gWebView = [[WKWebView alloc] initWithFrame:NSMakeRect(0,0,w,h) configuration:cfg];
        [_gWebView setValue:@NO forKey:@"drawsBackground"];
        _gOverlay.contentView = _gWebView;

        [_gWebView loadHTMLString:overlayHTML baseURL:nil];
    }

    void overlay_set_state(const char *jsonCStr) {
        NSString *json = [NSString stringWithUTF8String:jsonCStr];
        dispatch_async(dispatch_get_main_queue(), ^{
            overlay_ensure_created();
            NSString *js = [NSString stringWithFormat:@"setState(%@)", json];
            [_gWebView evaluateJavaScript:js completionHandler:nil];
            [_gOverlay orderFrontRegardless];
        });
    }

    void overlay_hide_c(void) {
        dispatch_async(dispatch_get_main_queue(), ^{
            if (!_gOverlay) return;
            NSString *js = @"setState(null)";
            [_gWebView evaluateJavaScript:js completionHandler:nil];
            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 300*NSEC_PER_MSEC),
                dispatch_get_main_queue(), ^{
                    [_gOverlay orderOut:nil];
                });
        });
    }

    void overlay_set_mouse(int ignore) {
        dispatch_async(dispatch_get_main_queue(), ^{
            _gOverlay.ignoresMouseEvents = (BOOL)ignore;
        });
    }

    void overlay_open(const char *pathCStr) {
        NSString *path = [NSString stringWithUTF8String:pathCStr];
        dispatch_async(dispatch_get_main_queue(), ^{
            [[NSWorkspace sharedWorkspace] openURL:[NSURL fileURLWithPath:path]];
        });
    }
    */
    import "C"

    import (
    	"encoding/json"
    	"fmt"
    	"path/filepath"
    	"unsafe"
    )

    // ── Go-side helpers ───────────────────────────────────────────────────────────

    type overlayState struct {
    	Title   string          `json:"title"`
    	Sub     string          `json:"sub,omitempty"`
    	Dot     string          `json:"dot"`
    	Timer   bool            `json:"timer,omitempty"`
    	Buttons []overlayButton `json:"buttons,omitempty"`
    }

    type overlayButton struct {
    	Label   string `json:"label"`
    	Action  string `json:"a"`
    	Primary bool   `json:"p,omitempty"`
    }

    func showOverlay(s overlayState) {
    	data, _ := json.Marshal(s)
    	cs := C.CString(string(data))
    	defer C.free(unsafe.Pointer(cs))
    	C.overlay_set_state(cs)
    }

    func overlayHide() {
    	C.overlay_hide_c()
    }

    func overlaySetMouseEvents(ignore bool) {
    	if ignore {
    		C.overlay_set_mouse(C.int(1))
    	} else {
    		C.overlay_set_mouse(C.int(0))
    	}
    }

    func overlayOpenFile(path string) {
    	cs := C.CString(path)
    	defer C.free(unsafe.Pointer(cs))
    	C.overlay_open(cs)
    }

    // ── Notifier implementation ───────────────────────────────────────────────────

    // NewNotifier returns the floating NSPanel HUD notifier.
    func NewNotifier() Notifier { return &overlayNotifier{} }

    type overlayNotifier struct{}

    func (n *overlayNotifier) Setup() {} // overlay is created lazily on first show

    func (n *overlayNotifier) MeetingDetected(app string, callback func(bool)) {
    	overlayMu.Lock()
    	overlayCurrentApp = app
    	overlayRecordCB = func(record bool) {
    		if record {
    			// Immediately transition to recording state
    			showOverlay(overlayState{
    				Title: fmt.Sprintf("Recording %s", app),
    				Dot:   "red",
    				Timer: true,
    			})
    		}
    		callback(record)
    	}
    	overlayMu.Unlock()

    	showOverlay(overlayState{
    		Title: fmt.Sprintf("%s meeting detected", app),
    		Sub:   "Record and transcribe with Whisper?",
    		Dot:   "green",
    		Buttons: []overlayButton{
    			{Label: "Record & Transcribe", Action: "record", Primary: true},
    			{Label: "Dismiss", Action: "dismiss"},
    		},
    	})
    }

    func (n *overlayNotifier) RecordingReady(wavPath string, durationSec int, callback func(bool)) {
    	overlayMu.Lock()
    	overlayTranscribeCB = callback
    	overlayMu.Unlock()

    	mins := durationSec / 60
    	secs := durationSec % 60
    	showOverlay(overlayState{
    		Title: "Recording saved",
    		Sub:   fmt.Sprintf("%dm %ds · Transcribe with Whisper?", mins, secs),
    		Dot:   "yellow",
    		Buttons: []overlayButton{
    			{Label: "Transcribe", Action: "transcribe", Primary: true},
    			{Label: "Save for Later", Action: "later"},
    		},
    	})
    }

    func (n *overlayNotifier) Transcribing() {
    	showOverlay(overlayState{
    		Title: "Transcribing\u2026",
    		Sub:   "This usually takes under a minute",
    		Dot:   "blue",
    	})
    }

    func (n *overlayNotifier) TranscriptReady(mdPath string) {
    	overlayMu.Lock()
    	overlayOpenPath = mdPath
    	overlayMu.Unlock()

    	showOverlay(overlayState{
    		Title: "Transcript ready",
    		Sub:   filepath.Base(mdPath),
    		Dot:   "green",
    		Buttons: []overlayButton{
    			{Label: "Open File", Action: "open", Primary: true},
    			{Label: "Dismiss", Action: "dismiss"},
    		},
    	})
    }
    