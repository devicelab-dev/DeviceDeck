//
//  Copyright © 2021 Apple Inc. All rights reserved.
//

#import <XCTest/XCTestDefines.h>
#import <XCTest/XCTestCase.h>

#if XCT_UI_TESTING_AVAILABLE

NS_ASSUME_NONNULL_BEGIN

XCT_SWIFT_MAIN_ACTOR
@interface XCTestCase (XCUIApplication_LaunchTesting)

/// Determines whether the tests in this class should run multiple times, once for each of the target application's UI configurations.
///
/// Returns false by default. If overridden in a UI test subclass to return true, each test in that
/// class will run multiple times, once for each supported UI configuration of the default target application.
///
/// Supported UI configurations are detected by Xcode according to the settings of the default target app
/// for the UI test target and may include:
///
/// - Appearances (e.g. light mode, dark mode)
/// - Orientations (e.g. portrait, landscape)
/// - Localizations (e.g. en_US, zh_CN)
///
/// Given the above example, one UI configuration would be {dark mode, landscape, en_US}, another would be
/// {light mode, portrait, zh_CN}, and so forth. The number of combinations determines the number of times each
/// test will run. The UI configuration is used automatically when calling `XCUIApplication.launch()` in each test.
@property (class, readonly) BOOL runsForEachTargetApplicationUIConfiguration;

@end

NS_ASSUME_NONNULL_END

#endif // XCT_UI_TESTING_AVAILABLE
