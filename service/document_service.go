// Package service holds the business logic for generating documents. It has
// no knowledge of HTTP, so it can be reused from a CLI, a queue worker, etc.
package service

import (
	"errors"
	"strings"

	"gogeneratedocs/htmldocx"
)

// ErrHTMLRequired is returned when the input has no HTML to render.
var ErrHTMLRequired = errors.New("html is required")

// GenerateInput is everything needed to build one .docx file.
type GenerateInput struct {
	HTML    string
	CSS     string // optional stylesheet applied after the HTML's own <style> blocks
	Options htmldocx.Options
}

// DocumentService generates Word documents.
type DocumentService interface {
	GenerateDocx(in GenerateInput) ([]byte, error)
}

// Config holds server-wide settings that callers must not control per request.
type Config struct {
	AllowRemoteImages bool
}

type documentService struct {
	cfg Config
}

func NewDocumentService(cfg Config) DocumentService {
	return &documentService{cfg: cfg}
}

func (s *documentService) GenerateDocx(in GenerateInput) ([]byte, error) {
	if strings.TrimSpace(in.HTML) == "" {
		return nil, ErrHTMLRequired
	}
	in.Options.AllowRemoteImages = s.cfg.AllowRemoteImages
	return htmldocx.ConvertWithCSS(in.HTML, in.CSS, in.Options)
}
