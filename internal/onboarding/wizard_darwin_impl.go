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
    dispatch_async(dispatch_get_main_queue(), ^{
        if (_gPanel) { [_gPanel makeKeyAndOrderFront:nil]; return; }

        NSString *html = [NSString stringWithUTF8String:htmlCStr];

        WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
        _gDelegate = [_AgentWizardDelegate new];
        [cfg.userContentController addScriptMessageHandler:_gDelegate name:@"agent"];

        NSRect frame = NSMakeRect(0, 0, 480, 520);

        _gWebView = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
        _gWebView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

        _gPanel = [[NSPanel alloc]
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
        [NSApp activateIgnoringOtherApps:YES];
    });
}

void wizard_eval_js(const char *jsCStr) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!_gWebView) return;
        NSString *js = [NSString stringWithUTF8String:jsCStr];
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
void wizard_observe_activation(void) {
    if (_gActObs) return;
    _gActObs = [[NSNotificationCenter defaultCenter]
        addObserverForName:NSApplicationDidBecomeActiveNotification
        object:nil
        queue:nil
        usingBlock:^(NSNotification *n) {
            wizardGoActivation();
        }];
}
*/
import "C"

import (
	_ "embed"
	"encoding/base64"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	if !s.fdaGranted {
		go pollFDA(s)
	}
}

// buildHTML substitutes Go-side placeholders into wizard.html.
//   - {{FDA_GUIDE_DATA_URI}}  → base64-encoded PNG as a data: URI
//   - {{ANTHROPIC_KEY}}       → HTML-escaped key (attribute context)
//   - {{OPENAI_KEY}}          → HTML-escaped key (attribute context)
//   - {{FDA_GRANTED}}         → "true" or "false" (JS boolean literal)
func buildHTML(s *state) string {
	pngDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fdaGuidePNG)
	fdaVal := "false"
	if s.fdaGranted {
		fdaVal = "true"
	}
	// HTML-escape keys: they are substituted into value="" attribute context.
	result := strings.NewReplacer(
		"{{FDA_GUIDE_DATA_URI}}", pngDataURI,
		"{{ANTHROPIC_KEY}}", html.EscapeString(s.anthropicKey),
		"{{OPENAI_KEY}}", html.EscapeString(s.openAIKey),
		"{{FDA_GRANTED}}", fdaVal,
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
// Uses the MacPaw PermissionsKit probe strategy: attempt to open a TCC-protected
// file and check for EPERM/EACCES vs success.
func CheckFDA() bool {
	home, _ := os.UserHomeDir()
	if f, err := os.Open(filepath.Join(home, "Library", "Safari", "Bookmarks.plist")); err == nil {
		f.Close()
		return true
	}
	// Fallback for users without Safari installed.
	if f, err := os.Open("/Library/Preferences/com.apple.TimeMachine.plist"); err == nil {
		f.Close()
		return true
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
		if s.closed || s.fdaGranted {
			return
		}
		if CheckFDA() {
			s.fdaGranted = true
			pushJS(`window.dispatchEvent(new CustomEvent('fdaGranted'))`)
			return
		}
	}
}
