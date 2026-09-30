//
//  Copyright © 2021 Apple Inc. All rights reserved.
//

#import <XCTest/XCTAttachment.h>

#if XCT_UI_TESTING_AVAILABLE

NS_ASSUME_NONNULL_BEGIN

@class XCUIScreenshot;

XCT_SWIFT_MAIN_ACTOR
@interface XCTAttachment (XCUIScreenshot_ConvenienceInitializers)

/*!
 * @method +attachmentWithScreenshot:
 * Creates an attachment with a screenshot and the specified quality.
 */
+ (instancetype)attachmentWithScreenshot:(XCUIScreenshot *)screenshot;

/*!
 * @method +attachmentWithScreenshot:quality:
 * Creates an attachment with a screenshot and the specified quality.
 */
+ (instancetype)attachmentWithScreenshot:(XCUIScreenshot *)screenshot quality:(XCTImageQuality)quality;

@end

NS_ASSUME_NONNULL_END

#endif // XCT_UI_TESTING_AVAILABLE
