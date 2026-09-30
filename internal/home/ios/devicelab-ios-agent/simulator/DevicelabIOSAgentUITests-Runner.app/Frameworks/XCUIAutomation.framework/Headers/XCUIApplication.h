//
//  Copyright (c) 2014-2015 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>
#import <XCUIAutomation/XCUIElement.h>
#import <XCUIAutomation/XCUIProtectedResource.h>
#import <XCUIAutomation/XCUIAccessibilityAuditIssue.h>

NS_ASSUME_NONNULL_BEGIN

typedef NS_ENUM(NSUInteger, XCUIApplicationState) {
    XCUIApplicationStateUnknown = 0,
    XCUIApplicationStateNotRunning = 1,
#if !TARGET_OS_OSX
    XCUIApplicationStateRunningBackgroundSuspended = 2,
#endif
    XCUIApplicationStateRunningBackground = 3,
    XCUIApplicationStateRunningForeground = 4
};

/*! Proxy for an application that may or may not be running. */
XCUI_SWIFT_MAIN_ACTOR
@interface XCUIApplication : XCUIElement

/*!
 * Returns a proxy for the application specified by the "Target Application" target setting.
 */
- (instancetype)init NS_DESIGNATED_INITIALIZER;
+ (instancetype)new;

/*!
 * Returns a proxy for an application associated with the specified bundle identifier.
 */
- (instancetype)initWithBundleIdentifier:(NSString *)bundleIdentifier NS_DESIGNATED_INITIALIZER;

#if TARGET_OS_OSX || TARGET_OS_MACCATALYST

/*!
 * Returns a proxy for the application at the specified file system URL.
 */
- (instancetype)initWithURL:(NSURL *)url NS_DESIGNATED_INITIALIZER;

#endif

/*!
 * Launches the application synchronously. On return the application ready to handle events. If the
 * application is already running, the existing instance will be terminated to ensure a clean state
 * for the launched instance.
 *
 * Any failure in the launch sequence will be reported as a test failure and the test will be halted
 * at that point.
*/
- (void)launch;

/*!
 * Launches the application synchronously using the provided URL. On return the application is ready
 * to handle events, similar to the behavior of -launch.
 *
 * Any failure in the launch sequence, or any failure in using the provided URL, will be reported as a
 * test failure and the test will be halted at that point.
*/
- (void)openURL:(NSURL *)url API_AVAILABLE(macos(13.3), ios(16.4), tvos(16.4), watchos(9.4));

/*!
 * Activates the application synchronously. On return the application is ready to handle events.
 * If the application was not running prior, it will be launched automatically. If the application
 * would be launched as a result of this method and was previously launched via -launch, the launch
 * arguments and environment variables that were used then will be supplied again for the new launch.
 *
 * Unlike -launch, if the application is already running this call will not terminate the existing
 * instance.
 *
 * Any failure in the activation or launch sequence will be reported as a test failure and the test
 * will be halted at that point.
*/
- (void)activate;

/*!
 * Terminates any running instance of the application. If the application has an existing debug session
 * via Xcode, the termination is implemented as a halt via that debug connection. Otherwise, a SIGKILL
 * is sent to the process.
 */
- (void)terminate;

/*!
 * The arguments that will be passed to the application on launch. If not modified, these are the
 * arguments that Xcode will pass on launch. Those arguments can be changed, added to, or removed.
 * Unlike NSTask, it is legal to modify these arguments after the application has been launched. These
 * changes will not affect the current launch session, but will take effect the next time the application
 * is launched.
 */
@property (nonatomic, copy) NSArray <NSString *> *launchArguments;

/*!
 * The environment that will be passed to the application on launch. If not modified, this is the
 * environment that Xcode will pass on launch. Those variables can be changed, added to, or removed.
 * Unlike NSTask, it is legal to modify the environment after the application has been launched. These
 * changes will not affect the current launch session, but will take effect the next time the application
 * is launched.
 */
@property (nonatomic, copy) NSDictionary <NSString *, NSString *> *launchEnvironment;

/*!
 * The most recently observed state of the application. Applications are passively monitored to update
 * this property as they change state. Consequently, updates to this property are inherently asynchronous.
 *
 * Some guarantees are made, however:
 *
 *    - When -launch and -activate return, if they were successful, the state of the application will be
 *      XCUIApplicationStateRunningBackground or XCUIApplicationStateRunningForeground, whichever is
 *      appropriate for the application. Most applications will be XCUIApplicationStateRunningForeground
 *      after launch or activation.
 *
 *    - When -terminate returns, if it was successful, the state of the application will be
 *      XCUIApplicationStateNotRunning.
 */
@property (readonly) XCUIApplicationState state;

/*!
 * Waits for the application to become a specific state, giving up after a number of seconds.
 *
 * Returns YES if the application is currently in or transitions to the desired state within the specified
 * timeout period.
 */
- (BOOL)waitForState:(XCUIApplicationState)state timeout:(NSTimeInterval)timeout XCUI_WARN_UNUSED;

/*!
 * Reset the authorization status for a protected resource, such that the system will display the
 * authorization prompt the next time the resource is accessed. The app does not have to be in a running state to
 * use this API. If the app is running, it might get terminated while the reset occurs for some protected resources.
 *
 * Example usage:
 * @code
 * XCUIApplication *app = [XCUIApplication new];
 * [app resetAuthorizationStatusForResource:XCUIProtectedResourcePhotos];
 * [app launch];
 * @endcode
 */
- (void)resetAuthorizationStatusForResource:(XCUIProtectedResource)resource XCUI_PROTECTED_RESOURCES_RESET_API_AVAILABLE;

/*!
 * @method -performAccessibilityAuditWithAuditTypes:issueHandler:error:
 * Runs an accessibility audit on the current view. Generates XCTIssue objects for each issue returned by the audit, and fails the test case by recording each of these issues. Returns an error if the audit failed to run.
 *
 * @param auditTypes Set of audit types which which configure what the audit will test for.
 * @param block An optional filter can be used to determine whether or not an issue should be recorded. To prevent an issue from recorded, return YES to handle it yourself.
 */
- (BOOL)performAccessibilityAuditWithAuditTypes:(XCUIAccessibilityAuditType)auditTypes issueHandler:(nullable XCUI_NOESCAPE BOOL (^)(XCUIAccessibilityAuditIssue *issue))block error:(NSError **)outError API_AVAILABLE(macos(14.0), ios(17.0), tvos(17.0), watchos(10.0)) NS_REFINED_FOR_SWIFT;


@end

NS_ASSUME_NONNULL_END
