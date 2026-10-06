// Package helper contains small HTTP utilities shared by the controllers.
package helper

import (
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"gogeneratedocs/htmldocx"
)

// IsJSON reports whether the request body is JSON.
func IsJSON(c *fiber.Ctx) bool { return hasContentType(c, fiber.MIMEApplicationJSON) }

// IsMultipart reports whether the request body is multipart/form-data.
func IsMultipart(c *fiber.Ctx) bool { return hasContentType(c, fiber.MIMEMultipartForm) }

func hasContentType(c *fiber.Ctx, mime string) bool {
	return strings.HasPrefix(strings.ToLower(string(c.Request().Header.ContentType())), mime)
}

// ReadFormFile loads an uploaded file into *dst when field is present. When
// name is non-nil and still empty, it is set from the uploaded file name.
func ReadFormFile(c *fiber.Ctx, field string, dst, name *string) error {
	fh, err := c.FormFile(field)
	if err != nil {
		return nil // field not sent
	}
	f, err := fh.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot read uploaded file "+field)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "cannot read uploaded file "+field)
	}
	*dst = string(b)
	if name != nil && *name == "" {
		*name = strings.TrimSuffix(fh.Filename, filepath.Ext(fh.Filename))
	}
	return nil
}

// FormOptions reads conversion options from multipart form fields.
func FormOptions(c *fiber.Ctx) htmldocx.Options {
	return optionsFrom(func(k string) string { return c.FormValue(k) })
}

// QueryOptions reads conversion options from query parameters.
func QueryOptions(c *fiber.Ctx) htmldocx.Options {
	return optionsFrom(func(k string) string { return c.Query(k) })
}

func optionsFrom(get func(key string) string) htmldocx.Options {
	var o htmldocx.Options
	o.Font = get("font")
	o.FontSize, _ = strconv.ParseFloat(get("fontSize"), 64)
	o.PageSize = get("pageSize")
	o.Landscape = get("landscape") == "true"
	return o
}
