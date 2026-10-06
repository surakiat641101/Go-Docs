package service

import (
	"bytes"
	"errors"
	"testing"
)

func TestGenerateDocx(t *testing.T) {
	svc := NewDocumentService(Config{})

	if _, err := svc.GenerateDocx(GenerateInput{HTML: " \n "}); !errors.Is(err, ErrHTMLRequired) {
		t.Fatalf("blank html: err = %v, want ErrHTMLRequired", err)
	}

	data, err := svc.GenerateDocx(GenerateInput{HTML: "<p class=a>สวัสดี</p>", CSS: ".a{color:red}"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		t.Error("result is not a docx (zip)")
	}
}
