//
//  Copyright © 2026 Apple Inc. All rights reserved.
//

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

/// Error domain for XCUIVoiceOverService errors.
extern NSErrorDomain const XCUIVoiceOverServiceErrorDomain
    API_AVAILABLE(ios(27.0), macos(27.0), tvos(27.0), watchos(27.0), visionos(27.0));

/// Error codes for XCUIVoiceOverService operations.
typedef NS_ERROR_ENUM(XCUIVoiceOverServiceErrorDomain, XCUIVoiceOverServiceError) {
    /// VoiceOver daemon did not start within the timeout.
    XCUIVoiceOverServiceErrorFailedToStart = 1,
    /// A navigation or speech method was called without first calling @c enable().
    XCUIVoiceOverServiceErrorNotRunning = 2,
    /// VoiceOver did not produce any speech within the timeout.
    XCUIVoiceOverServiceErrorNoSpeech = 3,
    /// VoiceOver daemon did not stop within the timeout after @c disable().
    XCUIVoiceOverServiceErrorFailedToStop = 4,
} NS_SWIFT_NAME(XCUIVoiceOverService.Error);

NS_ASSUME_NONNULL_END
