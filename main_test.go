package main

import (
	"archive/zip"
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"gogeneratedocs/config"
	"gogeneratedocs/helper"
)

func TestConvertEndpoint(t *testing.T) {
	app := newApp(testConfig)

	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	fw, _ := mw.CreateFormFile("file", "รายงาน.html")
	fw.Write([]byte("<h1>ทดสอบ</h1>"))
	mw.Close()

	cases := []struct {
		name, contentType, body, url string
		wantStatus                   int
		wantFile                     string
	}{
		{"json", "application/json", `{"html":"<p>สวัสดี</p>","filename":"ใบเสนอราคา"}`, "/api/convert", 200, "%E0%B9%83%E0%B8%9A"},
		{"raw html", "text/html; charset=utf-8", "<p>hello</p>", "/api/convert?filename=report", 200, `filename="report.docx"`},
		{"multipart", mw.FormDataContentType(), form.String(), "/api/convert", 200, "%E0%B8%A3"},
		{"empty", "application/json", `{"html":"  "}`, "/api/convert", 400, ""},
		{"bad json", "application/json", `{`, "/api/convert", 400, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.url, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d, want %d: %s", resp.StatusCode, tc.wantStatus, body)
			}
			if tc.wantStatus != 200 {
				return
			}
			if ct := resp.Header.Get("Content-Type"); ct != helper.DocxMIME {
				t.Errorf("content-type %q", ct)
			}
			if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, tc.wantFile) {
				t.Errorf("content-disposition %q lacks %q", cd, tc.wantFile)
			}
			if !bytes.HasPrefix(body, []byte("PK")) {
				t.Error("body is not a zip/docx")
			}
		})
	}
}

// documentXML extracts word/document.xml from a .docx response body.
func documentXML(t *testing.T, docx []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("not a docx: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			return string(b)
		}
	}
	t.Fatal("word/document.xml missing")
	return ""
}

func TestConvertHTMLCSSEndpoint(t *testing.T) {
	app := newApp(testConfig)
	const html = `<style>.title{color:#00FF00}</style><h1 class="title">หัวเรื่อง</h1><p class="note">หมายเหตุ</p>`
	const css = `.title{color:#1F4E79} .note{font-weight:bold; text-align:center}`

	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	hw, _ := mw.CreateFormFile("htmlFile", "ใบงาน.html")
	hw.Write([]byte(html))
	cw, _ := mw.CreateFormFile("cssFile", "style.css")
	cw.Write([]byte(css))
	mw.WriteField("fontSize", "16")
	mw.Close()

	jsonBody := `{"html":` + strconv.Quote(html) + `,"css":` + strconv.Quote(css) + `,"filename":"report"}`
	cases := []struct {
		name, contentType, body string
		wantStatus              int
		wantFile                string
	}{
		{"json", "application/json", jsonBody, 200, `filename="report.docx"`},
		{"multipart files", mw.FormDataContentType(), form.String(), 200, "%E0%B9%83%E0%B8%9A"},
		{"raw body rejected", "text/html", html, 415, ""},
		{"missing html", "application/json", `{"css":".a{}"}`, 400, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/convert/html-css", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status %d, want %d: %s", resp.StatusCode, tc.wantStatus, body)
			}
			if tc.wantStatus != 200 {
				return
			}
			if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, tc.wantFile) {
				t.Errorf("content-disposition %q lacks %q", cd, tc.wantFile)
			}
			doc := documentXML(t, body)
			if !strings.Contains(doc, `<w:color w:val="1F4E79"/>`) || strings.Contains(doc, `00FF00`) {
				t.Error("separate CSS should override the <style> block on equal specificity")
			}
			if !strings.Contains(doc, `<w:jc w:val="center"/>`) || !strings.Contains(doc, `<w:b/><w:bCs/>`) {
				t.Error("separate CSS rules for .note not applied")
			}
		})
	}
}

func TestConvertIgnoresSeparateCSS(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/convert", strings.NewReader(`{"html":"<p class=\"x\">a</p>","css":".x{color:red}"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := newApp(testConfig).Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(documentXML(t, body), "FF0000") {
		t.Error("/api/convert must not apply the css field")
	}
}

var testConfig = config.Config{Port: "3000", BodyLimitMB: 20, CORSAllowOrigins: "*"}
