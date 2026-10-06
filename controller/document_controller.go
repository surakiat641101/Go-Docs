// Package controller turns HTTP requests into service calls and service
// results into HTTP responses.
package controller

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"gogeneratedocs/helper"
	"gogeneratedocs/htmldocx"
	"gogeneratedocs/service"
)

// ConvertRequest is the JSON body accepted by the convert endpoints.
type ConvertRequest struct {
	HTML     string           `json:"html"`
	CSS      string           `json:"css"`
	Filename string           `json:"filename"`
	Options  htmldocx.Options `json:"options"`
}

type DocumentController struct {
	svc service.DocumentService
}

func NewDocumentController(svc service.DocumentService) *DocumentController {
	return &DocumentController{svc: svc}
}

// Convert handles POST /api/convert. It accepts HTML three ways:
//   - application/json:    {"html": "...", "filename": "report.docx", "options": {...}}
//   - multipart/form-data: field "file" (an .html file) or "html", plus optional "filename"
//   - anything else (e.g. text/html): the raw body is the HTML; ?filename=... optional
func (ctl *DocumentController) Convert(c *fiber.Ctx) error {
	var req ConvertRequest
	switch {
	case helper.IsJSON(c):
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid JSON: "+err.Error())
		}
		req.CSS = "" // separate CSS belongs to /api/convert/html-css
	case helper.IsMultipart(c):
		req.HTML = c.FormValue("html")
		req.Filename = c.FormValue("filename")
		if err := helper.ReadFormFile(c, "file", &req.HTML, &req.Filename); err != nil {
			return err
		}
		req.Options = helper.FormOptions(c)
	default:
		req.HTML = string(c.Body())
		req.Filename = c.Query("filename")
		req.Options = helper.QueryOptions(c)
	}
	return ctl.generate(c, req)
}

// ConvertHTMLCSS handles POST /api/convert/html-css, taking HTML and CSS separately:
//   - application/json:    {"html": "...", "css": "...", "filename": "...", "options": {...}}
//   - multipart/form-data: text fields "html" and "css", or file fields
//     "htmlFile" and "cssFile" (a file wins over the text field), plus
//     "filename", "font", "fontSize", "pageSize", "landscape"
func (ctl *DocumentController) ConvertHTMLCSS(c *fiber.Ctx) error {
	var req ConvertRequest
	switch {
	case helper.IsJSON(c):
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid JSON: "+err.Error())
		}
	case helper.IsMultipart(c):
		req.HTML = c.FormValue("html")
		req.CSS = c.FormValue("css")
		req.Filename = c.FormValue("filename")
		if err := helper.ReadFormFile(c, "htmlFile", &req.HTML, &req.Filename); err != nil {
			return err
		}
		if err := helper.ReadFormFile(c, "cssFile", &req.CSS, nil); err != nil {
			return err
		}
		req.Options = helper.FormOptions(c)
	default:
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "use application/json or multipart/form-data")
	}
	return ctl.generate(c, req)
}

func (ctl *DocumentController) generate(c *fiber.Ctx, req ConvertRequest) error {
	data, err := ctl.svc.GenerateDocx(service.GenerateInput{HTML: req.HTML, CSS: req.CSS, Options: req.Options})
	switch {
	case errors.Is(err, service.ErrHTMLRequired):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case err != nil:
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}
	return helper.SendDocx(c, data, req.Filename)
}
