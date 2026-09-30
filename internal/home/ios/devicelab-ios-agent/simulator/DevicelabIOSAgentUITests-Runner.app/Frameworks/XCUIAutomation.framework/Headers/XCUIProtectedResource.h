//
//  Copyright © 2019 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

XCUI_PROTECTED_RESOURCES_RESET_API_AVAILABLE
typedef NS_ENUM(NSInteger, XCUIProtectedResource) {
    // All platforms
    XCUIProtectedResourceContacts               = 1,
    XCUIProtectedResourceCalendar               = 2,
    XCUIProtectedResourceReminders              = 3,
    XCUIProtectedResourcePhotos                 = 4,
    XCUIProtectedResourceMicrophone             = 5,
    XCUIProtectedResourceCamera                 = 6,
    XCUIProtectedResourceMediaLibrary           = 7,
    XCUIProtectedResourceHomeKit                = 8,

#if TARGET_OS_OSX || TARGET_OS_IOS || TARGET_OS_WATCH
    XCUIProtectedResourceFocus API_AVAILABLE(macos(12.0), ios(15.0), watchos(8.0)) = 9,
#endif

#if TARGET_OS_OSX // macOS-specific resources

    XCUIProtectedResourceSystemRootDirectory    = 0x40000000,
    XCUIProtectedResourceUserDesktopDirectory   = 0x40000001,
    XCUIProtectedResourceUserDownloadsDirectory = 0x40000002,
    XCUIProtectedResourceUserDocumentsDirectory = 0x40000003,
    XCUIProtectedResourceNetworkVolumes         = 0x40000004,
    XCUIProtectedResourceRemovableVolumes       = 0x40000005,
    XCUIProtectedResourceAppleEvents            = 0x40000006,

#endif

#if TARGET_OS_IOS
    XCUIProtectedResourceLocalNetwork API_AVAILABLE(ios(15.4))                            = -0x40000005,
#endif

#if TARGET_OS_IPHONE // iOS Family-specific resources

    XCUIProtectedResourceBluetooth                                                        = -0x40000000,
    XCUIProtectedResourceKeyboardNetwork                                                  = -0x40000001,
    XCUIProtectedResourceLocation                                                         = -0x40000002,
    XCUIProtectedResourceHealth API_AVAILABLE(ios(14.0), tvos(14.0))                      = -0x40000003,
    XCUIProtectedResourceUserTracking API_AVAILABLE(ios(15.0), tvos(15.0))                = -0x40000004,

#endif
};

NS_ASSUME_NONNULL_END
