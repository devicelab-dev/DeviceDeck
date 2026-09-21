package capture

import (
	"fmt"
	"strings"

	"github.com/devicelab-dev/DeviceDeck/internal/runner"
	"github.com/devicelab-dev/DeviceDeck/internal/uisem"
)

// DesertFinding is one capture-time warning that a screen has few or no
// durable selectors, naming the framework and the exact fix its developer
// should apply. The mirror's whole value is deterministic selectors, so a
// screen that offers none is a hole the reviewer must see while capturing,
// not discover when a replay flakes.
type DesertFinding struct {
	// Framework is the UI toolkit detected ("Flutter", "React Native",
	// "Jetpack Compose", "Unity", "WebView"), or "" for a generic desert.
	Framework string
	// Message is the developer-facing fix, ready to show or write as a
	// comment at the top of the flow.
	Message string
}

// DesertLint inspects a UI tree and reports the frameworks whose screens
// cannot be addressed by a durable selector, with the fix for each.
//
// A screen that is fully addressable — every actionable element carries an
// identifier — produces nothing, whatever framework built it. Detection
// keys on the raw class name (Android) carried through normalisation, so a
// React Native desert reads differently from a Flutter or Compose one;
// Unity and WebView are their own cases because the problem there is an
// absent tree, not a missing id on a present element.
func DesertLint(tree []runner.Node) []DesertFinding {
	var f []DesertFinding
	classes := classNames(tree)

	// Unity renders the whole app as one opaque surface: there is nothing
	// to select regardless of identifiers, so its presence alone is the
	// finding, independent of the id-bearing check below.
	if hasAny(classes, "com.unity3d", "UnityPlayer") {
		f = append(f, DesertFinding{"Unity", unityFix})
	}

	// The id-bearing case: count actionable elements and how many lack an
	// identifier. A fully-addressed screen (every actionable element has an
	// id) is not a desert even when a framework marker is present, so a
	// properly-instrumented React Native app is left alone.
	total, unaddressed := addressability(tree)
	if unaddressed == 0 {
		return f
	}
	return append(f, toolkitFinding(tree, classes, unaddressed, total))
}

// toolkitFinding names the toolkit responsible for the unaddressed elements,
// most specific first: a WebView's web controls never carry native ids, and
// each native framework has its own fix; anything else is a generic reminder.
func toolkitFinding(tree []runner.Node, classes []string, unaddressed, total int) DesertFinding {
	switch {
	case hasAny(classes, "io.flutter", "FlutterView"):
		return DesertFinding{"Flutter", desertMsg(flutterFix, unaddressed, total)}
	case hasAny(classes, "androidx.compose", "ComposeView"):
		return DesertFinding{"Jetpack Compose", desertMsg(composeFix, unaddressed, total)}
	case hasAny(classes, "com.facebook.react"):
		return DesertFinding{"React Native", desertMsg(rnFix, unaddressed, total)}
	case hasWebView(tree):
		return DesertFinding{"WebView", desertMsg(webViewFix, unaddressed, total)}
	default:
		return DesertFinding{"", desertMsg(genericFix, unaddressed, total)}
	}
}

// classNames collects every node's raw class for marker matching.
func classNames(tree []runner.Node) []string {
	out := make([]string, 0, len(tree))
	for i := range tree {
		out = append(out, tree[i].ClassName)
	}
	return out
}

// hasAny reports whether any class name contains one of the markers.
func hasAny(classes []string, markers ...string) bool {
	for _, c := range classes {
		for _, m := range markers {
			if strings.Contains(c, m) {
				return true
			}
		}
	}
	return false
}

// hasWebView reports a web view in the tree, by mapped type or raw class.
func hasWebView(tree []runner.Node) bool {
	for i := range tree {
		if tree[i].Type == "WebView" || strings.Contains(tree[i].ClassName, "webkit.WebView") {
			return true
		}
	}
	return false
}

// addressability counts actionable elements and how many of them lack an
// identifier — the raw material of a desert.
func addressability(tree []runner.Node) (total, unaddressed int) {
	for i := range tree {
		n := &tree[i]
		if !uisem.Interactive(n.Type) {
			continue
		}
		total++
		if n.Identifier == "" {
			unaddressed++
		}
	}
	return total, unaddressed
}

// desertMsg suffixes a fix with the count it applies to, so a reviewer
// sees how much of the screen is affected.
func desertMsg(fix string, unaddressed, total int) string {
	return fmt.Sprintf("%s (%d of %d actionable elements have no identifier)", fix, unaddressed, total)
}

const flutterFix = "Flutter screen with no durable ids: add Semantics(identifier: '…') to widgets " +
	"(surfaces as resource-id / accessibilityIdentifier). Widget Keys are invisible to accessibility; " +
	"add semanticLabel to icon buttons."

const composeFix = "Jetpack Compose without testTagsAsResourceId: set " +
	"Modifier.semantics { testTagsAsResourceId = true } on the root, then Modifier.testTag(\"…\") " +
	"appears as a resource-id."

const rnFix = "React Native views without testID: add testID (iOS accessibilityIdentifier, " +
	"Android resource-id), plus accessibilityRole/accessibilityLabel; don't nest touchables inside " +
	"an accessible={true} view."

const unityFix = "Unity renders one opaque surface with no elements to select: build an " +
	"AccessibilityHierarchy (Unity 6+), or expect a vision-only fallback for this screen."

const webViewFix = "WebView content: the page's id/testid do not cross the native bridge. Give the " +
	"controls visible labels or aria-label so text selectors can reach them."

const genericFix = "Screen has actionable elements with no durable id: add a test identifier " +
	"(iOS accessibilityIdentifier, Android resource-id / Compose testTag, Flutter Semantics.identifier, " +
	"React Native testID)."
