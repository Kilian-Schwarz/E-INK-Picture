package services

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"e-ink-picture/server/internal/services/widgets"
)

// Registration completeness for widget types (F7 AC1/AC2/AC3/AC13).
//
// Adding a widget type means touching eight places. Six of them are frontend
// files with no compiler to catch an omission, so a half-registered widget
// reaches the panel as a missing palette entry, an empty layout dropdown or a
// font size that differs between canvas and panel. These tests derive the list
// of widget types from the Go dispatch itself, so a NEW widget type is checked
// automatically the moment it appears in WidgetTextContent — nobody has to
// remember to extend a list here.

// frontendFile resolves a path relative to the server root (two levels up from
// internal/services) and reads it. Missing file -> skip, never a flaky fail.
func frontendFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("frontend file %s not readable from the test working directory: %v", rel, err)
	}
	return string(data)
}

var (
	jsLineComment  = regexp.MustCompile(`(?m)//.*$`)
	jsBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	htmlComment    = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// stripJSComments removes comments so that a mere mention of a widget type in
// prose cannot satisfy — or inflate — a registration assertion.
//
// LIMITATION, accepted on purpose: this is regex text removal, not a JS parser.
// A "//" inside a string literal (e.g. 'https://example.com') is stripped as if
// it started a comment, which can truncate a line and unbalance the brace count
// braceBlockAfter walks. No file under static/js currently contains "://"; if
// one grows it, the failure mode is a t.Skip or a spurious failure from
// braceBlockAfter — never a false PASS, because stripping can only remove
// tokens an assertion looks for, never invent them. Fix it then by quoting the
// URL differently or splitting the literal; do not build a JS parser here.
func stripJSComments(src string) string {
	return jsLineComment.ReplaceAllString(jsBlockComment.ReplaceAllString(src, ""), "")
}

// dispatchWidgetTypes parses preview.go and returns every "widget_*" case
// label of the WidgetTextContent switch. This is the authoritative list of
// server-backed widget types; parsing the AST means the list cannot go stale.
func dispatchWidgetTypes(t *testing.T) []string {
	t.Helper()
	var types []string
	for typ := range dispatchWidgetFills(t) {
		types = append(types, typ)
	}
	sort.Strings(types)
	return types
}

// dispatchWidgetFills maps each dispatched widget type to the fill*Content
// function that serves it, e.g. widget_progress -> fillProgressContent.
func dispatchWidgetFills(t *testing.T) map[string]string {
	t.Helper()

	pkg := parseServicesPackage(t)

	fills := map[string]string{}
	for _, file := range pkg {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "WidgetTextContent" {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				clause, ok := n.(*ast.CaseClause)
				if !ok {
					return true
				}
				callee := ""
				ast.Inspect(&ast.BlockStmt{List: clause.Body}, func(m ast.Node) bool {
					if sel, ok := m.(*ast.SelectorExpr); ok && strings.HasPrefix(sel.Sel.Name, "fill") {
						callee = sel.Sel.Name
					}
					return true
				})
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					val, err := strconv.Unquote(lit.Value)
					if err == nil && strings.HasPrefix(val, "widget_") {
						fills[val] = callee
					}
				}
				return true
			})
		}
	}

	if len(fills) == 0 {
		t.Fatal("no widget_* case labels found in WidgetTextContent — did the dispatch switch change shape?")
	}
	return fills
}

// parseServicesPackage parses the non-test sources of this package. The
// fill*Content functions are spread over several files (fillProgressContent
// lives in widget_progress.go), so parsing preview.go alone is not enough.
func parseServicesPackage(t *testing.T) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse services package: %v", err)
	}
	pkg, ok := pkgs["services"]
	if !ok {
		t.Fatal("package services not found in the current directory")
	}
	return pkg.Files
}

// funcReadsProp reports whether the named function mentions the given property
// name as a string literal, i.e. whether the widget actually consumes that
// property. This is how the test decides which registration points APPLY to a
// widget instead of demanding all of them from every type.
func funcReadsProp(t *testing.T, funcName, propName string) bool {
	t.Helper()
	if funcName == "" {
		return false
	}
	found := false
	for _, file := range parseServicesPackage(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != funcName {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				if v, err := strconv.Unquote(lit.Value); err == nil && v == propName {
					found = true
				}
				return true
			})
		}
	}
	return found
}

// TestDispatchWidgetTypesFound guards the AST parser itself: if it silently
// stopped finding types, every registration assertion below would pass
// vacuously.
func TestDispatchWidgetTypesFound(t *testing.T) {
	types := dispatchWidgetTypes(t)
	for _, want := range []string{"widget_clock", "widget_weather", "widget_progress"} {
		if !containsString(types, want) {
			t.Errorf("dispatch type %q not discovered by the AST parser; found %v", want, types)
		}
	}
	if len(types) < 8 {
		t.Errorf("only %d widget types discovered (%v) — parser likely broken", len(types), types)
	}
}

// TestWidgetRegistrationCompleteness is AC3: every widget type served by the
// Go dispatch must also be registered in all remaining seven places. A type
// present in some places but not all fails here with the exact missing point.
func TestWidgetRegistrationCompleteness(t *testing.T) {
	elementFactory := stripJSComments(frontendFile(t, "static/js/element-factory.js"))
	propertiesPanel := stripJSComments(frontendFile(t, "static/js/properties-panel.js"))
	widgetsJS := stripJSComments(frontendFile(t, "static/js/widgets.js"))
	designerHTML := htmlComment.ReplaceAllString(frontendFile(t, "templates/designer.html"), "")

	// Registration point 1+2 and 3 live inside specific functions; slicing the
	// file down to the enclosing block keeps the assertion honest instead of
	// accepting a match anywhere in the file.
	defaultSizes := jsObjectBlock(t, elementFactory, "defaultSizes")
	defaultProps := jsFunctionBody(t, elementFactory, "getDefaultProperties")
	widgetPropDefs := jsFunctionBody(t, propertiesPanel, "getWidgetPropertyDefs")
	previewContent := jsFunctionBody(t, widgetsJS, "getPreviewContent")
	defaultLayouts := jsFunctionBody(t, widgetsJS, "getDefaultLayout")
	fontSizes := jsFunctionBody(t, widgetsJS, "getPreviewFontSize")

	fills := dispatchWidgetFills(t)
	for _, widgetType := range dispatchWidgetTypes(t) {
		t.Run(widgetType, func(t *testing.T) {
			checks := []struct {
				point string
				where string
				body  string
				token string
			}{
				{"1", "element-factory.js defaultSizes", defaultSizes, widgetType + ":"},
				{"2", "element-factory.js getDefaultProperties", defaultProps, widgetType + ":"},
				{"3", "properties-panel.js getWidgetPropertyDefs", widgetPropDefs, widgetType + ":"},
				{"4", "widgets.js getPreviewContent", previewContent, "'" + widgetType + "'"},
				{"5b", "widgets.js getPreviewFontSize", fontSizes, widgetType + ":"},
				{"6", "designer.html widget palette", designerHTML, `data-type="` + widgetType + `"`},
			}
			for _, c := range checks {
				if !strings.Contains(c.body, c.token) {
					t.Errorf("registration point %s missing: %q not found in %s", c.point, c.token, c.where)
				}
			}

			// Point 7: the Go font-size table (see also the parity test below).
			if _, ok := widgetDefaultFontSizes[widgetType]; !ok {
				t.Errorf("registration point 7 missing: %q absent from widgetDefaultFontSizes in preview.go", widgetType)
			}

			// Points 5a and 8 only APPLY to widgets that actually consume the
			// property. widget_custom and widget_hass read neither "layout" nor
			// "customTemplate" — they have no layouts and no placeholders by
			// design, and demanding entries for them would be noise.
			//
			// Both directions are asserted, so neither a missing registration
			// nor a stale exemption can survive: the moment a fill*Content
			// starts reading "layout", its layouts.go entry becomes mandatory.
			fill := fills[widgetType]

			hasLayouts := len(widgets.GetLayouts(widgetType)) > 1 || widgets.GetLayouts(widgetType)[0].ID != "default"
			if funcReadsProp(t, fill, "layout") {
				if !hasLayouts {
					t.Errorf("registration point 8 missing: %s reads props[\"layout\"] but %q is absent from allLayouts in layouts.go (GetLayouts returned only the generic fallback)", fill, widgetType)
				}
				if !strings.Contains(defaultLayouts, widgetType+":") {
					t.Errorf("registration point 5a missing: %s reads props[\"layout\"] but %q is absent from getDefaultLayout in widgets.js", fill, widgetType)
				}
			} else if hasLayouts {
				t.Errorf("%q is registered in allLayouts but %s never reads props[\"layout\"] — dead layout entry, or the fill function lost its layout handling", widgetType, fill)
			}

			hasPlaceholders := len(widgets.Placeholders(widgetType)) > 0
			if funcReadsProp(t, fill, "customTemplate") && !hasPlaceholders {
				t.Errorf("registration point 8 missing: %s reads props[\"customTemplate\"] but %q is absent from allPlaceholders in layouts.go — the properties panel would offer no placeholders", fill, widgetType)
			}
		})
	}
}

// knownDeadPlaceholderWidgets documents a PRE-EXISTING defect, it does not
// bless it: these widget types are listed in allPlaceholders, so the properties
// panel offers their %...% placeholders to the user, but their fill*Content
// never reads props["customTemplate"] and therefore never substitutes them. A
// user typing "%next_event%" gets the literal text back.
//
// Out of scope for F7 (fixing it changes rendered output). It is pinned as an
// exact set so the gap cannot grow silently and cannot be forgotten once fixed:
// adding a dead entry OR fixing one of these fails TestDeadPlaceholderRegistry.
var knownDeadPlaceholderWidgets = map[string]bool{
	"widget_calendar": true,
	"widget_news":     true,
}

// TestDeadPlaceholderRegistry asserts the set of widget types that advertise
// placeholders without consuming customTemplate is EXACTLY the known set.
func TestDeadPlaceholderRegistry(t *testing.T) {
	fills := dispatchWidgetFills(t)

	dead := map[string]bool{}
	for _, widgetType := range dispatchWidgetTypes(t) {
		if len(widgets.Placeholders(widgetType)) > 0 && !funcReadsProp(t, fills[widgetType], "customTemplate") {
			dead[widgetType] = true
		}
	}

	for widgetType := range dead {
		if !knownDeadPlaceholderWidgets[widgetType] {
			t.Errorf("NEW dead placeholder entry: %q is in allPlaceholders but %s never reads props[\"customTemplate\"] — the panel would offer placeholders that are never substituted", widgetType, fills[widgetType])
		}
	}
	for widgetType := range knownDeadPlaceholderWidgets {
		if !dead[widgetType] {
			t.Errorf("%q is no longer a dead placeholder entry — remove it from knownDeadPlaceholderWidgets", widgetType)
		}
	}
}

// jsObjectBlock returns the source of `name = { ... }` / `name: { ... }` by
// brace matching from the first '{' after the identifier.
func jsObjectBlock(t *testing.T, src, name string) string {
	t.Helper()
	return braceBlockAfter(t, src, regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\s*[:=]\s*\{`))
}

// jsFunctionBody returns the body of a method declared as `name(...) {` or
// `name: function(...) {` / `async name(...) {`.
//
// The character class excludes newlines and semicolons so the pattern cannot
// match a CALL site (`this.getDefaultProperties(type);`) and then run on to
// some unrelated block further down the file — that silently yields the wrong
// body and makes every assertion against it fail.
func jsFunctionBody(t *testing.T, src, name string) string {
	t.Helper()
	return braceBlockAfter(t, src, regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\s*[:(][^{;\n]*\{`))
}

func braceBlockAfter(t *testing.T, src string, re *regexp.Regexp) string {
	t.Helper()
	loc := re.FindStringIndex(src)
	if loc == nil {
		t.Skipf("could not locate %s in the JS source — file reformatted? (this test is an early warning, not a gate)", re)
	}
	start := strings.Index(src[loc[0]:loc[1]], "{") + loc[0]
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Skipf("unbalanced braces after %s in the JS source", re)
	return ""
}

// TestProgressCanvasPanelParity is AC2 on the frontend side.
//
// widget_progress is a PASSTHROUGH widget: widgets.js must render the server's
// content string verbatim, so canvas and panel cannot disagree by
// construction. The dangerous regression is not a wrong number — it is someone
// "helpfully" reimplementing the period math in JS so the canvas stops
// waiting for the server. This test fails on exactly that.
func TestProgressCanvasPanelParity(t *testing.T) {
	raw := frontendFile(t, "static/js/widgets.js")
	src := stripJSComments(raw)

	previewContent := jsFunctionBody(t, src, "getPreviewContent")

	// 1. widget_progress must sit in the passthrough branch, i.e. the same
	//    case group whose body returns liveData.content untouched.
	branch := passthroughBranch(t, previewContent)
	if !strings.Contains(branch, "'widget_progress'") {
		t.Errorf("widget_progress is not in the getPreviewContent passthrough case group; the passthrough group is:\n%s", branch)
	}
	if !strings.Contains(branch, "liveData.content") {
		t.Errorf("the passthrough branch no longer returns liveData.content verbatim:\n%s", branch)
	}

	// 2. No JS-side content builder for progress. Every other widget with
	//    client-side formatting has a build*Content function; progress must
	//    not grow one.
	for _, forbidden := range []string{
		"buildProgressContent",
		"progressContent",
		"buildProgress",
	} {
		if strings.Contains(src, forbidden) {
			t.Errorf("widgets.js defines %q — widget_progress must stay a passthrough widget; WidgetTextContent in widget_progress.go is the single source", forbidden)
		}
	}

	// 3. No JS-side progress math. These are the primitives a reimplementation
	//    of progressBar/progressRatio would need.
	for _, forbidden := range []string{
		`repeat('#')`,
		`'#'.repeat`,
		`"#".repeat`,
		"barWidth",
		"getDayOfYear",
	} {
		if strings.Contains(src, forbidden) {
			t.Errorf("widgets.js contains %q — that looks like progress math moving into the frontend, which would fork the single source in widget_progress.go", forbidden)
		}
	}

	// 4. widget_progress may appear in widgets.js only in its three registered
	//    roles. Any further occurrence is a second code path by definition.
	wantForms := []string{
		"case 'widget_progress':",         // getPreviewContent passthrough
		"widget_progress: 'bar_percent',", // getDefaultLayout
		"widget_progress: 18,",            // getPreviewFontSize
	}
	occurrences := strings.Count(src, "widget_progress")
	if occurrences != len(wantForms) {
		t.Errorf("widget_progress appears %d times in widgets.js (comments stripped), want exactly %d — one per registered role",
			occurrences, len(wantForms))
	}
	for _, form := range wantForms {
		if !strings.Contains(src, form) {
			t.Errorf("expected registered form %q not found in widgets.js", form)
		}
	}

	// 5. The Go dispatch really is the single source for the panel: the
	//    exported entry point and the drawElement path must agree. Both read
	//    fillProgressContent, so this pins that they still do.
	svc := newProgressService(goldenNow)
	// barWidth MUST be float64: GetPropInt decodes only float64 and string (the
	// shapes a JSON design file produces), so an untyped int constant is
	// silently discarded and the default applies instead of the stated value.
	props := map[string]any{"period": "year", "layout": "bar_percent", "barWidth": float64(20), "timezone": "Europe/Berlin"}
	content, ok := svc.WidgetTextContent("widget_progress", props)
	if !ok {
		t.Fatal("WidgetTextContent(widget_progress) returned ok == false")
	}
	if direct := svc.fillProgressContent(props); direct != content {
		t.Errorf("WidgetTextContent = %q but fillProgressContent = %q — the dispatch is no longer a pure pass-through", content, direct)
	}
	if content != "[##########----------] 54%" {
		t.Errorf("content = %q, want the value pinned by the golden design and the spec", content)
	}
}

// passthroughBranch extracts the fall-through case group of getPreviewContent
// that ends in the liveData.content return.
func passthroughBranch(t *testing.T, previewContent string) string {
	t.Helper()
	idx := strings.Index(previewContent, "liveData.content")
	if idx < 0 {
		t.Fatal("getPreviewContent no longer references liveData.content — the passthrough pattern is gone")
	}
	// The passthrough group is the contiguous run of `case` labels that falls
	// through into the `return (liveData ...)` statement. Walk back to the
	// return, then to the end of the previous statement/block opener; what lies
	// between is exactly that run of labels.
	retStart := strings.LastIndex(previewContent[:idx], "return")
	if retStart < 0 {
		t.Fatal("no return statement precedes liveData.content in getPreviewContent")
	}
	groupStart := strings.LastIndexAny(previewContent[:retStart], ";{")
	if groupStart < 0 {
		groupStart = 0
	}
	return previewContent[groupStart+1 : idx+len("liveData.content")]
}

// jsFontSizeDefaults parses the getPreviewFontSize defaults table out of
// widgets.js.
func jsFontSizeDefaults(t *testing.T, widgetsJS string) map[string]int {
	t.Helper()
	body := jsFunctionBody(t, widgetsJS, "getPreviewFontSize")
	re := regexp.MustCompile(`(widget_\w+):\s*(\d+)`)
	out := map[string]int{}
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		size, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("font size %q for %s is not an integer: %v", m[2], m[1], err)
		}
		out[m[1]] = size
	}
	if len(out) == 0 {
		t.Skip("no widget font-size entries parsed from widgets.js — block reformatted?")
	}
	return out
}

// TestWidgetDefaultFontSizesMatchFrontend is AC13: preview.go and widgets.js
// hold the same "widget type -> default font size" table twice. When they
// drift, the designer canvas and the rendered panel disagree on glyph size for
// the same element and nothing else notices.
func TestWidgetDefaultFontSizesMatchFrontend(t *testing.T) {
	widgetsJS := stripJSComments(frontendFile(t, "static/js/widgets.js"))
	jsSizes := jsFontSizeDefaults(t, widgetsJS)

	goSizes := map[string]int{}
	for typ, size := range widgetDefaultFontSizes {
		if strings.HasPrefix(typ, "widget_") {
			goSizes[typ] = size
		}
	}

	for typ, goSize := range goSizes {
		jsSize, ok := jsSizes[typ]
		if !ok {
			t.Errorf("%s = %d in preview.go but missing from widgets.js getPreviewFontSize", typ, goSize)
			continue
		}
		if jsSize != goSize {
			t.Errorf("%s: preview.go says %d, widgets.js says %d", typ, goSize, jsSize)
		}
	}
	for typ, jsSize := range jsSizes {
		if _, ok := goSizes[typ]; !ok {
			t.Errorf("%s = %d in widgets.js but missing from widgetDefaultFontSizes in preview.go", typ, jsSize)
		}
	}

	// Every dispatch-backed widget type must be listed EXPLICITLY on both
	// sides. The two fallbacks differ — Go falls back to
	// widgetFallbackFontSize, widgets.js falls back to `|| 14` — so a type
	// that relies on either fallback renders at a different size on the canvas
	// than on the panel.
	for _, widgetType := range dispatchWidgetTypes(t) {
		if _, ok := goSizes[widgetType]; !ok {
			t.Errorf("%s relies on the Go fallback (%d); widgets.js falls back to 14, so canvas and panel would disagree", widgetType, widgetFallbackFontSize)
		}
		if _, ok := jsSizes[widgetType]; !ok {
			t.Errorf("%s relies on the widgets.js fallback (14); preview.go falls back to %d, so canvas and panel would disagree", widgetType, widgetFallbackFontSize)
		}
	}

	if t.Failed() {
		t.Log(fmt.Sprintf("go table: %v\njs table: %v", goSizes, jsSizes))
	}
}
