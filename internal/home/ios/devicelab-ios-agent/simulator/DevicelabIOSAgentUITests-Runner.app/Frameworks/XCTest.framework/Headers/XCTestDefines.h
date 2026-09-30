#if __has_include(<XCTestCore/XCTestDefines.h>)
#import <XCTestCore/XCTestDefines.h>
#else

//
// Copyright (c) 2013-2015 Apple Inc. All rights reserved.
//
// Copyright (c) 1997-2005, Sen:te (Sente SA).  All rights reserved.
//
// Use of this source code is governed by the following license:
// 
// Redistribution and use in source and binary forms, with or without modification, 
// are permitted provided that the following conditions are met:
// 
// (1) Redistributions of source code must retain the above copyright notice, 
// this list of conditions and the following disclaimer.
// 
// (2) Redistributions in binary form must reproduce the above copyright notice, 
// this list of conditions and the following disclaimer in the documentation 
// and/or other materials provided with the distribution.
// 
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS ``AS IS'' 
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED 
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. 
// IN NO EVENT SHALL Sente SA OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, 
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT 
// OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) 
// HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, 
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, 
// EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
// 
// Note: this license is equivalent to the FreeBSD license.
// 
// This notice may not be removed from this file.

#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

#if defined(__cplusplus)
    #define XCT_EXPORT extern "C"
#else
    #define XCT_EXPORT extern
#endif

#ifndef XCT_UI_TESTING_AVAILABLE
#define XCT_UI_TESTING_AVAILABLE 1
#endif

#ifndef XCT_UI_TESTING_AVAILABLE
#define XCT_UI_TESTING_AVAILABLE 0
#endif

#if TARGET_OS_SIMULATOR
#define XCTEST_SIMULATOR_UNAVAILABLE(_msg) __attribute__((availability(ios,unavailable,message=_msg)))
#else
#define XCTEST_SIMULATOR_UNAVAILABLE(_msg)
#endif

#if __has_attribute(warn_unused_result)
#define XCT_WARN_UNUSED __attribute__((__warn_unused_result__))
#else
#define XCT_WARN_UNUSED
#endif

#if __has_attribute(noescape)
#define XCT_NOESCAPE __attribute__((noescape))
#else
#define XCT_NOESCAPE
#endif

#if __has_attribute(weak_import)
#define XCT_WEAK_EXPORT __attribute__((weak_import)) XCT_EXPORT
#else
#define XCT_WEAK_EXPORT XCT_EXPORT
#endif

#define XCT_UNAVAILABLE(msg) __attribute__((unavailable(msg)))

#define XCT_METRIC_API_AVAILABLE API_AVAILABLE(ios(13.0), tvos(13.0), watchos(7.0))

#define XCT_DEPRECATED_WITH_REPLACEMENT(REPLACEMENT) __attribute__((deprecated("Replaced by '" REPLACEMENT "'")))
#define XCT_DEPRECATED_WITH_DIRECT_REPLACEMENT(REPLACEMENT) __attribute__((deprecated("Replaced by '" REPLACEMENT "'", REPLACEMENT)))

#if defined(__swift__)
    #define XCT_DEPRECATED_WITH_SWIFT_REPLACEMENT(REPLACEMENT) XCT_DEPRECATED_WITH_REPLACEMENT(REPLACEMENT)
    #define XCT_TO_BE_DEPRECATED_WITH_SWIFT_REPLACEMENT(REPLACEMENT) \
        __attribute__((availability(macos, deprecated=API_TO_BE_DEPRECATED, message="Replaced by '" REPLACEMENT "'"))) \
        __attribute__((availability(ios, deprecated=API_TO_BE_DEPRECATED, message="Replaced by '" REPLACEMENT "'"))) \
        __attribute__((availability(watchos, deprecated=API_TO_BE_DEPRECATED, message="Replaced by '" REPLACEMENT "'"))) \
        __attribute__((availability(tvos, deprecated=API_TO_BE_DEPRECATED, message="Replaced by '" REPLACEMENT "'")))
#else
    #define XCT_DEPRECATED_WITH_SWIFT_REPLACEMENT(REPLACEMENT)
    #define XCT_TO_BE_DEPRECATED_WITH_SWIFT_REPLACEMENT(REPLACEMENT)
#endif

#if __has_attribute(swift_attr)
#define XCT_SWIFT_MAIN_ACTOR __attribute__((swift_attr("@MainActor")))
#else
#define XCT_SWIFT_MAIN_ACTOR
#endif

#if __has_attribute(swift_attr)
#define XCT_SWIFT_UNAVAILABLE_FROM_ASYNC(msg) __attribute__((swift_attr("@_unavailableFromAsync(message: \"" msg "\")")))
#else
#define XCT_SWIFT_UNAVAILABLE_FROM_ASYNC(msg)
#endif

#ifndef XCT_HEADER_AUDIT_BEGIN
    #if defined(NS_HEADER_AUDIT_BEGIN)
        #define XCT_HEADER_AUDIT_BEGIN(...) NS_HEADER_AUDIT_BEGIN(__VA_ARGS__)
    #else
        #define XCT_HEADER_AUDIT_BEGIN(...) NS_ASSUME_NONNULL_BEGIN
    #endif
#endif

#ifndef XCT_HEADER_AUDIT_END
    #if defined(NS_HEADER_AUDIT_END)
        #define XCT_HEADER_AUDIT_END(...) NS_HEADER_AUDIT_END(__VA_ARGS__)
    #else
        #define XCT_HEADER_AUDIT_END(...) NS_ASSUME_NONNULL_END
    #endif
#endif

#ifndef XCT_SWIFT_SENDABLE
    #if defined(__SWIFT_ATTR_SUPPORTS_SENDABLE_DECLS) && __SWIFT_ATTR_SUPPORTS_SENDABLE_DECLS
        #define XCT_SWIFT_SENDABLE __attribute__((swift_attr("@Sendable")))
    #else
        #define XCT_SWIFT_SENDABLE
    #endif
#endif

NS_ASSUME_NONNULL_END

#endif
