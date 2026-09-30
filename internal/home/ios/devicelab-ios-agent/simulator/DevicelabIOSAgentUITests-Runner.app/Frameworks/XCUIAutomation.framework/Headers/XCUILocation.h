//
//  Copyright © 2022 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

@class CLLocation;

/*!
 * @class XCUILocation
 * Represents a device's simulated location by wrapping a CLLocation object from Core Location
 */
XCUI_SWIFT_MAIN_ACTOR
@interface XCUILocation : NSObject

+ (instancetype)new XCUI_UNAVAILABLE("Initialize XCUILocation using a CLLocation object");
- (instancetype)init XCUI_UNAVAILABLE("Initialize XCUILocation using a CLLocation object");

- (instancetype)initWithLocation:(CLLocation *)location;

/*!
 * Provides debugging information about the underlying CLLocation wrapped by this object.
 */
@property (readonly, copy) NSString *debugDescription;

/*!
 * Provides access to the CLLocation object stored by this XCUILocation instance.
 */
@property (readonly, copy) CLLocation *location;

@end

NS_ASSUME_NONNULL_END
