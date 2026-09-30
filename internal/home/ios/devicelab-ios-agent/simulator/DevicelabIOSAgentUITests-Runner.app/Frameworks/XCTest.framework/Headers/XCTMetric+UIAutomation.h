//
//  Copyright © 2018 Apple Inc. All rights reserved.
//

#import <XCTest/XCTMetric.h>

@class XCUIApplication;

NS_ASSUME_NONNULL_BEGIN


XCT_METRIC_API_AVAILABLE
XCT_SWIFT_MAIN_ACTOR
@interface XCTCPUMetric (UIAutomation)

/*!
 * @method initWithApplication:
 * Creates a metric which will target the process described by the XCUIApplication instance.
 *
 * @param application An instance of XCUIApplication which will be targeted to gather measurements.
 *
 * @return An initialized metric.
 */
- (instancetype)initWithApplication:(XCUIApplication *)application;

@end

XCT_METRIC_API_AVAILABLE
XCT_SWIFT_MAIN_ACTOR
@interface XCTMemoryMetric (UIAutomation)

/*!
 * @method initWithApplication:
 * Creates a metric which will target the process described by the XCUIApplication instance.
 *
 * @param application An instance of XCUIApplication which will be targeted to gather measurements.
 *
 * @return An initialized metric.
 */
- (instancetype)initWithApplication:(XCUIApplication *)application;

@end

XCT_METRIC_API_AVAILABLE
XCT_SWIFT_MAIN_ACTOR
@interface XCTStorageMetric (UIAutomation)

/*!
 * @method initWithApplication:
 * Creates a metric which will target the process described by the XCUIApplication instance.
 *
 * @param application An instance of XCUIApplication which will be targeted to gather measurements.
 *
 * @return An initialized metric.
 */
- (instancetype)initWithApplication:(XCUIApplication *)application;

@end

API_AVAILABLE(macos(26.0), ios(26.0), watchos(26.0), tvos(26.0)) API_UNAVAILABLE(visionos)
XCT_SWIFT_MAIN_ACTOR
@interface XCTHitchMetric (UIAutomation)

/*!
 * @method initWithApplication:
 * Creates a metric which will target the process described by the XCUIApplication instance.
 *
 * @param application An instance of XCUIApplication which will be targeted to gather measurements.
 *
 * @return An initialized metric.
 */
- (instancetype)initWithApplication:(XCUIApplication *)application;

@end


NS_ASSUME_NONNULL_END
