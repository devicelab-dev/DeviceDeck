//
//  Copyright © 2026 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>
#import <XCUIAutomation/XCUIVoiceOverOutput.h>

NS_ASSUME_NONNULL_BEGIN

@class XCUIVoiceOverOutput;

/*!
 * @class XCUIVoiceOverService
 * Provides programmatic control of VoiceOver for UI testing.
 *
 * Access this service through the @c voiceOverService property on @c XCUIDevice.
 */
API_AVAILABLE(ios(27.0), macos(27.0), tvos(27.0), watchos(27.0), visionos(27.0))
XCUI_SWIFT_MAIN_ACTOR
@interface XCUIVoiceOverService : NSObject

+ (instancetype)new XCUI_UNAVAILABLE("Access XCUIVoiceOverService through the voiceOverService property on XCUIDevice.");
- (instancetype)init XCUI_UNAVAILABLE("Access XCUIVoiceOverService through the voiceOverService property on XCUIDevice.");

/// Provides debugging information about the service.
@property (readonly, copy) NSString *debugDescription;

/// Enable VoiceOver.
- (BOOL)enableAndReturnError:(NSError **)error
    NS_SWIFT_NAME(enable());

/// Disable VoiceOver.
- (BOOL)disableAndReturnError:(NSError **)error
    NS_SWIFT_NAME(disable());

/// Whether VoiceOver is currently enabled.
@property (readonly, getter=isEnabled) BOOL enabled;

/// Move VoiceOver to the next element and return its speech.
- (nullable XCUIVoiceOverOutput *)moveForwardAndReturnError:(NSError **)error
    NS_SWIFT_NAME(moveForward())
    NS_REFINED_FOR_SWIFT;

/// Move VoiceOver to the previous element and return its speech.
- (nullable XCUIVoiceOverOutput *)moveBackwardAndReturnError:(NSError **)error
    NS_SWIFT_NAME(moveBackward())
    NS_REFINED_FOR_SWIFT;

/// Return the speech for the currently focused element.
- (nullable XCUIVoiceOverOutput *)currentSpeechAndReturnError:(NSError **)error
    NS_SWIFT_NAME(currentSpeech())
    NS_REFINED_FOR_SWIFT;

#if TARGET_OS_OSX || TARGET_OS_IOS
/// Move VoiceOver into the current container and return its speech.
- (nullable XCUIVoiceOverOutput *)moveInAndReturnError:(NSError **)error
    NS_SWIFT_NAME(moveIn())
    NS_REFINED_FOR_SWIFT
    API_AVAILABLE(ios(27.0), macos(27.0))
    API_UNAVAILABLE(tvos, watchos, visionos);

/// Move VoiceOver out of the current container and return its speech.
- (nullable XCUIVoiceOverOutput *)moveOutAndReturnError:(NSError **)error
    NS_SWIFT_NAME(moveOut())
    NS_REFINED_FOR_SWIFT
    API_AVAILABLE(ios(27.0), macos(27.0))
    API_UNAVAILABLE(tvos, watchos, visionos);
#endif

@end

NS_ASSUME_NONNULL_END
