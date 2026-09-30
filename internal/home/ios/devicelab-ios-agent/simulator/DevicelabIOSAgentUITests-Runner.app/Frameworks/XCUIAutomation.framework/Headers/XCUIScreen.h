//
//  Copyright © 2017 Apple. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>
#import <XCUIAutomation/XCUIScreenshotProviding.h>

NS_ASSUME_NONNULL_BEGIN

/*!
 * Represents an active screen of a device. One screen of each device is designated
 * as the "main" screen, on macOS that is the one owning the menu bar, on other platforms
 * it is the primary screen of the device.
 *
 * A screen can be asked for a screenshot using the methods declared in XCUIScreenshotProviding.
 */
XCUI_SWIFT_MAIN_ACTOR
@interface XCUIScreen : NSObject <XCUIScreenshotProviding>

+ (instancetype)new XCUI_UNAVAILABLE("Use XCUIScreen.screens or XCUIScreen.mainScreen to acquire particular screen objects.");
- (instancetype)init XCUI_UNAVAILABLE("Use XCUIScreen.screens or XCUIScreen.mainScreen to acquire particular screen objects.");

/*!
 * Returns the current device's main screen.
 */
@property (class, readonly, strong) XCUIScreen *mainScreen;

/*!
 * Returns the list of active screens.
 * The first screen returned in the list is the main screen.
 */
@property (class, readonly, copy) NSArray<XCUIScreen *> *screens;

@end

NS_ASSUME_NONNULL_END
