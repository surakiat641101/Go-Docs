package htmldocx

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// declaration is a single CSS "property: value" pair.
type declaration struct {
	prop, value string
	important   bool
}

// compound is one simple selector such as "td.total#sum".
type compound struct {
	tag     string
	id      string
	classes []string
}

func (c compound) matches(n *html.Node) bool {
	if c.tag != "" && c.tag != "*" && c.tag != n.Data {
		return false
	}
	if c.id != "" && attr(n, "id") != c.id {
		return false
	}
	if len(c.classes) > 0 {
		have := strings.Fields(attr(n, "class"))
		for _, want := range c.classes {
			found := false
			for _, h := range have {
				if h == want {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

// selector is a chain of compounds joined by descendant (' ') or child ('>') combinators.
type selector struct {
	parts []compound
	combs []byte // combs[i] joins parts[i] and parts[i+1]
}

func (s *selector) matches(n *html.Node) bool { return s.matchAt(n, len(s.parts)-1) }

func (s *selector) matchAt(n *html.Node, idx int) bool {
	if !s.parts[idx].matches(n) {
		return false
	}
	if idx == 0 {
		return true
	}
	if s.combs[idx-1] == '>' {
		p := parentElement(n)
		return p != nil && s.matchAt(p, idx-1)
	}
	for p := parentElement(n); p != nil; p = parentElement(p) {
		if s.matchAt(p, idx-1) {
			return true
		}
	}
	return false
}

type rule struct {
	sel   selector
	spec  int
	order int
	decls []declaration
}

var commentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// parseStylesheet appends the rules found in css to rules. Selectors using
// unsupported syntax (pseudo-classes, attribute selectors, sibling combinators)
// are skipped rather than applied too broadly.
func parseStylesheet(css string, rules []rule) []rule {
	css = commentRe.ReplaceAllString(css, "")
	for {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			return rules
		}
		prelude := css[:open]
		if i := strings.LastIndexByte(prelude, ';'); i >= 0 { // e.g. "@import x; body"
			prelude = prelude[i+1:]
		}
		prelude = strings.TrimSpace(prelude)
		end := matchBrace(css, open)
		body := css[open+1 : end]
		if end >= len(css) {
			css = ""
		} else {
			css = css[end+1:]
		}

		if strings.HasPrefix(prelude, "@") {
			if strings.HasPrefix(strings.ToLower(prelude), "@media") {
				rules = parseStylesheet(body, rules)
			}
			continue
		}
		decls := parseDeclarations(body)
		for _, s := range strings.Split(prelude, ",") {
			sel, spec, ok := parseSelector(s)
			if !ok {
				continue
			}
			rules = append(rules, rule{sel: sel, spec: spec, order: len(rules), decls: decls})
		}
	}
}

func sortRules(rules []rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].spec != rules[j].spec {
			return rules[i].spec < rules[j].spec
		}
		return rules[i].order < rules[j].order
	})
}

func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(s)
}

func parseSelector(s string) (selector, int, bool) {
	s = strings.ReplaceAll(s, ">", " > ")
	var sel selector
	spec := 0
	pending := byte(' ')
	for _, tok := range strings.Fields(s) {
		if tok == ">" {
			pending = '>'
			continue
		}
		c, sp, ok := parseCompound(tok)
		if !ok {
			return sel, 0, false
		}
		if len(sel.parts) > 0 {
			sel.combs = append(sel.combs, pending)
		}
		sel.parts = append(sel.parts, c)
		spec += sp
		pending = ' '
	}
	return sel, spec, len(sel.parts) > 0
}

func parseCompound(t string) (compound, int, bool) {
	if strings.ContainsAny(t, ":[]+~()") {
		return compound{}, 0, false
	}
	var c compound
	spec := 0
	head := t
	if i := strings.IndexAny(t, ".#"); i >= 0 {
		head, t = t[:i], t[i:]
	} else {
		t = ""
	}
	if head != "" {
		c.tag = strings.ToLower(head)
		if head != "*" {
			spec++
		}
	}
	for t != "" {
		kind := t[0]
		t = t[1:]
		name := t
		if j := strings.IndexAny(t, ".#"); j >= 0 {
			name, t = t[:j], t[j:]
		} else {
			t = ""
		}
		if name == "" {
			return c, 0, false
		}
		if kind == '#' {
			c.id = name
			spec += 10000
		} else {
			c.classes = append(c.classes, name)
			spec += 100
		}
	}
	return c, spec, true
}

func parseDeclarations(s string) []declaration {
	var out []declaration
	for _, part := range splitTopLevel(s, ';') {
		i := strings.IndexByte(part, ':')
		if i < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(part[:i]))
		val := strings.TrimSpace(part[i+1:])
		imp := false
		if j := strings.Index(strings.ToLower(val), "!important"); j >= 0 {
			imp = true
			val = strings.TrimSpace(val[:j])
		}
		if prop == "" || val == "" {
			continue
		}
		out = append(out, declaration{prop: prop, value: val, important: imp})
	}
	return out
}

// splitTopLevel splits s on sep, ignoring separators inside parentheses
// (so data: URIs inside url(...) survive).
func splitTopLevel(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case sep:
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// parseLengthPt converts a CSS length to points. em/rem/% resolve against emBase.
func parseLengthPt(v string, emBase float64) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "0" || v == "auto" {
		return 0, v == "0"
	}
	units := []struct {
		suffix string
		factor float64
	}{
		{"pt", 1}, {"px", 0.75}, {"rem", emBase}, {"em", emBase},
		{"cm", 28.3465}, {"mm", 2.83465}, {"in", 72}, {"pc", 12}, {"%", emBase / 100},
	}
	for _, u := range units {
		if strings.HasSuffix(v, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(v, u.suffix)), 64)
			if err != nil {
				return 0, false
			}
			return n * u.factor, true
		}
	}
	return 0, false
}

var namedColors = map[string]string{
	"black": "000000", "white": "FFFFFF", "red": "FF0000", "green": "008000",
	"blue": "0000FF", "yellow": "FFFF00", "orange": "FFA500", "purple": "800080",
	"gray": "808080", "grey": "808080", "silver": "C0C0C0", "navy": "000080",
	"teal": "008080", "maroon": "800000", "olive": "808000", "lime": "00FF00",
	"aqua": "00FFFF", "cyan": "00FFFF", "fuchsia": "FF00FF", "magenta": "FF00FF",
	"pink": "FFC0CB", "brown": "A52A2A", "gold": "FFD700", "lightgray": "D3D3D3",
	"lightgrey": "D3D3D3", "darkgray": "A9A9A9", "darkgrey": "A9A9A9",
	"whitesmoke": "F5F5F5", "lightblue": "ADD8E6", "lightyellow": "FFFFE0",
	"lightgreen": "90EE90", "darkblue": "00008B", "darkred": "8B0000", "darkgreen": "006400",
}

var rgbRe = regexp.MustCompile(`^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)`)

// parseColor returns a 6-digit uppercase hex colour.
func parseColor(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if c, ok := namedColors[v]; ok {
		return c, true
	}
	if strings.HasPrefix(v, "#") {
		h := v[1:]
		if len(h) == 3 || len(h) == 4 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) == 8 {
			h = h[:6]
		}
		if len(h) == 6 {
			if _, err := strconv.ParseUint(h, 16, 32); err == nil {
				return strings.ToUpper(h), true
			}
		}
		return "", false
	}
	if m := rgbRe.FindStringSubmatch(v); m != nil {
		var rgb [3]int
		for i := range rgb {
			n, _ := strconv.Atoi(m[i+1])
			rgb[i] = min(n, 255)
		}
		return fmt.Sprintf("%02X%02X%02X", rgb[0], rgb[1], rgb[2]), true
	}
	return "", false
}

// colorInShorthand finds the first colour token in a shorthand like
// "1px solid #ccc" or "url(x) rgb(1, 2, 3) no-repeat".
func colorInShorthand(v string) (string, bool) {
	if c, ok := parseColor(v); ok {
		return c, true
	}
	// Strip spaces inside rgb(...) so strings.Fields keeps it as one token.
	v = rgbFuncRe.ReplaceAllStringFunc(v, func(m string) string { return strings.ReplaceAll(m, " ", "") })
	for _, tok := range strings.Fields(v) {
		if c, ok := parseColor(tok); ok {
			return c, true
		}
	}
	return "", false
}

var rgbFuncRe = regexp.MustCompile(`rgba?\([^)]*\)`)
