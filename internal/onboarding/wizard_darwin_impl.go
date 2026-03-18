//go:build darwin && cgo

package onboarding

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// Forward declarations of Go callbacks (exported via //export in wizard_darwin_callbacks.go).
// CGo generates _cgo_export.h which provides their full declarations at link time.
extern void wizardGoMessage(const char *action, const char *payload);
extern void wizardGoActivation(void);

// Custom panel that accepts keyboard input.
// Standard NSPanel does not become the key window, blocking WKWebView keyboard events.
@interface _AgentPanel : NSPanel
@end

@implementation _AgentPanel
- (BOOL)canBecomeKeyWindow { return YES; }
- (BOOL)canBecomeMainWindow { return YES; }
@end

// ── Script message + window delegate ────────────────────────────────────────
@interface _AgentWizardDelegate : NSObject <WKScriptMessageHandler, NSWindowDelegate>
@end

@implementation _AgentWizardDelegate

- (void)userContentController:(WKUserContentController *)ucc
      didReceiveScriptMessage:(WKScriptMessage *)message {
    NSDictionary *body = (NSDictionary *)message.body;
    NSString *action  = body[@"action"]  ?: @"";
    NSString *payload = body[@"payload"] ?: @"";
    wizardGoMessage(action.UTF8String, payload.UTF8String);
}

- (void)windowWillClose:(NSNotification *)notification {
    // Closing state is managed on the Go side via gState.closed.
}

@end

// ── Module-level state ───────────────────────────────────────────────────────
static NSPanel              *_gPanel    = nil;
static WKWebView            *_gWebView  = nil;
static _AgentWizardDelegate *_gDelegate = nil;
static id                    _gActObs   = nil;

// ── C API ────────────────────────────────────────────────────────────────────

void wizard_show(const char *htmlCStr) {
    NSString *html = [NSString stringWithUTF8String:htmlCStr]; // copy BEFORE async — Go frees the C string on return
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_gPanel) { [_gPanel makeKeyAndOrderFront:nil]; return; }
        // use `html` — ARC retains it across the async boundary

        WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
        _gDelegate = [_AgentWizardDelegate new];
        [cfg.userContentController addScriptMessageHandler:_gDelegate name:@"agent"];

        NSRect frame = NSMakeRect(0, 0, 480, 520);

        _gWebView = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
        _gWebView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

        _gPanel = [[_AgentPanel alloc]
            initWithContentRect:frame
            styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable)
            backing:NSBackingStoreBuffered
            defer:NO];
        [_gPanel setTitle:@"Agent Daemon Setup"];
        [_gPanel setHidesOnDeactivate:NO];
        [_gPanel setLevel:NSFloatingWindowLevel];
        _gPanel.delegate = _gDelegate;
        [_gPanel setContentView:_gWebView];

        [_gWebView loadHTMLString:html baseURL:nil];
        [_gPanel center];
        [_gPanel makeKeyAndOrderFront:nil];
        [_gPanel makeFirstResponder:_gWebView];
        [NSApp activateIgnoringOtherApps:YES];
    });
}

void wizard_eval_js(const char *jsCStr) {
    NSString *js = [NSString stringWithUTF8String:jsCStr]; // copy BEFORE async — Go frees the C string on return
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!_gWebView) return;
        // use `js` — ARC retains it across the async boundary
        [_gWebView evaluateJavaScript:js completionHandler:nil];
    });
}

void wizard_close(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_gActObs) {
            [[NSNotificationCenter defaultCenter] removeObserver:_gActObs];
            _gActObs = nil;
        }
        [_gPanel close];
        _gPanel    = nil;
        _gWebView  = nil;
        _gDelegate = nil;
    });
}

// wizard_observe_activation registers an observer for NSApplicationDidBecomeActiveNotification.
// Uses NSNotificationCenter (not NSApplicationDelegate) to coexist safely with
// fyne.io/systray's existing delegate.
// Dispatched to the main queue to avoid a data race on _gActObs.
void wizard_observe_activation(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_gActObs) return;
        _gActObs = [[NSNotificationCenter defaultCenter]
            addObserverForName:NSApplicationDidBecomeActiveNotification
            object:nil
            queue:nil
            usingBlock:^(NSNotification *n) {
                wizardGoActivation();
            }];
    });
}
*/
import "C"

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

//go:embed wizard.html
var wizardHTMLBytes []byte

//go:embed fda-guide.png
var fdaGuidePNG []byte

// pollingActive prevents duplicate polling goroutines.
var pollingActive atomic.Bool

// showImpl is the darwin+cgo entry point for Show() in wizard.go.
func showImpl(s *state) {
	htmlStr := buildHTML(s)
	cHTML := C.CString(htmlStr)
	defer C.free(unsafe.Pointer(cHTML))
	C.wizard_show(cHTML)
	C.wizard_observe_activation()
	if !s.fdaGranted.Load() {
		go pollFDA(s)
	}
}

// buildHTML substitutes Go-side placeholders into wizard.html.
//   - {{FDA_GUIDE_DATA_URI}} → base64 PNG data URI
//   - {{ANTHROPIC_KEY}}      → HTML-escaped key (attribute context)
//   - {{OPENAI_KEY}}         → HTML-escaped key (attribute context)
//   - {{FDA_GRANTED}}        → "true"/"false" JS boolean
//   - {{NEEDS_API_KEY}}      → "true"/"false" — whether API key step is needed
//   - {{NEEDS_FDA}}          → "true"/"false" — whether FDA step is needed
//   - {{NEEDS_SERVICE}}      → "true"/"false" — whether service install step is needed
func buildHTML(s *state) string {
	pngDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fdaGuidePNG)
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	// HTML-escape keys: substituted into value="" attribute context.
	result := strings.NewReplacer(
		"{{FDA_GUIDE_DATA_URI}}", pngDataURI,
		"{{ANTHROPIC_KEY}}", html.EscapeString(s.anthropicKey),
		"{{OPENAI_KEY}}", html.EscapeString(s.openAIKey),
		"{{FDA_GRANTED}}", b(s.fdaGranted.Load()),
		"{{NEEDS_API_KEY}}", b(s.steps.NeedsAPIKey),
		"{{NEEDS_FDA}}", b(s.steps.NeedsFDA),
		"{{NEEDS_SERVICE}}", b(s.steps.NeedsService),
	).Replace(string(wizardHTMLBytes))
	return result
}

// pushJS evaluates a JavaScript expression in the WKWebView.
// Must be called from any goroutine — wizard_eval_js dispatches to main queue.
func pushJS(js string) {
	cJS := C.CString(js)
	defer C.free(unsafe.Pointer(cJS))
	C.wizard_eval_js(cJS)
}

// CheckFDA probes whether Full Disk Access has been granted to this process.
//
// Strategy: try to open each TCC-protected file.
//   - open() succeeds     → FDA granted, return true immediately
//   - EPERM / EACCES      → TCC blocked the access; FDA definitely not granted
//   - ENOENT / other      → file absent on this machine; try next probe
//
// If we exhaust every probe without a success, return false.
// Note: TCC changes made in System Settings may not propagate to an already-running
// process. If the user granted FDA but this still returns false, the wizard shows
// a manual "continue anyway" bypass so they can proceed without a restart.
func CheckFDA() bool {
	home, _ := os.UserHomeDir()

	probes := []string{
		// Safari bookmarks: always present if Safari has ever launched
		filepath.Join(home, "Library", "Safari", "Bookmarks.plist"),
		// Messages DB: present if Messages has been used
		filepath.Join(home, "Library", "Messages", "chat.db"),
		// Cookies: present on virtually all Macs
		filepath.Join(home, "Library", "Cookies", "Cookies.binarycookies"),
		// TimeMachine plist: in /Library, TCC-gated
		"/Library/Preferences/com.apple.TimeMachine.plist",
	}

	for _, p := range probes {
		f, err := os.Open(p)
		if err == nil {
			f.Close()
			return true // accessible → FDA granted
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			if pathErr.Err == syscall.EPERM || pathErr.Err == syscall.EACCES {
				// TCC explicitly denied access — FDA not granted.
				// No point trying other probes; the result will be the same.
				return false
			}
		}
		// ENOENT or other: file absent, try the next probe
	}
	return false
}

// pollFDA runs a 1-second polling loop until FDA is granted or the wizard closes.
// It is the backup mechanism — NSApplicationDidBecomeActive is the primary trigger
// (implemented in wizard_darwin_callbacks.go via //export wizardGoActivation).
func pollFDA(s *state) {
	if !pollingActive.CompareAndSwap(false, true) {
		return // already polling
	}
	defer pollingActive.Store(false)

	for {
		time.Sleep(1 * time.Second)
		if s.closed.Load() || s.fdaGranted.Load() {
			return
		}
		if CheckFDA() {
			s.fdaGranted.Store(true)
			pushJS(`window.dispatchEvent(new CustomEvent('fdaGranted'))`)
			return
		}
	}
}
