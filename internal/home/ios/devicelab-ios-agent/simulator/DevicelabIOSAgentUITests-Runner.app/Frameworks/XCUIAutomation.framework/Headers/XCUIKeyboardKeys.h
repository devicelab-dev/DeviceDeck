//
//  Copyright (c) 2015 Apple Inc. All rights reserved.
//

#import <XCUIAutomation/XCUIAutomationDefines.h>

NS_ASSUME_NONNULL_BEGIN

/*!
    Constants for use with -[XCUIElement typeKey:modifierFlags:],
    representing keys that have no textual representation. These comprise
    the set of control, function, and modifier keys found on most keyboards.
*/

typedef NSString * XCUIKeyboardKey NS_TYPED_EXTENSIBLE_ENUM;

XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyDelete;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyReturn;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyEnter;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyTab;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeySpace;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyEscape;

XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyUpArrow;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyDownArrow;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyLeftArrow;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyRightArrow;

XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF1;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF2;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF3;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF4;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF5;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF6;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF7;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF8;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF9;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF10;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF11;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF12;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF13;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF14;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF15;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF16;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF17;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF18;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyF19;

XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyForwardDelete;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyHome;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyEnd;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyPageUp;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyPageDown;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyClear;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyHelp;

XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyCapsLock;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyShift;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyControl;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyOption;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyCommand;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyRightShift;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyRightControl;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyRightOption;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeyRightCommand;
XCUI_EXPORT XCUIKeyboardKey const XCUIKeyboardKeySecondaryFn;

NS_ASSUME_NONNULL_END
