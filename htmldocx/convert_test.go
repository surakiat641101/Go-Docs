package htmldocx

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
)

func pngDataURI(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{255, 0, 0, 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// unzip returns every part of the package and fails if any XML part is malformed.
func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			dec := xml.NewDecoder(bytes.NewReader(b))
			for {
				if _, err := dec.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("%s is not well-formed XML: %v", f.Name, err)
				}
			}
		}
	}
	return parts
}

func TestConvertFullDocument(t *testing.T) {
	src := `<html><head><title>รายงาน</title><style>
		body { font-family: "TH Sarabun New"; font-size: 16pt }
		.red { color: #f00 } #big { font-size: 2em }
		table td.c { background-color: rgb(200, 220, 240); vertical-align: middle }
		@media print { .p { font-weight: bold } }
		a:hover { color: green }
	</style></head><body>
	<h1>หัวข้อ &amp; ทดสอบ</h1>
	<p class="red">ข้อความ <b>ตัวหนา</b> <i>เอียง</i> <u>ขีดเส้นใต้</u> H<sub>2</sub>O</p>
	<p id="big" class="p" style="text-align:justify; text-indent: 1cm">ใหญ่</p>
	<ul><li>หนึ่ง<ol><li>ย่อย</li></ol></li><li>สอง</li></ul>
	<ol start="5" type="a"><li>ห้า</li></ol>
	<table border="1"><thead><tr><th>A</th><th>B</th><th>C</th></tr></thead>
	<tr><td rowspan="2" class="c">x</td><td colspan="2">y</td></tr>
	<tr><td>z</td></tr></table>
	<p><a href="https://example.com/?a=1&b=2">ลิงก์</a> <img src="` + pngDataURI(t, 40, 20) + `" width="80" alt="โลโก้"></p>
	<pre>line 1
  line 2</pre>
	<div style="page-break-after:always"></div><p>หน้า 2</p>
	</body></html>`

	data, err := Convert(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	parts := unzip(t, data)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml",
		"word/_rels/document.xml.rels", "word/styles.xml", "word/numbering.xml",
		"word/settings.xml", "docProps/core.xml", "word/media/image1.png"} {
		if _, ok := parts[name]; !ok {
			t.Errorf("missing part %s", name)
		}
	}

	doc := parts["word/document.xml"]
	wants := map[string]string{
		"heading style":     `<w:pStyle w:val="Heading1"/>`,
		"escaped text":      `หัวข้อ &amp; ทดสอบ`,
		"author font":       `w:cs="TH Sarabun New"`,
		"class color":       `<w:color w:val="FF0000"/>`,
		"thai bold":         `<w:b/><w:bCs/>`,
		"subscript":         `<w:vertAlign w:val="subscript"/>`,
		"em size (2*16pt)":  `<w:sz w:val="64"/>`,
		"@media rule":       `<w:b/><w:bCs/><w:sz w:val="64"/>`,
		"justify":           `<w:jc w:val="both"/>`,
		"text-indent 1cm":   `w:firstLine="567"`,
		"nested list level": `<w:ilvl w:val="1"/>`,
		"table borders":     `<w:tblBorders>`,
		"repeat header":     `<w:tblHeader/>`,
		"colspan":           `<w:gridSpan w:val="2"/>`,
		"rowspan start":     `<w:vMerge w:val="restart"/>`,
		"rowspan continue":  `<w:vMerge/>`,
		"cell shading":      `w:fill="C8DCF0"`,
		"cell valign":       `<w:vAlign w:val="center"/>`,
		"hyperlink":         `<w:hyperlink r:id=`,
		"image 80px = 60pt": `cx="762000" cy="381000"`,
		"pre line break":    `<w:t xml:space="preserve">  line 2</w:t>`,
		"page break":        `<w:pageBreakBefore/>`,
	}
	for name, want := range wants {
		if !strings.Contains(doc, want) {
			t.Errorf("%s: document.xml lacks %q", name, want)
		}
	}
	if strings.Contains(doc, `<w:color w:val="008000"/>`) {
		t.Error("a:hover pseudo-class rule must not be applied")
	}

	num := parts["word/numbering.xml"]
	if !strings.Contains(num, `<w:start w:val="5"/><w:numFmt w:val="lowerLetter"/>`) {
		t.Error("ordered list start/type not honoured")
	}
	if !strings.Contains(parts["word/_rels/document.xml.rels"], `Target="https://example.com/?a=1&amp;b=2" TargetMode="External"`) {
		t.Error("hyperlink relationship missing or unescaped")
	}
	if !strings.Contains(parts["docProps/core.xml"], "<dc:title>รายงาน</dc:title>") {
		t.Error("title not taken from <title>")
	}
}

func TestConvertFragmentAndOptions(t *testing.T) {
	data, err := Convert(`สวัสดี <b>โลก</b>`, Options{Font: "Angsana New", FontSize: 14, PageSize: "Letter", Landscape: true})
	if err != nil {
		t.Fatal(err)
	}
	parts := unzip(t, data)
	doc := parts["word/document.xml"]
	if !strings.Contains(doc, `<w:pgSz w:w="15840" w:h="12240" w:orient="landscape"/>`) {
		t.Error("letter landscape page size not applied")
	}
	if !strings.Contains(doc, `สวัสดี </w:t>`) {
		t.Error("fragment text missing")
	}
	if !strings.Contains(parts["word/styles.xml"], `w:cs="Angsana New"/><w:sz w:val="28"/>`) {
		t.Error("default font/size not written to styles")
	}
}

func TestRemoteImagesBlockedByDefault(t *testing.T) {
	data, err := Convert(`<img src="http://127.0.0.1:1/x.png" alt="ภาพ">`, Options{})
	if err != nil {
		t.Fatal(err)
	}
	doc := unzip(t, data)["word/document.xml"]
	if strings.Contains(doc, "<w:drawing>") || !strings.Contains(doc, "[ภาพ]") {
		t.Error("remote image should be skipped and replaced by alt text")
	}
}
