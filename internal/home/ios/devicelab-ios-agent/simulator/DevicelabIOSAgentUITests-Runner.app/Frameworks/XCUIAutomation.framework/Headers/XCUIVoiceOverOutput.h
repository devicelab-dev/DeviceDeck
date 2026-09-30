//
//  Copyright © 2026 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

/*!
 * @class XCUIVoiceOverOutput
 * The speech output that VoiceOver produces when focusing an element.
 */
API_AVAILABLE(ios(27.0), macos(27.0), tvos(27.0), watchos(27.0), visionos(27.0))
XCUI_SWIFT_MAIN_ACTOR
NS_SWIFT_NAME(XCUIVoiceOverService.Output)
@interface XCUIVoiceOverOutput : NSObject

/// What VoiceOver spoke for this element, e.g. @"Add Favorites, button".
@property (readonly, copy) NSString *utterance;

+ (instancetype)new NS_UNAVAILABLE;
- (instancetype)init NS_UNAVAILABLE;

@end

NS_ASSUME_NONNULL_END
