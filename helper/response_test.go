package helper

import (
	"strings"
	"testing"
)

func TestContentDisposition(t *testing.T) {
	cases := map[string]string{
		"รายงาน":          `filename="______.docx"; filename*=UTF-8''%E0%B8%A3%E0%B8%B2%E0%B8%A2%E0%B8%87%E0%B8%B2%E0%B8%99.docx`,
		"report.DOCX":     `filename="report.DOCX"`,
		"":                `filename="document.docx"`,
		`..\..\evil"name`: `filename="evilname.docx"`,
		"a/b/c.docx":      `filename="c.docx"`,
	}
	for in, want := range cases {
		if got := ContentDisposition(in); !strings.Contains(got, want) {
			t.Errorf("ContentDisposition(%q) = %s, want it to contain %s", in, got, want)
		}
	}
}
