//
//  Copyright © 2017 Apple. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

@class XCUIScreenshot;

NS_ASSUME_NONNULL_BEGIN

@class XCTAttachment;

XCUI_SWIFT_MAIN_ACTOR
@protocol XCUIScreenshotProviding <NSObject>

/*!
 * Captures and returns a screenshot of the receiver.
 *
 * Equivalent to capturing a screenshot manually, e.g. if two windows are overlapping and
 * the occluded window is captured, the front window will be visible in the screenshot.
 */
- (XCUIScreenshot *)screenshot;

@end

NS_ASSUME_NONNULL_END
