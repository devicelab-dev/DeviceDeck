#if __has_include(<XCTestCore/XCTestAssertionsImpl.h>)
#import <XCTestCore/XCTestAssertionsImpl.h>
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

#import <XCTest/XCTestDefines.h>

#if defined(__cplusplus)
#include <type_traits>
#endif

@class XCTestCase;

NS_ASSUME_NONNULL_BEGIN

@interface _XCTestCaseInterruptionException : NSException
@end

XCT_EXPORT void _XCTFailureHandler(XCTestCase * _Nullable test, BOOL expected, const char *filePath, NSUInteger lineNumber, NSString *condition, NSString * _Nullable format, ...) NS_FORMAT_FUNCTION(6,7);

XCT_EXPORT void _XCTPreformattedFailureHandler(XCTestCase * _Nullable test, BOOL expected, NSString *filePath, NSUInteger lineNumber, NSString *condition, NSString *message);

#define _XCTRegisterFailure(test, condition, ...) \
    _XCTFailureHandler(test, YES, __FILE__, __LINE__, condition, @"" __VA_ARGS__)

#define _XCTRegisterUnexpectedFailure(test, condition, ...) \
    _XCTFailureHandler(test, NO, __FILE__, __LINE__, condition, @"" __VA_ARGS__)

#pragma mark -

typedef NS_ENUM(NSUInteger, _XCTAssertionType) {
    _XCTAssertion_Fail,
    _XCTAssertion_Nil,
    _XCTAssertion_NotNil,
    _XCTAssertion_EqualObjects,
    _XCTAssertion_NotEqualObjects,
    _XCTAssertion_Equal,
    _XCTAssertion_NotEqual,
    _XCTAssertion_EqualWithAccuracy,
    _XCTAssertion_NotEqualWithAccuracy,
    _XCTAssertion_GreaterThan,
    _XCTAssertion_GreaterThanOrEqual,
    _XCTAssertion_LessThan,
    _XCTAssertion_LessThanOrEqual,
    _XCTAssertion_True,
    _XCTAssertion_False,
    _XCTAssertion_Throws,
    _XCTAssertion_ThrowsSpecific,
    _XCTAssertion_ThrowsSpecificNamed,
    _XCTAssertion_NoThrow,
    _XCTAssertion_NoThrowSpecific,
    _XCTAssertion_NoThrowSpecificNamed,
    _XCTAssertion_Unwrap,
    _XCTAssertion_Identical,
    _XCTAssertion_NotIdentical,
};

XCT_EXPORT NSString * _XCTFailureFormat (_XCTAssertionType assertionType, NSUInteger formatIndex);

#define _XCTFailureDescription(assertion_type, format_index, ...) \
    _Pragma("clang diagnostic push") \
    _Pragma("clang diagnostic ignored \"-Wformat-nonliteral\"") \
    [NSString stringWithFormat:_XCTFailureFormat(assertion_type, format_index), @"" __VA_ARGS__] \
    _Pragma("clang diagnostic pop")

#pragma mark -

XCT_EXPORT NSString * _XCTDescriptionForValue (NSValue *value);

#if defined(__cplusplus) && __has_feature(cxx_exceptions)
#define _XCT_TRY try
#define _XCT_CATCHABLE_TYPE(T) std::conditional<std::is_convertible<T *, id>::value, T *, const T& >::type
#define _XCT_CATCH(T) catch(T)
#define _XCT_THROW throw
#else
#define _XCT_TRY @try
#define _XCT_CATCHABLE_TYPE(T) T *
#define _XCT_CATCH(T) @catch(T)
#define _XCT_THROW @throw
#endif

#define _XCT_ASSERT_BEGIN \
    _Pragma("clang diagnostic push") \
    _Pragma("clang diagnostic ignored \"-Wgnu-statement-expression-from-macro-expansion\"") \
    ({ \
    _Pragma("clang diagnostic pop")
#define _XCT_ASSERT_END })

XCT_WEAK_EXPORT NSString * _XCTGetCurrentExceptionReasonWithFallback (NSString *_Nullable fallback);

#define _XCTGetCurrentExceptionReason() _XCTGetCurrentExceptionReasonWithFallback(nil)

NS_ASSUME_NONNULL_END

#pragma mark -

#define _XCTPrimitiveFail(test, ...) \
    _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_Fail, 0, ""), __VA_ARGS__)

#define _XCTPrimitiveAssertNil(test, expression, expressionStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        id expressionValue = (expression); \
        if (expressionValue != nil) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_Nil, 0, expressionStr, expressionValue), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_Nil, 1, expressionStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNotNil(test, expression, expressionStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        id expressionValue = (expression); \
        if (expressionValue == nil) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NotNil, 0, expressionStr), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_NotNil, 1, expressionStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertTrue(test, expression, expressionStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        BOOL expressionValue = !!(expression); \
        if (!expressionValue) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_True, 0, expressionStr), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_True, 1, expressionStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertFalse(test, expression, expressionStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        BOOL expressionValue = !!(expression); \
        if (expressionValue) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_False, 0, expressionStr), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_False, 1, expressionStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertEqualObjects(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        id expressionValue1 = (expression1); \
        id expressionValue2 = (expression2); \
        if ((expressionValue1 != expressionValue2) && ![((id<NSObject>)expressionValue1) isEqual:expressionValue2]) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_EqualObjects, 0, expressionStr1, expressionStr2, expressionValue1, expressionValue2), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_EqualObjects, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNotEqualObjects(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        id expressionValue1 = (expression1); \
        id expressionValue2 = (expression2); \
        if ((expressionValue1 == expressionValue2) || [((id<NSObject>)expressionValue1) isEqual:expressionValue2]) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NotEqualObjects, 0, expressionStr1, expressionStr2, expressionValue1, expressionValue2), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_NotEqualObjects, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertEqual(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        if (expressionValue1 != expressionValue2) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_Equal, 0, expressionStr1, expressionStr2, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_Equal, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNotEqual(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        if (expressionValue1 == expressionValue2) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NotEqual, 0, expressionStr1, expressionStr2, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_NotEqual, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertIdentical(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        id expressionValue1 = (expression1); \
        id expressionValue2 = (expression2); \
        if (expressionValue1 != expressionValue2) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_Identical, 0, expressionStr1, expressionStr2, expressionValue1, expressionValue2), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_Identical, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNotIdentical(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        id expressionValue1 = (expression1); \
        id expressionValue2 = (expression2); \
        if (expressionValue1 == expressionValue2) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NotIdentical, 0, expressionStr1, expressionStr2, expressionValue1, expressionValue2), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_NotIdentical, 2, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertEqualWithAccuracy(test, expression1, expressionStr1, expression2, expressionStr2, accuracy, accuracyStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        __typeof__(accuracy) accuracyValue = (accuracy); \
        if (isnan(expressionValue1) || isnan(expressionValue2) || ((MAX(expressionValue1, expressionValue2) - MIN(expressionValue1, expressionValue2)) > accuracyValue)) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            NSValue *accuracyBox = [NSValue value:&accuracyValue withObjCType:@encode(__typeof__(accuracy))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_EqualWithAccuracy, 0, expressionStr1, expressionStr2, accuracyStr, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2), _XCTDescriptionForValue(accuracyBox)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_EqualWithAccuracy, 1, expressionStr1, expressionStr2, accuracyStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNotEqualWithAccuracy(test, expression1, expressionStr1, expression2, expressionStr2, accuracy, accuracyStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        __typeof__(accuracy) accuracyValue = (accuracy); \
        if (!isnan(expressionValue1) && !isnan(expressionValue2) && ((MAX(expressionValue1, expressionValue2) - MIN(expressionValue1, expressionValue2)) <= accuracyValue)) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            NSValue *accuracyBox = [NSValue value:&accuracyValue withObjCType:@encode(__typeof__(accuracy))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NotEqualWithAccuracy, 0, expressionStr1, expressionStr2, accuracyStr, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2), _XCTDescriptionForValue(accuracyBox)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_NotEqualWithAccuracy, 1, expressionStr1, expressionStr2, accuracyStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertGreaterThan(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        if (expressionValue1 <= expressionValue2) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_GreaterThan, 0, expressionStr1, expressionStr2, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_GreaterThan, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertGreaterThanOrEqual(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        if (expressionValue1 < expressionValue2) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_GreaterThanOrEqual, 0, expressionStr1, expressionStr2, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_GreaterThanOrEqual, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertLessThan(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        if (expressionValue1 >= expressionValue2) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_LessThan, 0, expressionStr1, expressionStr2, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_LessThan, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertLessThanOrEqual(test, expression1, expressionStr1, expression2, expressionStr2, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        __typeof__(expression1) expressionValue1 = (expression1); \
        __typeof__(expression2) expressionValue2 = (expression2); \
        if (expressionValue1 > expressionValue2) { \
            NSValue *expressionBox1 = [NSValue value:&expressionValue1 withObjCType:@encode(__typeof__(expression1))]; \
            NSValue *expressionBox2 = [NSValue value:&expressionValue2 withObjCType:@encode(__typeof__(expression2))]; \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_LessThanOrEqual, 0, expressionStr1, expressionStr2, _XCTDescriptionForValue(expressionBox1), _XCTDescriptionForValue(expressionBox2)), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (_XCTestCaseInterruptionException *interruption) { [interruption raise]; } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterUnexpectedFailure(test, _XCTFailureDescription(_XCTAssertion_LessThanOrEqual, 1, expressionStr1, expressionStr2, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertThrows(test, expression, expressionStr, ...) \
_XCT_ASSERT_BEGIN \
    BOOL _didThrow = NO; \
    _XCT_TRY { \
        (void)(expression); \
    } \
    _XCT_CATCH (...) { \
        _didThrow = YES; \
    } \
    if (!_didThrow) { \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_Throws, 0, expressionStr), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertThrowsSpecific(test, expression, expressionStr, exception_class, ...) \
_XCT_ASSERT_BEGIN \
    BOOL _didThrow = NO; \
    _XCT_TRY { \
        (void)(expression); \
    } \
    _XCT_CATCH (_XCT_CATCHABLE_TYPE(exception_class)) { \
        _didThrow = YES; \
    } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason; \
        _didThrow = YES; \
        _xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_ThrowsSpecific, 0, expressionStr, @#exception_class, @#exception_class, _xct_reason), __VA_ARGS__); \
    } \
    if (!_didThrow) { \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_ThrowsSpecific, 2, expressionStr, @#exception_class), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertThrowsSpecificNamed(test, expression, expressionStr, exception_class, exception_name, ...) \
_XCT_ASSERT_BEGIN \
    BOOL _didThrow = NO; \
    _XCT_TRY { \
        _XCT_TRY { \
            (void)(expression); \
        } \
        _XCT_CATCH (exception_class *exception) { \
            _didThrow = YES; \
            if (![exception_name isEqualToString:[exception name]]) { \
                _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_ThrowsSpecificNamed, 0, expressionStr, @#exception_class, exception_name, @#exception_class, [exception name], [exception reason]), __VA_ARGS__); \
            } \
        } \
    } \
    _XCT_CATCH (NSException *exception) { \
        _didThrow = YES; \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_ThrowsSpecificNamed, 1, expressionStr, @#exception_class, exception_name, @#exception_class, [exception name], [exception reason]), __VA_ARGS__); \
    } \
    _XCT_CATCH (...) { \
        _didThrow = YES; \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_ThrowsSpecificNamed, 2, expressionStr, @#exception_class, exception_name), __VA_ARGS__); \
    } \
    if (!_didThrow) { \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_ThrowsSpecificNamed, 3, expressionStr, @#exception_class, exception_name), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNoThrow(test, expression, expressionStr, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        (void)(expression); \
    } \
    _XCT_CATCH (...) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NoThrow, 0, expressionStr, _xct_reason), __VA_ARGS__); \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNoThrowSpecific(test, expression, expressionStr, exception_class, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        (void)(expression); \
    } \
    _XCT_CATCH (_XCT_CATCHABLE_TYPE(exception_class)) { \
        NSString *_xct_reason = _XCTGetCurrentExceptionReason(); \
        _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NoThrowSpecific, 0, expressionStr, @#exception_class, @#exception_class, _xct_reason), __VA_ARGS__); \
    } \
    _XCT_CATCH (...) { \
        ; \
    } \
_XCT_ASSERT_END

#define _XCTPrimitiveAssertNoThrowSpecificNamed(test, expression, expressionStr, exception_class, exception_name, ...) \
_XCT_ASSERT_BEGIN \
    _XCT_TRY { \
        (void)(expression); \
    } \
    _XCT_CATCH (exception_class *exception) { \
        if ([exception_name isEqualToString:[exception name]]) { \
            _XCTRegisterFailure(test, _XCTFailureDescription(_XCTAssertion_NoThrowSpecificNamed, 0, expressionStr, @#exception_class, exception_name, @#exception_class, [exception name], [exception reason]), __VA_ARGS__); \
        } \
    } \
    _XCT_CATCH (...) { \
        ; \
    } \
_XCT_ASSERT_END

#endif
