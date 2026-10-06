package htmldocx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	relStyles    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	relNumbering = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering"
	relSettings  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings"
	relHyperlink = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
	relImage     = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

type relationship struct {
	id, typ, target string
	external        bool
}

type mediaFile struct {
	name string
	data []byte
}

// docPackage collects everything that ends up in the .docx besides the body.
type docPackage struct {
	rels      []relationship
	media     []mediaFile
	abstracts []string
	nums      []string
	bulletID  int
}

func newPackage() *docPackage {
	return &docPackage{rels: []relationship{
		{id: "rId1", typ: relStyles, target: "styles.xml"},
		{id: "rId2", typ: relNumbering, target: "numbering.xml"},
		{id: "rId3", typ: relSettings, target: "settings.xml"},
	}}
}

func (p *docPackage) addRel(typ, target string, external bool) string {
	id := fmt.Sprintf("rId%d", len(p.rels)+1)
	p.rels = append(p.rels, relationship{id: id, typ: typ, target: target, external: external})
	return id
}

func (p *docPackage) addLink(href string) string { return p.addRel(relHyperlink, href, true) }

func (p *docPackage) addImage(data []byte, ext string) string {
	name := fmt.Sprintf("image%d.%s", len(p.media)+1, ext)
	p.media = append(p.media, mediaFile{name: name, data: data})
	return p.addRel(relImage, "media/"+name, false)
}

// bulletNum returns the shared numId used by every bulleted list.
func (p *docPackage) bulletNum() int {
	if p.bulletID == 0 {
		p.bulletID = p.addNum("bullet", 1)
	}
	return p.bulletID
}

// addNum creates a fresh numbering instance, so each <ol> restarts at start.
func (p *docPackage) addNum(format string, start int) int {
	abs := len(p.abstracts)
	var b strings.Builder
	fmt.Fprintf(&b, `<w:abstractNum w:abstractNumId="%d"><w:multiLevelType w:val="hybridMultilevel"/>`, abs)
	bullets := []string{"•", "◦", "▪"}
	for l := 0; l < 9; l++ {
		fmt.Fprintf(&b, `<w:lvl w:ilvl="%d"><w:start w:val="%d"/>`, l, start)
		if format == "bullet" {
			fmt.Fprintf(&b, `<w:numFmt w:val="bullet"/><w:lvlText w:val="%s"/>`, bullets[l%3])
		} else {
			fmt.Fprintf(&b, `<w:numFmt w:val="%s"/><w:lvlText w:val="%%%d."/>`, format, l+1)
		}
		fmt.Fprintf(&b, `<w:lvlJc w:val="left"/><w:pPr><w:ind w:left="%d" w:hanging="360"/></w:pPr></w:lvl>`, 720*(l+1))
	}
	b.WriteString(`</w:abstractNum>`)
	p.abstracts = append(p.abstracts, b.String())

	id := len(p.nums) + 1
	p.nums = append(p.nums, fmt.Sprintf(`<w:num w:numId="%d"><w:abstractNumId w:val="%d"/></w:num>`, id, abs))
	return id
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const wordNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

type pageSetup struct {
	width, height, margin int // twips
	landscape             bool
}

func (p *docPackage) build(body string, ps pageSetup, opt Options) ([]byte, error) {
	// Word refuses a table directly before sectPr; it must be followed by a paragraph.
	if body == "" || strings.HasSuffix(body, "</w:tbl>") {
		body += "<w:p/>"
	}
	orient := ""
	if ps.landscape {
		orient = ` w:orient="landscape"`
	}
	document := xmlHeader + `<w:document ` + wordNS +
		` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
		` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
		` xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body>` +
		body +
		fmt.Sprintf(`<w:sectPr><w:pgSz w:w="%d" w:h="%d"%s/>`+
			`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="708" w:footer="708" w:gutter="0"/>`+
			`</w:sectPr>`, ps.width, ps.height, orient, ps.margin, ps.margin, ps.margin, ps.margin) +
		`</w:body></w:document>`

	var rels strings.Builder
	rels.WriteString(xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range p.rels {
		mode := ""
		if r.external {
			mode = ` TargetMode="External"`
		}
		fmt.Fprintf(&rels, `<Relationship Id="%s" Type="%s" Target="%s"%s/>`, r.id, r.typ, escapeXML(r.target), mode)
	}
	rels.WriteString(`</Relationships>`)

	numbering := xmlHeader + `<w:numbering ` + wordNS + `>` +
		strings.Join(p.abstracts, "") + strings.Join(p.nums, "") + `</w:numbering>`

	core := xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + escapeXML(opt.Title) + `</dc:title><dc:creator>` + escapeXML(opt.Author) + `</dc:creator>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + time.Now().UTC().Format(time.RFC3339) + `</dcterms:created>` +
		`</cp:coreProperties>`

	parts := []struct {
		name string
		data []byte
	}{
		{"[Content_Types].xml", []byte(contentTypes)},
		{"_rels/.rels", []byte(rootRels)},
		{"docProps/core.xml", []byte(core)},
		{"word/document.xml", []byte(document)},
		{"word/_rels/document.xml.rels", []byte(rels.String())},
		{"word/styles.xml", []byte(stylesXML(opt))},
		{"word/numbering.xml", []byte(numbering)},
		{"word/settings.xml", []byte(settingsXML)},
	}
	for _, m := range p.media {
		parts = append(parts, struct {
			name string
			data []byte
		}{"word/media/" + m.name, m.data})
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range parts {
		w, err := zw.Create(part.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(part.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const contentTypes = xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Default Extension="png" ContentType="image/png"/>` +
	`<Default Extension="jpeg" ContentType="image/jpeg"/>` +
	`<Default Extension="gif" ContentType="image/gif"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
	`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
	`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
	`</Types>`

const rootRels = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`</Relationships>`

// compatibilityMode 15 stops Word from opening the file in compatibility mode.
const settingsXML = xmlHeader + `<w:settings ` + wordNS + `>` +
	`<w:defaultTabStop w:val="720"/>` +
	`<w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat>` +
	`</w:settings>`

func stylesXML(opt Options) string {
	font := escapeXML(opt.Font)
	hp := int(opt.FontSize*2 + 0.5)
	var b strings.Builder
	b.WriteString(xmlHeader + `<w:styles ` + wordNS + `>`)
	// w:cs / szCs are what Word uses for Thai (complex script) text.
	fmt.Fprintf(&b, `<w:docDefaults><w:rPrDefault><w:rPr>`+
		`<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/>`+
		`<w:sz w:val="%[2]d"/><w:szCs w:val="%[2]d"/>`+
		`<w:lang w:val="en-US" w:eastAsia="en-US" w:bidi="th-TH"/>`+
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>`,
		font, hp)
	b.WriteString(`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>`)
	for i := 1; i <= 6; i++ {
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="Heading%[1]d"><w:name w:val="heading %[1]d"/>`+
			`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="9"/><w:qFormat/>`+
			`<w:pPr><w:keepNext/><w:keepLines/><w:outlineLvl w:val="%[2]d"/></w:pPr></w:style>`, i, i-1)
	}
	b.WriteString(`<w:style w:type="character" w:default="1" w:styleId="DefaultParagraphFont"><w:name w:val="Default Paragraph Font"/><w:uiPriority w:val="1"/><w:semiHidden/></w:style>`)
	b.WriteString(`<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/><w:basedOn w:val="DefaultParagraphFont"/><w:rPr><w:color w:val="0563C1"/><w:u w:val="single"/></w:rPr></w:style>`)
	b.WriteString(`<w:style w:type="table" w:default="1" w:styleId="TableNormal"><w:name w:val="Normal Table"/><w:semiHidden/>` +
		`<w:tblPr><w:tblInd w:w="0" w:type="dxa"/><w:tblCellMar><w:top w:w="0" w:type="dxa"/><w:left w:w="108" w:type="dxa"/><w:bottom w:w="0" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>`)
	b.WriteString(`</w:styles>`)
	return b.String()
}

// escapeXML escapes text for XML and drops characters XML 1.0 forbids.
func escapeXML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			if r == '\t' || r == '\n' || r == '\r' ||
				(r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || r >= 0x10000 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
