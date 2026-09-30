//
//  Copyright © 2022 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

/*!
 * @class XCUISystem
 * Represents a device's base system, providing an interface for performing OS-specific actions like opening universal links.
 */
XCUI_SWIFT_MAIN_ACTOR
@interface XCUISystem : NSObject

+ (instancetype)new XCUI_UNAVAILABLE("Access XCUISystem through the system property on XCUIDevice.");
- (instancetype)init XCUI_UNAVAILABLE("Access XCUISystem through the system property on XCUIDevice.");

/*!
 * Open a URL on the system using the default application for it.
 */
- (void)openURL:(NSURL *)url;

@end

NS_ASSUME_NONNULL_END
