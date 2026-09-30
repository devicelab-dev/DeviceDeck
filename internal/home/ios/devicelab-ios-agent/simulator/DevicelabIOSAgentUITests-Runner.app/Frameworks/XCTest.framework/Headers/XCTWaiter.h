#if __has_include(<XCTestCore/XCTWaiter.h>)
#import <XCTestCore/XCTWaiter.h>
#else

//
//  Copyright (c) 2016 Apple Inc. All rights reserved.
//

#import <XCTest/XCTestDefines.h>

@class XCTestExpectation;
@protocol XCTWaiterDelegate;

NS_ASSUME_NONNULL_BEGIN

/*!
 * @enum XCTWaiterResult
 * Values returned by a waiter when it completes, times out, or is interrupted due to another waiter
 * higher in the call stack timing out.
 */
typedef NS_ENUM(NSInteger, XCTWaiterResult) {
    XCTWaiterResultCompleted = 1,
    XCTWaiterResultTimedOut,
    XCTWaiterResultIncorrectOrder,
    XCTWaiterResultInvertedFulfillment,
    XCTWaiterResultInterrupted
};

/*!
 * @class XCTWaiter
 * Manages waiting - pausing the current execution context - for an array of XCTestExpectations. Waiters
 * can be used with or without a delegate to respond to events such as completion, timeout, or invalid 
 * expectation fulfillment. XCTestCase conforms to the delegate protocol and will automatically report
 * timeouts and other unexpected events as test failures.
 *
 * Waiters can be used without a delegate or any association with a test case instance. This allows test
 * support libraries to provide convenience methods for waiting without having to pass test cases through
 * those APIs.
 */
NS_SWIFT_SENDABLE
@interface XCTWaiter : NSObject

/*!
 * @method -initWithDelegate:
 * Creates a new waiter with the specified delegate.
 */
- (instancetype)initWithDelegate:(nullable id<XCTWaiterDelegate>)delegate NS_DESIGNATED_INITIALIZER;

/*!
 * @property delegate
 * The waiter delegate will be called with various events described in <XCTWaiterDelegate>.
 */
@property (nullable, weak) id<XCTWaiterDelegate> delegate;

/*!
 * @property fulfilledExpectations
 * Returns an array containing the expectations that were fulfilled, in that order, up until the waiter
 * stopped waiting. Expectations fulfilled after the waiter stopped waiting will not be in the array.
 * The array will be empty until the waiter has started waiting, even if expectations have already been
 * fulfilled. If a waiter is used to wait multiple times, this array will contain all of the
 * fulfilled expectations from each wait operation.
 */
@property (readonly) NSArray<XCTestExpectation *> *fulfilledExpectations;

/*!
 * @method -waitForExpectations:
 *
 * Waits on a group of expectations indefinitely.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @discussion
 * The test will continue to run until @a expectations are fulfilled or the
 * test reaches its execution time allowance.
 *
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * Enabling test timeouts is recommended when using this method to prevent a
 * runaway expectation from hanging the test.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
- (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method -waitForExpectations:timeout:
 *
 * Waits on a group of expectations for up to the specified timeout.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @param seconds
 * The number of seconds within which all expectations must be fulfilled.
 *
 * @discussion
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
- (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations timeout:(NSTimeInterval)seconds XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method -waitForExpectations:enforceOrder:
 *
 * Waits on a group of expectations indefinitely, optionally enforcing their
 * order of fulfillment.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @param enforceOrderOfFulfillment
 * If @c YES, the expectations specified by the @a expectations parameter must
 * be satisfied in the order they appear in the array.
 *
 * @discussion
 * The test will continue to run until @a expectations are fulfilled or the
 * test reaches its execution time allowance.
 *
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * Enabling test timeouts is recommended when using this method to prevent a
 * runaway expectation from hanging the test.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
- (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations enforceOrder:(BOOL)enforceOrderOfFulfillment XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method -waitForExpectations:timeout:enforceOrder:
 *
 * Waits on a group of expectations for up to the specified timeout, optionally
 * enforcing their order of fulfillment.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @param seconds
 * The number of seconds within which all expectations must be fulfilled.
 *
 * @param enforceOrderOfFulfillment
 * If @c YES, the expectations specified by the @a expectations parameter must
 * be satisfied in the order they appear in the array.
 *
 * @discussion
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
- (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations timeout:(NSTimeInterval)seconds enforceOrder:(BOOL)enforceOrderOfFulfillment XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method +waitForExpectations:
 *
 * Creates a waiter that waits on a group of expectations indefinitely.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @discussion
 * The test will continue to run until @a expectations are fulfilled or the
 * test reaches its execution time allowance.
 *
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * Enabling test timeouts is recommended when using this method to prevent a
 * runaway expectation from hanging the test.
 *
 * The waiter is discarded when the wait completes.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
+ (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations XCT_WARN_UNUSED XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method +waitForExpectations:timeout:
 *
 * Creates a waiter that waits on a group of expectations for up to the
 * specified timeout.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @param seconds
 * The number of seconds within which all expectations must be fulfilled.
 *
 * @discussion
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * The waiter is discarded when the wait completes.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
+ (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations timeout:(NSTimeInterval)seconds XCT_WARN_UNUSED XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method +waitForExpectations:enforceOrder:
 *
 * Creates a waiter that waits on a group of expectations indefinitely,
 * optionally enforcing their order of fulfillment.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @param enforceOrderOfFulfillment
 * If @c YES, the expectations specified by the @a expectations parameter must
 * be satisfied in the order they appear in the array.
 *
 * @discussion
 * The test will continue to run until @a expectations are fulfilled or the
 * test reaches its execution time allowance.
 *
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * Enabling test timeouts is recommended when using this method to prevent a
 * runaway expectation from hanging the test.
 *
 * The waiter is discarded when the wait completes.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
+ (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations enforceOrder:(BOOL)enforceOrderOfFulfillment XCT_WARN_UNUSED XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

/*!
 * @method +waitForExpectations:timeout:enforceOrder:
 *
 * Creates a waiter that waits on a group of expectations for up to the
 * specified timeout, optionally enforcing their order of fulfillment.
 *
 * @param expectations
 * An array of expectations that must be fulfilled.
 *
 * @param seconds
 * The number of seconds within which all expectations must be fulfilled.
 *
 * @param enforceOrderOfFulfillment
 * If @c YES, the expectations specified by the @a expectations parameter must
 * be satisfied in the order they appear in the array.
 *
 * @discussion
 * Expectations can only appear in the list once. This method may return
 * early based on fulfillment of the provided expectations.
 *
 * The waiter is discarded when the wait completes.
 *
 * @returns A value describing the outcome of waiting for @a expectations.
 */
+ (XCTWaiterResult)waitForExpectations:(NSArray<XCTestExpectation *> *)expectations timeout:(NSTimeInterval)seconds enforceOrder:(BOOL)enforceOrderOfFulfillment XCT_WARN_UNUSED XCT_SWIFT_UNAVAILABLE_FROM_ASYNC("Use await fulfillment(of:timeout:enforceOrder:) instead");

@end

/*!
 * @protocol XCTWaiterDelegate
 * Events are reported to the waiter's delegate via these methods. XCTestCase conforms to the delegate
 * protocol and will automatically report timeouts and other unexpected events as test failures.
 *
 * @note These methods are invoked on an arbitrary queue.
 */
@protocol XCTWaiterDelegate <NSObject>
@optional
/*!
 * @method -waiter:didTimeoutWithUnfulfilledExpectations:
 * Invoked when not all waited on expectations are fulfilled during the timeout period. If the delegate
 * is an XCTestCase instance, this will be reported as a test failure.
 */
- (void)waiter:(XCTWaiter *)waiter didTimeoutWithUnfulfilledExpectations:(NSArray<XCTestExpectation *> *)unfulfilledExpectations;

/*!
 * @method -waiter:fulfillmentDidViolateOrderingConstraintsForExpectation:requiredExpectation:
 * Invoked when the -wait call has specified that fulfillment order should be enforced and an expectation
 * has been fulfilled in the wrong order. If the delegate is an XCTestCase instance, this will be reported
 * as a test failure.
 */
- (void)waiter:(XCTWaiter *)waiter fulfillmentDidViolateOrderingConstraintsForExpectation:(XCTestExpectation *)expectation requiredExpectation:(XCTestExpectation *)requiredExpectation;

/*!
 * @method -waiter:didFulfillInvertedExpectation:
 * Invoked when an expectation marked as inverted (/see inverted) is fulfilled. If the delegate is an
 * XCTestCase instance, this will be reported as a test failure.
 */
- (void)waiter:(XCTWaiter *)waiter didFulfillInvertedExpectation:(XCTestExpectation *)expectation;

/*!
 * @method -nestedWaiter:wasInterruptedByTimedOutWaiter:
 * Invoked when the waiter is interrupted prior to its expectations being fulfilled or timing out.
 * This occurs when an "outer" waiter times out, resulting in any waiters nested inside it being
 * interrupted to allow the call stack to quickly unwind.
 */
- (void)nestedWaiter:(XCTWaiter *)waiter wasInterruptedByTimedOutWaiter:(XCTWaiter *)outerWaiter;

@end

NS_ASSUME_NONNULL_END


#endif
