//
//  Copyright © 2024 Apple Inc. All rights reserved.
//

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

#if defined(__cplusplus)
    #define XCUI_EXPORT extern "C"
#else
    #define XCUI_EXPORT extern
#endif

    #ifndef XCUI_UI_TESTING_AVAILABLE
        #define XCUI_UI_TESTING_AVAILABLE 1
    #endif

#ifndef XCUI_UI_TESTING_AVAILABLE
    #define XCUI_UI_TESTING_AVAILABLE 0
#endif

#if __has_attribute(warn_unused_result)
    #define XCUI_WARN_UNUSED __attribute__((__warn_unused_result__))
#else
    #define XCUI_WARN_UNUSED
#endif

#if __has_attribute(noescape)
    #define XCUI_NOESCAPE __attribute__((noescape))
#else
    #define XCUI_NOESCAPE
#endif

#if TARGET_OS_SIMULATOR
    #define XCUI_SIMULATOR_UNAVAILABLE(_msg) __attribute__((availability(ios,unavailable,message=_msg)))
#else
    #define XCUI_SIMULATOR_UNAVAILABLE(_msg)
#endif

#define XCUI_UNAVAILABLE(msg) __attribute__((unavailable(msg)))

#ifndef XCUI_PROTECTED_RESOURCES_RESET_API_AVAILABLE
    #define XCUI_PROTECTED_RESOURCES_RESET_API_AVAILABLE API_AVAILABLE(macos(10.15.4), ios(13.4), tvos(13.4))
#endif

#define XCUI_DEPRECATED_WITH_REPLACEMENT(REPLACEMENT) __attribute__((deprecated("Replaced by '" REPLACEMENT "'")))
#define XCUI_DEPRECATED_WITH_DIRECT_REPLACEMENT(REPLACEMENT) __attribute__((deprecated("Replaced by '" REPLACEMENT "'", REPLACEMENT)))

#if defined(__swift__)
    #define XCUI_DEPRECATED_WITH_SWIFT_REPLACEMENT(REPLACEMENT) XCUI_DEPRECATED_WITH_REPLACEMENT(REPLACEMENT)
#else
    #define XCUI_DEPRECATED_WITH_SWIFT_REPLACEMENT(REPLACEMENT)
#endif

#if __has_attribute(swift_attr)
    #define XCUI_SWIFT_MAIN_ACTOR __attribute__((swift_attr("@MainActor")))
#else
    #define XCUI_SWIFT_MAIN_ACTOR
#endif

NS_ASSUME_NONNULL_END
