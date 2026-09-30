#if __has_include(<XCTestCore/XCTIssue.h>)
#import <XCTestCore/XCTIssue.h>
#else

//
//  Copyright © 2019 Apple Inc. All rights reserved.
//

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

/*!
 * @enum XCTIssueType
 * Types of failures and other issues that can be reported for tests.
 */
typedef NS_ENUM(NSInteger, XCTIssueType) {
    /// Issue raised by a failed XCTAssert or related API.
    XCTIssueTypeAssertionFailure = 0,
    /// Issue raised by the test throwing an error in Swift. This could also occur if an Objective C test is implemented in the form `- (BOOL)testFoo:(NSError **)outError` and returns NO with a non-nil out error.
    XCTIssueTypeThrownError = 1,
    /// Code in the test throws and does not catch an exception, Objective C, C++, or other.
    XCTIssueTypeUncaughtException = 2,
    /// One of the XCTestCase(measure:) family of APIs detected a performance regression.
    XCTIssueTypePerformanceRegression = 3,
    /// One of the framework APIs failed internally. For example, XCUIApplication was unable to launch or terminate an app or XCUIElementQuery was unable to complete a query.
    XCTIssueTypeSystem = 4,
    /// Issue raised when XCTExpectFailure is used but no matching issue is recorded.
    XCTIssueTypeUnmatchedExpectedFailure = 5,
};

/// An enum representing the severity of a test issue.
///
/// The numeric values of this enum's cases are comparable. A case which represents
/// higher severity has a larger numeric value than one which represents lower
/// severity. Specifying a numeric severity value other than one corresponding to
/// a case defined below when initializing an ``XCTIssue`` is unsupported.
typedef NS_ENUM(NSInteger, XCTIssueSeverity) {
    /// The severity level for an issue which should be noted but is not
    /// necessarily an error.
    /// 
    /// An issue with warning severity does not cause the test it's associated
    /// with to be marked as a failure, but is noted in the results.
    XCTIssueSeverityWarning = 4,

    /// The severity level for an issue which represents an error in a test.
    ///
    /// An issue with error severity causes the test it's associated with to be
    /// marked as a failure.
    XCTIssueSeverityError = 8,
} NS_SWIFT_NAME(XCTIssue.Severity);

@class XCTAttachment;
@class XCTSourceCodeContext;

/*!
 * @class XCTIssue
 * Encapsulates all data concerning a test failure or other issue.
 */
@interface XCTIssue : NSObject <NSCopying, NSMutableCopying, NSSecureCoding>

- (instancetype)initWithType:(XCTIssueType)type
          compactDescription:(NSString *)compactDescription
         detailedDescription:(nullable NSString *)detailedDescription
           sourceCodeContext:(XCTSourceCodeContext *)sourceCodeContext
             associatedError:(nullable NSError *)associatedError
                 attachments:(NSArray<XCTAttachment *> *)attachments NS_DESIGNATED_INITIALIZER;

- (instancetype)initWithType:(XCTIssueType)type
          compactDescription:(NSString *)compactDescription
         detailedDescription:(nullable NSString *)detailedDescription
           sourceCodeContext:(XCTSourceCodeContext *)sourceCodeContext
             associatedError:(nullable NSError *)associatedError
                 attachments:(NSArray<XCTAttachment *> *)attachments
                    severity:(XCTIssueSeverity)severity NS_DESIGNATED_INITIALIZER;
                    
- (instancetype)initWithType:(XCTIssueType)type
          compactDescription:(NSString *)compactDescription
                    severity:(XCTIssueSeverity)severity;

- (instancetype)initWithType:(XCTIssueType)type compactDescription:(NSString *)compactDescription;

- (instancetype)init NS_UNAVAILABLE;
+ (instancetype)new NS_UNAVAILABLE;

/// The type of the issue.
@property (readonly) XCTIssueType type;

/// A concise description of the issue, expected to be free of transient data and suitable for use in test run
/// summaries and for aggregation of results across multiple test runs.
@property (readonly, copy) NSString *compactDescription;

/// A detailed description of the issue designed to help diagnose the issue. May include transient data such as
/// numbers, object identifiers, timestamps, etc.
@property (readonly, copy, nullable) NSString *detailedDescription;

/// The source code location (file and line number) and the call stack associated with the issue.
@property (readonly, strong) XCTSourceCodeContext *sourceCodeContext;

/// Error associated with the issue.
@property (readonly, strong, nullable) NSError *associatedError;

/// All attachments associated with the issue.
@property (readonly, copy) NSArray<XCTAttachment *> *attachments;

/// The severity of the issue.
@property (readonly) XCTIssueSeverity severity;

/// Whether or not this issue should cause the test it's associated with to be
/// considered a failure.
///
/// The value of this property is `YES` for issues which have a severity level of
/// ``XCTIssueSeverityError`` or higher.
/// Otherwise, the value of this property is `NO`.
///
/// Use this property to determine if an issue should be considered a failure, instead of
/// directly comparing the value of the ``severity`` property.
@property (readonly) BOOL isFailure;

@end

/*!
* @class XCTMutableIssue
* Mutable variant of XCTIssue, suitable for modifying by overrides in the reporting chain.
*/
@interface XCTMutableIssue : XCTIssue

@property (readwrite) XCTIssueType type;
@property (readwrite, copy) NSString *compactDescription;
@property (readwrite, copy, nullable) NSString *detailedDescription;
@property (readwrite, strong) XCTSourceCodeContext *sourceCodeContext;
@property (readwrite, strong, nullable) NSError *associatedError;
@property (readwrite, copy) NSArray<XCTAttachment *> *attachments;
@property (readwrite) XCTIssueSeverity severity;

/// Add an attachment to this issue.
- (void)addAttachment:(XCTAttachment *)attachment;

@end

NS_ASSUME_NONNULL_END

#endif
