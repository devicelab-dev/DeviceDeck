#if __has_include(<XCTestCore/XCTExpectedFailure.h>)
#import <XCTestCore/XCTExpectedFailure.h>
#else

//
//  Copyright © 2020 Apple Inc. All rights reserved.
//

#import <XCTest/XCTestDefines.h>

NS_ASSUME_NONNULL_BEGIN

@class XCTIssue;
@class XCTSourceCodeContext;
@class XCTExpectedFailureOptions;

/*!
 * @function XCTExpectFailure()
 * Declares that the test is expected to fail at some point beyond the call. This can be used to both document and
 * suppress a known issue when immediate resolution is not possible. Issues caught by XCTExpectFailure do not
 * impact the aggregate results of the test suites which own them.
 *
 * This function may be invoked repeatedly and has stack semantics. Failures are associated with the closest
 * matching expected failure and the stack is cleaned up by the test after it runs. If a failure is expected
 * but none is recorded, a distinct failure for the unmatched expected failure will be recorded instead.
 *
 * By default XCTExpectFailure matches all issues, including issues with warning severity.
 *
 * Threading considerations: when XCTExpectFailure is called on the test's primary thread it will match against
 * any issue recorded on any thread. When XCTExpectFailure is called on any other thread, it will only match
 * against issues recorded on the same thread.
 *
 * @param failureReason Explanation of the issue being suppressed. If it contains
 * a URL, that URL can be extracted and presented as a link in reporting UI (Xcode and CI).
 */
XCT_EXPORT void XCTExpectFailure(NSString *_Nullable failureReason) NS_SWIFT_UNAVAILABLE("Use XCTExpectFailure(_:options:) instead");

/*!
 * @function XCTExpectFailureWithOptions()
 * Like XCTExpectFailure, but takes an options object that can be used to customize the behavior.
 *
 * @param options The options can include a custom issue matching block as well as the ability to
 * disable "strict" behavior, which relaxes the requirement that a call to XCTExpectFailure must be matched
 * against at least one recorded issue.
 */
XCT_EXPORT void XCTExpectFailureWithOptions(NSString *_Nullable failureReason,
                                            XCTExpectedFailureOptions *options) NS_REFINED_FOR_SWIFT;

/*!
 * @function XCTExpectFailureInBlock()
 * Like XCTExpectFailure, but limits the scope in which issues are matched.
 *
 * @param failingBlock The scope of code in which the failure is expected. Note that this will only
 * match against failures in that scope on the same thread; failures in dispatch callouts or other code
 * running on a different thread will not be matched.
 */
XCT_EXPORT void XCTExpectFailureInBlock(NSString *_Nullable failureReason,
                                        XCT_NOESCAPE void (^failingBlock)(void)) NS_SWIFT_UNAVAILABLE("Use XCTExpectFailure(_:options:failingBlock:) instead");

/*!
 * @function XCTExpectFailureWithOptionsInBlock()
 * Like XCTExpectFailure, but takes an options object that can be used to customize the behavior and
 * limits the scope in which issues are matched.
 */
XCT_EXPORT void XCTExpectFailureWithOptionsInBlock(NSString *_Nullable failureReason,
                                                   XCTExpectedFailureOptions *options,
                                                   XCT_NOESCAPE void (^failingBlock)(void)) NS_REFINED_FOR_SWIFT;

/*!
 * @class XCTExpectedFailureOptions
 * Describes the rules for matching issues to expected failures and other behaviors related to
 * expected failure handling.
 */
@interface XCTExpectedFailureOptions : NSObject <NSCopying, NSSecureCoding>

/*!
 * @property issueMatcher
 * An optional filter can be used to determine whether or not an issue recorded inside an expected
 * failure block should be matched to the expected failure. Issues that are not matched to an expected
 * failure will be recorded as normal issues (real test failures). By default the filter is nil and
 * all issues are matched, including issues with warning severity.
 */
@property (copy) BOOL (^issueMatcher)(XCTIssue *);

/*!
 * @property enabled
 * For expected failures that only occur under certain circumstances, this flag can be used to
 * disable the expected failure. In the closure-based variants of XCTExpectFailure, the failing block
 * will be executed normally. Defaults to YES/true.
 */
@property (getter=isEnabled) BOOL enabled;

/*!
 * @property strict
 * If true (the default) and no issue is matched to the expected failure, then an issue will be
 * recorded for the unmatched expected failure itself.
 */
@property (getter=isStrict) BOOL strict;

/*!
 * @method nonStrictOptions
 * Convenience factory method which returns a new instance of XCTExpectedFailureOptions that has `isStrict` set to NO, with every other value set to its default.
 */
+ (XCTExpectedFailureOptions *)nonStrictOptions;

@end

/*!
 * @class XCTExpectedFailure
 * Contains the details about a single instance of an expected failure, including the failure
 * reason and the underlying issue that was recorded.
 */
@interface XCTExpectedFailure : NSObject <NSSecureCoding>

- (instancetype)init XCT_UNAVAILABLE("Instances of XCTExpectedFailure should not be created directly, they are provided by the framework.");
+ (instancetype)new XCT_UNAVAILABLE("Instances of XCTExpectedFailure should not be created directly, they are provided by the framework.");

/*!
 * @property failureReason
 * Explanation of the problem requiring the issue to be suppressed.
 */
@property (readonly, nullable, copy) NSString *failureReason;

/*!
 * @property issue
 * The issue being suppressed.
 */
@property (readonly, copy) XCTIssue *issue;

@end

NS_ASSUME_NONNULL_END

#endif
