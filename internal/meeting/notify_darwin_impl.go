    //go:build darwin && cgo

    package meeting

    /*
    #cgo CFLAGS: -x objective-c -fobjc-arc
    #cgo LDFLAGS: -framework UserNotifications -framework Foundation -framework AppKit

    #import <UserNotifications/UserNotifications.h>
    #import <Foundation/Foundation.h>
    #import <AppKit/AppKit.h>

    extern void notifGoAction(const char *notifID, const char *actionID);

    @interface _MeetingNotifDelegate : NSObject <UNUserNotificationCenterDelegate>
    @end

    @implementation _MeetingNotifDelegate

    - (void)userNotificationCenter:(UNUserNotificationCenter *)center
           willPresentNotification:(UNNotification *)notification
             withCompletionHandler:(void (^)(UNNotificationPresentationOptions))handler {
        handler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
    }

    - (void)userNotificationCenter:(UNUserNotificationCenter *)center
        didReceiveNotificationResponse:(UNNotificationResponse *)response
                 withCompletionHandler:(void (^)(void))handler {
        notifGoAction(
            response.notification.request.identifier.UTF8String,
            response.actionIdentifier.UTF8String
        );
        handler();
    }

    @end

    static _MeetingNotifDelegate *_gDelegate = nil;

    void meeting_notif_setup(void) {
        dispatch_async(dispatch_get_main_queue(), ^{
            // UNUserNotificationCenter requires a proper .app bundle.
            // Skip silently when running as a raw binary (no bundle identifier).
            if (![[NSBundle mainBundle] bundleIdentifier]) {
                NSLog(@"meeting: UNUserNotificationCenter unavailable (no bundle ID)");
                return;
            }
            _gDelegate = [[_MeetingNotifDelegate alloc] init];
            UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
            [center setDelegate:_gDelegate];

            UNNotificationAction *recordAction = [UNNotificationAction
                actionWithIdentifier:@"RECORD"
                title:@"Record & Transcribe"
                options:UNNotificationActionOptionForeground];
            UNNotificationAction *dismissAction = [UNNotificationAction
                actionWithIdentifier:@"DISMISS"
                title:@"Dismiss"
                options:UNNotificationActionOptionDestructive];
            UNNotificationCategory *detectCat = [UNNotificationCategory
                categoryWithIdentifier:@"MEETING_DETECTED"
                actions:@[recordAction, dismissAction]
                intentIdentifiers:@[]
                options:UNNotificationCategoryOptionCustomDismissAction];

            UNNotificationAction *transcribeAction = [UNNotificationAction
                actionWithIdentifier:@"TRANSCRIBE"
                title:@"Transcribe"
                options:UNNotificationActionOptionForeground];
            UNNotificationAction *laterAction = [UNNotificationAction
                actionWithIdentifier:@"LATER"
                title:@"Save for Later"
                options:0];
            UNNotificationCategory *readyCat = [UNNotificationCategory
                categoryWithIdentifier:@"RECORDING_READY"
                actions:@[transcribeAction, laterAction]
                intentIdentifiers:@[]
                options:UNNotificationCategoryOptionCustomDismissAction];

            UNNotificationAction *openAction = [UNNotificationAction
                actionWithIdentifier:@"OPEN"
                title:@"Open File"
                options:UNNotificationActionOptionForeground];
            UNNotificationCategory *doneCat = [UNNotificationCategory
                categoryWithIdentifier:@"TRANSCRIPT_READY"
                actions:@[openAction]
                intentIdentifiers:@[]
                options:UNNotificationCategoryOptionCustomDismissAction];

            [center setNotificationCategories:[NSSet setWithObjects:detectCat, readyCat, doneCat, nil]];
            [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                completionHandler:^(BOOL granted, NSError *error) {
                    if (!granted) NSLog(@"meeting: notification permission denied");
                }];
        });
    }

    void meeting_notif_send(const char *identifier, const char *title, const char *body, const char *categoryID) {
        NSString *nsID    = [NSString stringWithUTF8String:identifier];
        NSString *nsTitle = [NSString stringWithUTF8String:title];
        NSString *nsBody  = [NSString stringWithUTF8String:body];
        NSString *nsCat   = categoryID ? [NSString stringWithUTF8String:categoryID] : nil;

        dispatch_async(dispatch_get_main_queue(), ^{
            if (![[NSBundle mainBundle] bundleIdentifier]) { return; }
            UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
            content.title = nsTitle;
            content.body  = nsBody;
            content.sound = UNNotificationSound.defaultSound;
            if (nsCat) {
                content.categoryIdentifier = nsCat;
            }
            UNNotificationRequest *req = [UNNotificationRequest
                requestWithIdentifier:nsID
                content:content
                trigger:nil];
            [[UNUserNotificationCenter currentNotificationCenter]
                addNotificationRequest:req
                withCompletionHandler:^(NSError *e) {
                    if (e) NSLog(@"meeting: notif error: %@", e);
                }];
        });
    }

    void meeting_notif_open_file(const char *pathCStr) {
        NSString *path = [NSString stringWithUTF8String:pathCStr];
        dispatch_async(dispatch_get_main_queue(), ^{
            NSURL *url = [NSURL fileURLWithPath:path];
            [[NSWorkspace sharedWorkspace] openURL:url];
        });
    }
    */
    import "C"

    import (
    	"fmt"
    	"path/filepath"
    	"sync/atomic"
    	"unsafe"
    )

    // NewNotifier returns a darwin UNUserNotificationCenter-backed Notifier.
    func NewNotifier() Notifier { return &darwinNotifier{} }

    type darwinNotifier struct{}

    var notifCounter atomic.Uint64

    func nextNotifID() string {
    	return fmt.Sprintf("loom-meeting-%d", notifCounter.Add(1))
    }

    func (n *darwinNotifier) Setup() {
    	C.meeting_notif_setup()
    }

    func (n *darwinNotifier) MeetingDetected(app string, callback func(bool)) {
    	id := nextNotifID()
    	title := fmt.Sprintf("%s meeting detected", app)

    	notifMu.Lock()
    	notifCallbacks[id] = func(action string) { callback(action == "RECORD") }
    	notifMu.Unlock()

    	cID    := C.CString(id)
    	cTitle := C.CString(title)
    	cBody  := C.CString("Record and transcribe with Whisper?")
    	cCat   := C.CString("MEETING_DETECTED")
    	defer C.free(unsafe.Pointer(cID))
    	defer C.free(unsafe.Pointer(cTitle))
    	defer C.free(unsafe.Pointer(cBody))
    	defer C.free(unsafe.Pointer(cCat))

    	C.meeting_notif_send(cID, cTitle, cBody, cCat)
    }

    func (n *darwinNotifier) RecordingReady(wavPath string, durationSec int, callback func(bool)) {
    	id := nextNotifID()
    	mins := durationSec / 60
    	secs := durationSec % 60
    	title := fmt.Sprintf("Recording saved · %dm %ds", mins, secs)

    	notifMu.Lock()
    	notifCallbacks[id] = func(action string) { callback(action == "TRANSCRIBE") }
    	notifMu.Unlock()

    	cID    := C.CString(id)
    	cTitle := C.CString(title)
    	cBody  := C.CString("Transcribe now with whisper-1?")
    	cCat   := C.CString("RECORDING_READY")
    	defer C.free(unsafe.Pointer(cID))
    	defer C.free(unsafe.Pointer(cTitle))
    	defer C.free(unsafe.Pointer(cBody))
    	defer C.free(unsafe.Pointer(cCat))

    	C.meeting_notif_send(cID, cTitle, cBody, cCat)
    }

    func (n *darwinNotifier) Transcribing() {
    	id   := nextNotifID()
    	cID  := C.CString(id)
    	cT   := C.CString("Transcribing…")
    	cB   := C.CString("This usually takes under a minute")
    	defer C.free(unsafe.Pointer(cID))
    	defer C.free(unsafe.Pointer(cT))
    	defer C.free(unsafe.Pointer(cB))
    	C.meeting_notif_send(cID, cT, cB, nil)
    }

    func (n *darwinNotifier) TranscriptReady(mdPath string) {
    	id := nextNotifID()

    	notifMu.Lock()
    	notifCallbacks[id] = func(action string) {
    		if action == "OPEN" {
    			cPath := C.CString(mdPath)
    			defer C.free(unsafe.Pointer(cPath))
    			C.meeting_notif_open_file(cPath)
    		}
    	}
    	notifMu.Unlock()

    	cID    := C.CString(id)
    	cTitle := C.CString("Transcript ready")
    	cBody  := C.CString(filepath.Base(mdPath))
    	cCat   := C.CString("TRANSCRIPT_READY")
    	defer C.free(unsafe.Pointer(cID))
    	defer C.free(unsafe.Pointer(cTitle))
    	defer C.free(unsafe.Pointer(cBody))
    	defer C.free(unsafe.Pointer(cCat))

    	C.meeting_notif_send(cID, cTitle, cBody, cCat)
    }
    