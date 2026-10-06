// Package htmldocx converts HTML + CSS into a Word (.docx) document using only
// the standard library and golang.org/x/net/html.
package htmldocx

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Options controls page setup and defaults. Zero values get sensible defaults.
type Options struct {
	Font      string  `json:"font"`      // default font, e.g. "TH Sarabun New" (default "Tahoma")
	FontSize  float64 `json:"fontSize"`  // pt (default 11)
	PageSize  string  `json:"pageSize"`  // "A4" (default) | "Letter"
	Landscape bool    `json:"landscape"` // landscape orientation
	MarginMM  float64 `json:"marginMM"`  // page margin in mm (default 25.4)
	Title     string  `json:"title"`     // document title (default: <title>)
	Author    string  `json:"author"`

	// AllowRemoteImages lets <img src="http(s)://..."> be downloaded. Off by
	// default because it lets document authors make the server fetch URLs.
	AllowRemoteImages bool `json:"-"`
}

func (o *Options) normalize() {
	if o.Font == "" {
		o.Font = "Tahoma"
	}
	if o.FontSize <= 0 {
		o.FontSize = 11
	}
	if o.MarginMM <= 0 {
		o.MarginMM = 25.4
	}
}

// uaCSS plays the role of the browser's default stylesheet.
const uaCSS = `
h1{font-size:2em;font-weight:bold;margin-top:12pt;margin-bottom:6pt}
h2{font-size:1.5em;font-weight:bold;margin-top:10pt;margin-bottom:6pt}
h3{font-size:1.17em;font-weight:bold;margin-top:8pt;margin-bottom:4pt}
h4{font-weight:bold;margin-top:8pt;margin-bottom:4pt}
h5{font-size:0.83em;font-weight:bold;margin-top:6pt;margin-bottom:4pt}
h6{font-size:0.67em;font-weight:bold;margin-top:6pt;margin-bottom:4pt}
p{margin-bottom:8pt}
pre{font-family:Courier New;white-space:pre;margin-bottom:8pt}
blockquote{margin-left:36pt;margin-bottom:8pt}
dd{margin-left:36pt}
dt,th{font-weight:bold}
th,caption,center{text-align:center}
b,strong{font-weight:bold}
i,em,cite,var,dfn,address{font-style:italic}
u,ins{text-decoration:underline}
s,strike,del{text-decoration:line-through}
code,kbd,samp,tt{font-family:Courier New}
a{color:#0563C1;text-decoration:underline}
mark{background-color:yellow}
small{font-size:0.83em}
big{font-size:1.2em}
sup{vertical-align:super;font-size:0.83em}
sub{vertical-align:sub;font-size:0.83em}
ul{list-style-type:disc}
ol{list-style-type:decimal}
`

var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "body": true,
	"caption": true, "center": true, "dd": true, "details": true, "div": true, "dl": true,
	"dt": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true,
	"form": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "html": true, "li": true, "main": true, "nav": true,
	"ol": true, "p": true, "pre": true, "section": true, "summary": true, "table": true,
	"tbody": true, "td": true, "tfoot": true, "th": true, "thead": true, "tr": true, "ul": true,
}

var skipTags = map[string]bool{
	"head": true, "script": true, "style": true, "noscript": true, "template": true,
	"title": true, "meta": true, "link": true, "iframe": true, "object": true, "embed": true,
	"svg": true, "canvas": true, "video": true, "audio": true, "input": true,
	"select": true, "textarea": true, "button": true,
}

type blockCtx struct {
	st      computed
	styleID string
}

type numRef struct{ id, lvl int }

type paragraph struct {
	blk         blockCtx
	num         *numRef
	breakBefore bool
	runs        strings.Builder
	hasContent  bool
	lastSpace   bool
}

type converter struct {
	opt       Options
	pkg       *docPackage
	ua        []rule
	author    []rule
	out       *strings.Builder
	para      *paragraph
	block     blockCtx
	pendNum   *numRef
	pendBreak bool
	listDepth int
	inLink    bool
	drawingID int
	textWidth int // twips
}

// Convert renders an HTML document (with <style> blocks and style attributes) to .docx bytes.
func Convert(src string, opt Options) ([]byte, error) {
	return ConvertWithCSS(src, "", opt)
}

// ConvertWithCSS is Convert with an extra stylesheet supplied separately from
// the HTML. It is applied after any <style> blocks in the document, so on equal
// specificity its rules win; inline style attributes still win over both.
func ConvertWithCSS(src, extraCSS string, opt Options) ([]byte, error) {
	opt.normalize()
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	ps := pageSetup{width: 11906, height: 16838} // A4
	if strings.EqualFold(opt.PageSize, "letter") {
		ps = pageSetup{width: 12240, height: 15840}
	}
	if opt.Landscape {
		ps.width, ps.height, ps.landscape = ps.height, ps.width, true
	}
	ps.margin = int(opt.MarginMM * 56.6929)

	c := &converter{
		opt:       opt,
		pkg:       newPackage(),
		out:       &strings.Builder{},
		textWidth: ps.width - 2*ps.margin,
	}
	c.ua = parseStylesheet(uaCSS, nil)
	sortRules(c.ua)
	var css strings.Builder
	walkAll(root, func(n *html.Node) {
		switch n.Data {
		case "style":
			if n.FirstChild != nil {
				css.WriteString(n.FirstChild.Data)
				css.WriteString("\n")
			}
		case "title":
			if c.opt.Title == "" && n.FirstChild != nil {
				c.opt.Title = strings.TrimSpace(n.FirstChild.Data)
			}
		}
	})
	css.WriteString(extraCSS)
	c.author = parseStylesheet(css.String(), nil)
	sortRules(c.author)

	st := computed{sizePt: opt.FontSize}
	body := root
	for _, tag := range []string{"html", "body"} {
		if n := findElement(root, tag); n != nil {
			st = c.compute(n, st)
			body = n
		}
	}
	c.block = blockCtx{st: st}
	c.walkChildren(body, st)
	c.flush()
	return c.pkg.build(c.out.String(), ps, c.opt)
}

// compute resolves the style of n: UA rules < presentational attributes <
// author rules (by specificity) < style attribute < !important.
func (c *converter) compute(n *html.Node, parent computed) computed {
	st := parent.inherit()
	var decls []declaration
	for _, r := range c.ua {
		if r.sel.matches(n) {
			decls = append(decls, r.decls...)
		}
	}
	decls = append(decls, presentationalHints(n)...)
	for _, r := range c.author {
		if r.sel.matches(n) {
			decls = append(decls, r.decls...)
		}
	}
	if s := attr(n, "style"); s != "" {
		decls = append(decls, parseDeclarations(s)...)
	}

	ordered := make([]declaration, 0, len(decls))
	for _, important := range []bool{false, true} {
		for _, d := range decls {
			if d.important == important {
				ordered = append(ordered, d)
			}
		}
	}

	block := blockTags[n.Data]
	for _, d := range ordered { // display first: it decides block vs inline
		if d.prop == "display" {
			st.apply(d, block, parent.sizePt)
		}
	}
	if st.display == "block" {
		block = true
	} else if st.display == "inline" || st.display == "inline-block" {
		block = false
	}
	for _, d := range ordered { // font-size next: em units depend on it
		if d.prop == "font-size" {
			st.apply(d, block, parent.sizePt)
		}
	}
	for _, d := range ordered {
		if d.prop != "font-size" && d.prop != "display" {
			st.apply(d, block, parent.sizePt)
		}
	}
	if block {
		st.indentPt = parent.indentPt + st.ownLeftPt
	}
	return st
}

func presentationalHints(n *html.Node) []declaration {
	var d []declaration
	add := func(p, v string) { d = append(d, declaration{prop: p, value: v}) }
	if v := attr(n, "align"); v != "" && n.Data != "img" && n.Data != "table" {
		add("text-align", v)
	}
	if v := attr(n, "bgcolor"); v != "" {
		add("background-color", v)
	}
	if v := attr(n, "valign"); v != "" {
		add("vertical-align", v)
	}
	for _, k := range []string{"width", "height"} {
		if v := attr(n, k); v != "" {
			if _, err := strconv.ParseFloat(v, 64); err == nil {
				v += "px"
			}
			add(k, v)
		}
	}
	switch n.Data {
	case "font":
		if v := attr(n, "color"); v != "" {
			add("color", v)
		}
		if v := attr(n, "face"); v != "" {
			add("font-family", v)
		}
		if v, err := strconv.Atoi(attr(n, "size")); err == nil && v >= 1 && v <= 7 {
			add("font-size", []string{"7.5pt", "10pt", "12pt", "13.5pt", "18pt", "24pt", "36pt"}[v-1])
		}
	case "table":
		if b := attr(n, "border"); b != "" && b != "0" {
			add("border", "1px solid black")
		}
	}
	return d
}

func (c *converter) walkChildren(n *html.Node, st computed) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.walk(ch, st)
	}
}

func (c *converter) walk(n *html.Node, parent computed) {
	if n.Type == html.TextNode {
		c.text(n.Data, parent)
		return
	}
	if n.Type != html.ElementNode || skipTags[n.Data] {
		return
	}
	st := c.compute(n, parent)
	if st.display == "none" {
		return
	}

	switch n.Data {
	case "br":
		c.ensurePara()
		c.para.runs.WriteString(`<w:r><w:br/></w:r>`)
		c.para.hasContent, c.para.lastSpace = true, true
		return
	case "img":
		c.image(n, st)
		return
	case "hr":
		c.flush()
		c.out.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="auto"/></w:pBdr>` +
			`<w:spacing w:before="120" w:after="120"/></w:pPr></w:p>`)
		return
	case "table":
		c.table(n, st)
		return
	case "ul", "ol":
		c.list(n, st)
		return
	case "a":
		if href := strings.TrimSpace(attr(n, "href")); isExternalLink(href) && !hasBlockDescendant(n) {
			c.ensurePara()
			c.para.runs.WriteString(`<w:hyperlink r:id="` + c.pkg.addLink(href) + `" w:history="1">`)
			c.inLink = true
			c.walkChildren(n, st)
			c.inLink = false
			c.para.runs.WriteString(`</w:hyperlink>`)
			return
		}
	}

	isBlock := blockTags[n.Data]
	if st.display == "block" {
		isBlock = true
	} else if st.display == "inline" || st.display == "inline-block" {
		isBlock = false
	}
	if !isBlock {
		c.walkChildren(n, st)
		return
	}

	c.flush()
	if st.breakBefore {
		c.pendBreak = true
	}
	saved := c.block
	c.block = blockCtx{st: st}
	if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
		c.block.styleID = "Heading" + n.Data[1:]
	}
	c.walkChildren(n, st)
	c.flush()
	c.block = saved
	if st.breakAfter {
		c.pendBreak = true
	}
}

func (c *converter) text(data string, st computed) {
	if st.pre {
		data = strings.ReplaceAll(strings.ReplaceAll(data, "\r\n", "\n"), "\t", "    ")
		for i, line := range strings.Split(data, "\n") {
			c.ensurePara()
			if i > 0 {
				c.para.runs.WriteString(`<w:r><w:br/></w:r>`)
			}
			if line != "" {
				c.writeRun(line, st)
			}
			c.para.hasContent = true
		}
		return
	}

	t := collapseWhitespace(data)
	if t == "" || (c.para == nil && strings.TrimSpace(t) == "") {
		return
	}
	c.ensurePara()
	if c.para.lastSpace || !c.para.hasContent {
		t = strings.TrimLeft(t, " ")
	}
	if t == "" {
		return
	}
	c.writeRun(t, st)
	c.para.hasContent = true
	c.para.lastSpace = strings.HasSuffix(t, " ")
}

func (c *converter) writeRun(t string, st computed) {
	c.para.runs.WriteString(`<w:r>` + c.rPr(st) + `<w:t xml:space="preserve">` + escapeXML(t) + `</w:t></w:r>`)
}

func (c *converter) ensurePara() {
	if c.para == nil {
		c.para = &paragraph{blk: c.block, num: c.pendNum, breakBefore: c.pendBreak}
		c.pendNum, c.pendBreak = nil, false
	}
}

func (c *converter) flush() {
	p := c.para
	if p == nil {
		return
	}
	c.para = nil
	c.out.WriteString(`<w:p>` + c.pPr(p) + p.runs.String() + `</w:p>`)
}

// pPr builds paragraph properties. Element order follows the OOXML schema (CT_PPr).
func (c *converter) pPr(p *paragraph) string {
	st := p.blk.st
	var b strings.Builder
	if p.blk.styleID != "" {
		fmt.Fprintf(&b, `<w:pStyle w:val="%s"/>`, p.blk.styleID)
	}
	if p.breakBefore {
		b.WriteString(`<w:pageBreakBefore/>`)
	}
	if p.num != nil {
		fmt.Fprintf(&b, `<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, p.num.lvl, p.num.id)
	}
	if st.blockBg != "" {
		fmt.Fprintf(&b, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, st.blockBg)
	}
	line := ""
	if st.lineAtLeast > 0 {
		line = fmt.Sprintf(` w:line="%d" w:lineRule="atLeast"`, twips(st.lineAtLeast))
	} else if st.lineMul > 0 {
		line = fmt.Sprintf(` w:line="%d" w:lineRule="auto"`, int(math.Round(240*st.lineMul)))
	}
	fmt.Fprintf(&b, `<w:spacing w:before="%d" w:after="%d"%s/>`, twips(st.marginTopPt), twips(st.marginBottomPt), line)
	if p.num == nil && (st.indentPt != 0 || st.textIndentPt != 0) {
		first := ""
		if st.textIndentPt > 0 {
			first = fmt.Sprintf(` w:firstLine="%d"`, twips(st.textIndentPt))
		} else if st.textIndentPt < 0 {
			first = fmt.Sprintf(` w:hanging="%d"`, twips(-st.textIndentPt))
		}
		fmt.Fprintf(&b, `<w:ind w:left="%d"%s/>`, twips(st.indentPt), first)
	}
	if st.align != "" {
		fmt.Fprintf(&b, `<w:jc w:val="%s"/>`, st.align)
	}
	return `<w:pPr>` + b.String() + `</w:pPr>`
}

// rPr builds run properties. Element order follows the OOXML schema (CT_RPr).
// The *Cs variants matter: Word formats Thai text with the complex-script props.
func (c *converter) rPr(st computed) string {
	var b strings.Builder
	if c.inLink {
		b.WriteString(`<w:rStyle w:val="Hyperlink"/>`)
	}
	if st.font != "" {
		f := escapeXML(st.font)
		fmt.Fprintf(&b, `<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/>`, f)
	}
	if st.bold {
		b.WriteString(`<w:b/><w:bCs/>`)
	}
	if st.italic {
		b.WriteString(`<w:i/><w:iCs/>`)
	}
	if st.strike {
		b.WriteString(`<w:strike/>`)
	}
	if st.color != "" {
		fmt.Fprintf(&b, `<w:color w:val="%s"/>`, st.color)
	}
	if hp := int(math.Round(st.sizePt * 2)); hp != int(math.Round(c.opt.FontSize*2)) && hp > 0 {
		fmt.Fprintf(&b, `<w:sz w:val="%[1]d"/><w:szCs w:val="%[1]d"/>`, hp)
	}
	if st.underline {
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if st.highlight != "" {
		fmt.Fprintf(&b, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, st.highlight)
	}
	if st.vertAlign != "" {
		fmt.Fprintf(&b, `<w:vertAlign w:val="%s"/>`, st.vertAlign)
	}
	if b.Len() == 0 {
		return ""
	}
	return `<w:rPr>` + b.String() + `</w:rPr>`
}

func (c *converter) list(n *html.Node, st computed) {
	c.flush()
	format := listFormats[st.listStyle]
	switch attr(n, "type") {
	case "1":
		format = "decimal"
	case "a":
		format = "lowerLetter"
	case "A":
		format = "upperLetter"
	case "i":
		format = "lowerRoman"
	case "I":
		format = "upperRoman"
	}
	numID := 0
	switch format {
	case "none":
	case "bullet", "":
		numID = c.pkg.bulletNum()
	default:
		start := 1
		if v, err := strconv.Atoi(attr(n, "start")); err == nil {
			start = v
		}
		numID = c.pkg.addNum(format, start)
	}
	lvl := min(c.listDepth, 8)
	c.listDepth++
	defer func() { c.listDepth-- }()

	for li := n.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.Data != "li" {
			c.walk(li, st)
			continue
		}
		lst := c.compute(li, st)
		if lst.display == "none" {
			continue
		}
		c.flush()
		saved := c.block
		c.block = blockCtx{st: lst}
		if numID > 0 {
			c.pendNum = &numRef{id: numID, lvl: lvl}
		}
		c.walkChildren(li, lst)
		c.flush()
		c.pendNum = nil
		c.block = saved
	}
}

type tableCell struct {
	node             *html.Node
	row, col, cs, rs int
}

func (c *converter) table(n *html.Node, st computed) {
	c.flush()
	if c.pendBreak { // a table cannot carry pageBreakBefore itself
		c.out.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
		c.pendBreak = false
	}
	var rows []*html.Node
	header := map[*html.Node]bool{}
	var caption *html.Node
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		switch ch.Data {
		case "caption":
			caption = ch
		case "tr":
			rows = append(rows, ch)
		case "thead", "tbody", "tfoot":
			for tr := ch.FirstChild; tr != nil; tr = tr.NextSibling {
				if tr.Type == html.ElementNode && tr.Data == "tr" {
					rows = append(rows, tr)
					header[tr] = ch.Data == "thead"
				}
			}
		}
	}
	if caption != nil {
		cst := c.compute(caption, st)
		saved := c.block
		c.block = blockCtx{st: cst}
		c.walkChildren(caption, cst)
		c.flush()
		c.block = saved
	}

	// Lay cells out on a grid, honouring colspan/rowspan.
	owner := map[[2]int]*tableCell{}
	ncols := 0
	var firstCell *html.Node
	for r, tr := range rows {
		col := 0
		for td := tr.FirstChild; td != nil; td = td.NextSibling {
			if td.Type != html.ElementNode || (td.Data != "td" && td.Data != "th") {
				continue
			}
			if firstCell == nil {
				firstCell = td
			}
			for owner[[2]int{r, col}] != nil {
				col++
			}
			cs := clamp(atoiAttr(td, "colspan", 1), 1, 63)
			rs := atoiAttr(td, "rowspan", 1)
			if rs <= 0 || rs > len(rows)-r {
				rs = len(rows) - r
			}
			cell := &tableCell{node: td, row: r, col: col, cs: cs, rs: rs}
			for i := 0; i < rs; i++ {
				for j := 0; j < cs; j++ {
					owner[[2]int{r + i, col + j}] = cell
				}
			}
			col += cs
			ncols = max(ncols, col)
		}
	}
	if ncols == 0 {
		return
	}

	bordered := st.border
	if !bordered && firstCell != nil {
		bordered = c.compute(firstCell, st).border
	}
	total := c.textWidth - twips(st.indentPt)
	if st.widthPct > 0 {
		total = int(float64(total) * st.widthPct / 100)
	} else if st.widthPt > 0 {
		total = twips(st.widthPt)
	}
	colW := total / ncols

	o := c.out
	o.WriteString(`<w:tbl><w:tblPr>`)
	fmt.Fprintf(o, `<w:tblW w:w="%d" w:type="dxa"/>`, total)
	if strings.EqualFold(attr(n, "align"), "center") {
		o.WriteString(`<w:jc w:val="center"/>`)
	}
	if st.indentPt > 0 {
		fmt.Fprintf(o, `<w:tblInd w:w="%d" w:type="dxa"/>`, twips(st.indentPt))
	}
	if bordered {
		o.WriteString(`<w:tblBorders>`)
		for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
			fmt.Fprintf(o, `<w:%s w:val="single" w:sz="4" w:space="0" w:color="auto"/>`, side)
		}
		o.WriteString(`</w:tblBorders>`)
	}
	o.WriteString(`<w:tblCellMar><w:top w:w="40" w:type="dxa"/><w:left w:w="100" w:type="dxa"/>` +
		`<w:bottom w:w="40" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`)
	for i := 0; i < ncols; i++ {
		fmt.Fprintf(o, `<w:gridCol w:w="%d"/>`, colW)
	}
	o.WriteString(`</w:tblGrid>`)

	for r, tr := range rows {
		trSt := c.compute(tr, st)
		o.WriteString(`<w:tr>`)
		if header[tr] {
			o.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for col := 0; col < ncols; {
			cell := owner[[2]int{r, col}]
			switch {
			case cell == nil: // short row: pad with an empty cell
				fmt.Fprintf(o, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr><w:p/></w:tc>`, colW)
				col++
			case cell.row != r: // covered by a rowspan from above
				fmt.Fprintf(o, `<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s<w:vMerge/></w:tcPr><w:p/></w:tc>`,
					colW*cell.cs, gridSpan(cell.cs))
				col += cell.cs
			default:
				c.tableCell(cell, trSt, colW)
				col += cell.cs
			}
		}
		o.WriteString(`</w:tr>`)
	}
	o.WriteString(`</w:tbl>`)
}

func (c *converter) tableCell(cell *tableCell, trSt computed, colW int) {
	st := c.compute(cell.node, trSt)
	bg := st.blockBg
	st.blockBg = "" // shading goes on the cell, not on each paragraph
	st.indentPt = 0

	var tcPr strings.Builder
	fmt.Fprintf(&tcPr, `<w:tcW w:w="%d" w:type="dxa"/>%s`, colW*cell.cs, gridSpan(cell.cs))
	if cell.rs > 1 {
		tcPr.WriteString(`<w:vMerge w:val="restart"/>`)
	}
	if bg != "" {
		fmt.Fprintf(&tcPr, `<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, bg)
	}
	if st.vAlign != "" {
		fmt.Fprintf(&tcPr, `<w:vAlign w:val="%s"/>`, st.vAlign)
	}

	// Render the cell body into its own buffer.
	savedOut, savedPara, savedBlock := c.out, c.para, c.block
	savedNum, savedBreak, savedDepth := c.pendNum, c.pendBreak, c.listDepth
	var body strings.Builder
	c.out, c.para, c.block = &body, nil, blockCtx{st: st}
	c.pendNum, c.pendBreak, c.listDepth = nil, false, 0
	c.walkChildren(cell.node, st)
	c.flush()
	c.out, c.para, c.block = savedOut, savedPara, savedBlock
	c.pendNum, c.pendBreak, c.listDepth = savedNum, savedBreak, savedDepth

	content := body.String()
	if !strings.HasSuffix(content, "</w:p>") { // a cell must end with a paragraph
		content += "<w:p/>"
	}
	c.out.WriteString(`<w:tc><w:tcPr>` + tcPr.String() + `</w:tcPr>` + content + `</w:tc>`)
}

func gridSpan(cs int) string {
	if cs <= 1 {
		return ""
	}
	return fmt.Sprintf(`<w:gridSpan w:val="%d"/>`, cs)
}

func (c *converter) image(n *html.Node, st computed) {
	data, err := c.loadImage(strings.TrimSpace(attr(n, "src")))
	var cfg image.Config
	var format string
	if err == nil {
		cfg, format, err = image.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil || cfg.Width == 0 || cfg.Height == 0 {
		if alt := attr(n, "alt"); alt != "" { // fall back to the alt text
			c.text("["+alt+"]", st)
		}
		return
	}

	ratio := float64(cfg.Height) / float64(cfg.Width)
	w, h := st.widthPt, st.heightPt
	maxW := float64(c.textWidth) / 20
	switch {
	case st.widthPct > 0:
		w, h = maxW*st.widthPct/100, maxW*st.widthPct/100*ratio
	case w > 0 && h == 0:
		h = w * ratio
	case h > 0 && w == 0:
		w = h / ratio
	case w == 0 && h == 0:
		w, h = float64(cfg.Width)*0.75, float64(cfg.Height)*0.75
	}
	if w > maxW { // keep the image inside the page margins
		h, w = h*maxW/w, maxW
	}
	cx, cy := int64(w*12700), int64(h*12700) // 1pt = 12700 EMU

	ext := format
	if ext == "jpg" {
		ext = "jpeg"
	}
	rid := c.pkg.addImage(data, ext)
	c.drawingID++
	id := c.drawingID
	c.ensurePara()
	fmt.Fprintf(&c.para.runs, `<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%[1]d" cy="%[2]d"/><wp:docPr id="%[3]d" name="Picture %[3]d" descr="%[4]s"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="%[3]d" name="Picture %[3]d"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="%[5]s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%[1]d" cy="%[2]d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic>`+
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`,
		cx, cy, id, escapeXML(attr(n, "alt")), rid)
	c.para.hasContent, c.para.lastSpace = true, false
}

const maxImageBytes = 10 << 20

func (c *converter) loadImage(src string) ([]byte, error) {
	if strings.HasPrefix(src, "data:") {
		comma := strings.IndexByte(src, ',')
		if comma < 0 {
			return nil, errors.New("malformed data URI")
		}
		meta, payload := src[5:comma], src[comma+1:]
		if strings.HasSuffix(meta, ";base64") {
			return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(payload), ""))
		}
		s, err := url.PathUnescape(payload)
		return []byte(s), err
	}
	if !c.opt.AllowRemoteImages || !(strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")) {
		return nil, errors.New("image source not allowed")
	}
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch image: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
}

// --- helpers ---

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func atoiAttr(n *html.Node, key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(attr(n, key))); err == nil {
		return v
	}
	return def
}

func parentElement(n *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			return p
		}
	}
	return nil
}

func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if f := findElement(ch, tag); f != nil {
			return f
		}
	}
	return nil
}

func walkAll(n *html.Node, fn func(*html.Node)) {
	if n.Type == html.ElementNode {
		fn(n)
	}
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		walkAll(ch, fn)
	}
}

func hasBlockDescendant(n *html.Node) bool {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && (blockTags[ch.Data] || hasBlockDescendant(ch)) {
			return true
		}
	}
	return false
}

func isExternalLink(href string) bool {
	h := strings.ToLower(href)
	return strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") || strings.HasPrefix(h, "mailto:")
}

// collapseWhitespace mimics white-space: normal. &nbsp; (U+00A0) is preserved.
func collapseWhitespace(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			if !space {
				b.WriteByte(' ')
				space = true
			}
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

func twips(pt float64) int { return int(math.Round(pt * 20)) }

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }
