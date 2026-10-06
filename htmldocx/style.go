package htmldocx

import (
	"strconv"
	"strings"
)

// computed is the resolved style of an element. Fields above the blank line
// are inherited by children; the rest are reset by inherit().
type computed struct {
	font         string
	sizePt       float64
	bold         bool
	italic       bool
	underline    bool
	strike       bool
	color        string
	highlight    string // background of inline content
	vertAlign    string // superscript | subscript
	align        string // left | center | right | both
	pre          bool
	indentPt     float64 // accumulated left indent of enclosing blocks
	textIndentPt float64
	lineMul      float64
	lineAtLeast  float64 // pt
	blockBg      string  // background of block content
	listStyle    string

	display        string
	marginTopPt    float64
	marginBottomPt float64
	ownLeftPt      float64
	widthPt        float64
	widthPct       float64
	heightPt       float64
	border         bool
	vAlign         string // top | center | bottom (table cells)
	breakBefore    bool
	breakAfter     bool
}

func (s computed) inherit() computed {
	s.display = ""
	s.marginTopPt, s.marginBottomPt, s.ownLeftPt = 0, 0, 0
	s.widthPt, s.widthPct, s.heightPt = 0, 0, 0
	s.border = false
	s.vAlign = ""
	s.breakBefore, s.breakAfter = false, false
	return s
}

var fontSizeKeywords = map[string]float64{
	"xx-small": 7, "x-small": 7.5, "small": 10, "medium": 12,
	"large": 13.5, "x-large": 18, "xx-large": 24, "xxx-large": 36,
}

func fontSizePt(v string, parent float64) (float64, bool) {
	switch v {
	case "smaller":
		return parent / 1.2, true
	case "larger":
		return parent * 1.2, true
	}
	if pt, ok := fontSizeKeywords[v]; ok {
		return pt, true
	}
	return parseLengthPt(v, parent)
}

// apply applies one declaration. block reports whether the element lays out
// as a block (affects backgrounds, vertical-align and margins).
func (s *computed) apply(d declaration, block bool, parentSize float64) {
	v := strings.ToLower(strings.TrimSpace(d.value))
	length := func(x string) (float64, bool) { return parseLengthPt(x, s.sizePt) }

	switch d.prop {
	case "font-size":
		if pt, ok := fontSizePt(v, parentSize); ok && pt > 0 {
			s.sizePt = pt
		}
	case "font-family":
		if f := firstFamily(d.value); f != "" {
			s.font = f
		}
	case "font-weight":
		switch v {
		case "bold", "bolder":
			s.bold = true
		case "normal", "lighter":
			s.bold = false
		default:
			if n, err := strconv.Atoi(v); err == nil {
				s.bold = n >= 600
			}
		}
	case "font-style":
		s.italic = v == "italic" || v == "oblique"
	case "text-decoration", "text-decoration-line":
		if strings.Contains(v, "none") {
			s.underline, s.strike = false, false
		}
		if strings.Contains(v, "underline") {
			s.underline = true
		}
		if strings.Contains(v, "line-through") {
			s.strike = true
		}
	case "color":
		if c, ok := parseColor(v); ok {
			s.color = c
		}
	case "background", "background-color":
		c, ok := colorInShorthand(v)
		if !ok && !strings.Contains(v, "none") && !strings.Contains(v, "transparent") {
			return
		}
		if block {
			s.blockBg = c
		} else {
			s.highlight = c
		}
	case "text-align":
		switch v {
		case "left", "start":
			s.align = "left"
		case "right", "end":
			s.align = "right"
		case "center":
			s.align = "center"
		case "justify":
			s.align = "both"
		}
	case "vertical-align", "valign":
		if block {
			switch v {
			case "top", "bottom":
				s.vAlign = v
			case "middle", "center":
				s.vAlign = "center"
			}
		} else {
			switch v {
			case "super", "sup":
				s.vertAlign = "superscript"
			case "sub":
				s.vertAlign = "subscript"
			case "baseline":
				s.vertAlign = ""
			}
		}
	case "white-space":
		s.pre = strings.HasPrefix(v, "pre")
	case "margin":
		f := strings.Fields(v)
		var top, right, bottom, left string
		switch len(f) {
		case 1:
			top, right, bottom, left = f[0], f[0], f[0], f[0]
		case 2:
			top, right, bottom, left = f[0], f[1], f[0], f[1]
		case 3:
			top, right, bottom, left = f[0], f[1], f[2], f[1]
		case 4:
			top, right, bottom, left = f[0], f[1], f[2], f[3]
		default:
			return
		}
		_ = right
		if pt, ok := length(top); ok {
			s.marginTopPt = pt
		}
		if pt, ok := length(bottom); ok {
			s.marginBottomPt = pt
		}
		if pt, ok := length(left); ok && block {
			s.ownLeftPt = pt
		}
	case "margin-top":
		if pt, ok := length(v); ok {
			s.marginTopPt = pt
		}
	case "margin-bottom":
		if pt, ok := length(v); ok {
			s.marginBottomPt = pt
		}
	case "margin-left":
		if pt, ok := length(v); ok && block {
			s.ownLeftPt = pt
		}
	case "text-indent":
		if pt, ok := length(v); ok {
			s.textIndentPt = pt
		}
	case "line-height":
		if v == "normal" {
			s.lineMul, s.lineAtLeast = 0, 0
		} else if n, err := strconv.ParseFloat(v, 64); err == nil {
			s.lineMul, s.lineAtLeast = n, 0
		} else if strings.HasSuffix(v, "%") {
			if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
				s.lineMul, s.lineAtLeast = n/100, 0
			}
		} else if pt, ok := length(v); ok {
			s.lineMul, s.lineAtLeast = 0, pt
		}
	case "display":
		s.display = v
	case "width":
		if strings.HasSuffix(v, "%") {
			if n, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
				s.widthPct = n
			}
		} else if pt, ok := length(v); ok {
			s.widthPt = pt
		}
	case "height":
		if pt, ok := length(v); ok {
			s.heightPt = pt
		}
	case "border", "border-top", "border-bottom", "border-left", "border-right", "border-style", "border-width":
		s.border = !(v == "0" || strings.Contains(v, "none") || strings.Contains(v, "hidden") || strings.HasPrefix(v, "0 "))
	case "page-break-before", "break-before":
		s.breakBefore = v == "always" || v == "page" || v == "left" || v == "right"
	case "page-break-after", "break-after":
		s.breakAfter = v == "always" || v == "page" || v == "left" || v == "right"
	case "list-style-type", "list-style":
		for _, tok := range strings.Fields(v) {
			if _, ok := listFormats[tok]; ok {
				s.listStyle = tok
				break
			}
		}
	}
}

// listFormats maps CSS list-style-type values to Word numFmt values.
var listFormats = map[string]string{
	"disc": "bullet", "circle": "bullet", "square": "bullet",
	"decimal":     "decimal",
	"lower-alpha": "lowerLetter", "lower-latin": "lowerLetter",
	"upper-alpha": "upperLetter", "upper-latin": "upperLetter",
	"lower-roman": "lowerRoman", "upper-roman": "upperRoman",
	"thai": "thaiNumbers",
	"none": "none",
}

func firstFamily(v string) string {
	f := strings.TrimSpace(strings.Split(v, ",")[0])
	return strings.Trim(f, `"' `)
}
