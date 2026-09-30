//
//  Copyright © 2020 Apple Inc. All rights reserved.
//

#import <XCTest/XCTestDefines.h>
#import <XCTest/XCTestErrors.h>

#import <XCTest/XCAbstractTest.h>
#import <XCTest/XCTestAssertions.h>
#import <XCTest/XCTestAssertionsImpl.h>
#import <XCTest/XCTestCase.h>
#import <XCTest/XCTestCaseRun.h>
#import <XCTest/XCTestLog.h>
#import <XCTest/XCTestObserver.h>
#import <XCTest/XCTestObservationCenter.h>
#import <XCTest/XCTestObservation.h>
#import <XCTest/XCTestProbe.h>
#import <XCTest/XCTestRun.h>
#import <XCTest/XCTestSkipping.h>
#import <XCTest/XCTestSkippingImpl.h>
#import <XCTest/XCTestSuite.h>
#import <XCTest/XCTestSuiteRun.h>

#import <XCTest/XCTActivity.h>
#import <XCTest/XCTAttachment.h>
#import <XCTest/XCTAttachmentLifetime.h>
#import <XCTest/XCTContext.h>
#import <XCTest/XCTExpectedFailure.h>
#import <XCTest/XCTIssue.h>
#import <XCTest/XCTMeasureOptions.h>
#import <XCTest/XCTMetric.h>
#import <XCTest/XCTSourceCodeContext.h>

#ifndef XCTEST_DEFINES_DISABLE_WAITERS // NOTE: This conditional is processed before compilation by the unifdef tool. It will not be present in the final framework header
#import <XCTest/XCTestCase+AsynchronousTesting.h>
#import <XCTest/XCTestExpectation.h>
#import <XCTest/XCTDarwinNotificationExpectation.h>
#import <XCTest/XCTKVOExpectation.h>
#import <XCTest/XCTNSNotificationExpectation.h>
#import <XCTest/XCTNSPredicateExpectation.h>
#import <XCTest/XCTWaiter.h>
#endif
