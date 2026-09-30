//
//  Copyright © 2022 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

/*!
    Bitmasks for use with [XCUIApplication performAccessibilityAuditWithAuditTypes:issueHandler:error:].
    These represent the types of audits that can be run, and they can be combined using bitwise OR.
*/

API_AVAILABLE(macos(14.0), ios(17.0), tvos(17.0), watchos(10.0))
typedef NS_OPTIONS(uint64_t, XCUIAccessibilityAuditType) {
    // Types of audits supported on all platforms
    XCUIAccessibilityAuditTypeContrast                     = (1UL << 0),
    XCUIAccessibilityAuditTypeElementDetection             = (1UL << 1),
    XCUIAccessibilityAuditTypeHitRegion                    = (1UL << 2),
    XCUIAccessibilityAuditTypeSufficientElementDescription = (1UL << 3),
    
#if TARGET_OS_IOS || TARGET_OS_TV || TARGET_OS_WATCH || TARGET_OS_SIMULATOR
    // Types of audits supported on iOS, watchOS, and tvOS
    XCUIAccessibilityAuditTypeDynamicType                  = (1UL << 16),
    XCUIAccessibilityAuditTypeTextClipped                  = (1UL << 17),
    XCUIAccessibilityAuditTypeTrait                        = (1UL << 18),
    
#elif TARGET_OS_OSX || TARGET_OS_MACCATALYST
    // Types of audits supported on macOS
    XCUIAccessibilityAuditTypeAction                       = (1UL << 32),
    XCUIAccessibilityAuditTypeParentChild                  = (1UL << 33),
#endif
    XCUIAccessibilityAuditTypeAll                          = ~0UL,
};

NS_ASSUME_NONNULL_END
