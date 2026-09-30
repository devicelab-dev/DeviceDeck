//
//  Copyright © 2022 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

#if TARGET_OS_IPHONE && !TARGET_OS_MACCATALYST

/*!
 * @enum XCUIDeviceButton
 *
 * Represents a physical button on a device.
 *
 * @note Some buttons are not available in the Simulator, and should not be used in your tests.
 * You can use a block like this:
 *
 *     #if !TARGET_OS_SIMULATOR
 *     // test code that depends on buttons not available in the Simulator
 *     #endif
 *
 * in your test code to ensure it does not call unavailable APIs.
 */
typedef NS_ENUM(NSInteger, XCUIDeviceButton) {
    XCUIDeviceButtonHome = 1,
    XCUIDeviceButtonVolumeUp XCUI_SIMULATOR_UNAVAILABLE("This API is not available in the Simulator, see the XCUIDeviceButton documentation for details.") = 2,
    XCUIDeviceButtonVolumeDown XCUI_SIMULATOR_UNAVAILABLE("This API is not available in the Simulator, see the XCUIDeviceButton documentation for details.") = 3,
#if TARGET_OS_WATCH || TARGET_OS_IOS
    XCUIDeviceButtonAction = 4,
#endif // TARGET_OS_WATCH || TARGET_OS_IOS
#if TARGET_OS_IOS && !TARGET_OS_VISION
    XCUIDeviceButtonCamera,
#endif

};

#endif

NS_ASSUME_NONNULL_END
