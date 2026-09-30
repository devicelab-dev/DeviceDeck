//
//  Copyright © 2022 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAccessibilityAuditTypes.h>

NS_ASSUME_NONNULL_BEGIN

@class XCUIElement;

/// Represents an issue returned by an accessibility audit.
/// @seealso [XCUIApplication performAccessibilityAuditWithAuditTypes:issueHandler:error:]
API_AVAILABLE(macos(14.0), ios(17.0), tvos(17.0), watchos(10.0))
@interface XCUIAccessibilityAuditIssue : NSObject

/// The element associated with the issue.
@property (nullable, nonatomic, readonly, strong) XCUIElement *element;

/// A short description about the issue.
@property (nonatomic, readonly, copy) NSString *compactDescription;

/// A longer description of the issue with more details about the failure.
@property (nonatomic, readonly, copy) NSString *detailedDescription;

/// The type of audit which generated the issue.
@property (nonatomic, readonly) XCUIAccessibilityAuditType auditType;

+ (instancetype)new NS_UNAVAILABLE;
- (instancetype)init NS_UNAVAILABLE;

@end

NS_ASSUME_NONNULL_END
