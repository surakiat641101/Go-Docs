package helper

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const DocxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// SendDocx writes data as a .docx file download.
func SendDocx(c *fiber.Ctx, data []byte, filename string) error {
	c.Set(fiber.HeaderContentType, DocxMIME)
	c.Set(fiber.HeaderContentDisposition, ContentDisposition(filename))
	return c.Send(data)
}

// ContentDisposition builds an attachment header that also works for Thai
// file names (RFC 6266 filename* with an ASCII fallback).
func ContentDisposition(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`"/\:*?<>|`, r) {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." {
		name = "document"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".docx") {
		name += ".docx"
	}
	ascii := strings.Map(func(r rune) rune {
		if r > 0x7e {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(name))
}
