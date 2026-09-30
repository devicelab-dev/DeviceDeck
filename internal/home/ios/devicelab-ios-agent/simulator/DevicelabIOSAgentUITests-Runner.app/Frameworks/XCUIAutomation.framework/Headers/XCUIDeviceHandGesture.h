//
//  Copyright © 2023 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

#if TARGET_OS_WATCH

NS_ASSUME_NONNULL_BEGIN

/*!
 * @enum XCUIDeviceHandGesture
 *
 * Represents a hand gesture that can be performed while wearing an Apple Watch.
 *
 */
typedef NS_ENUM(NSInteger, XCUIDeviceHandGesture) {
    XCUIDeviceHandGestureDoubleTap                              = 1,
    XCUIDeviceHandGestureFlick API_AVAILABLE(watchos(12.0))     = 2,
};

NS_ASSUME_NONNULL_END

#endif // TARGET_OS_WATCH
