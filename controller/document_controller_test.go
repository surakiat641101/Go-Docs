package controller

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"gogeneratedocs/service"
)

// fakeService records the input it receives and returns a canned result.
type fakeService struct {
	got  service.GenerateInput
	data []byte
	err  error
}

func (f *fakeService) GenerateDocx(in service.GenerateInput) ([]byte, error) {
	f.got = in
	return f.data, f.err
}

func newTestApp(svc service.DocumentService) *fiber.App {
	ctl := NewDocumentController(svc)
	app := fiber.New()
	app.Post("/convert", ctl.Convert)
	app.Post("/convert/html-css", ctl.ConvertHTMLCSS)
	return app
}

func do(t *testing.T, app *fiber.App, url, contentType, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestControllerPassesInputToService(t *testing.T) {
	svc := &fakeService{data: []byte("DOCX")}
	app := newTestApp(svc)

	status, body := do(t, app, "/convert/html-css", "application/json",
		`{"html":"<p>a</p>","css":"p{color:red}","options":{"font":"TH Sarabun New","fontSize":16}}`)
	if status != 200 || body != "DOCX" {
		t.Fatalf("got %d %q", status, body)
	}
	if svc.got.HTML != "<p>a</p>" || svc.got.CSS != "p{color:red}" ||
		svc.got.Options.Font != "TH Sarabun New" || svc.got.Options.FontSize != 16 {
		t.Errorf("service got %+v", svc.got)
	}

	do(t, app, "/convert", "application/json", `{"html":"<p>a</p>","css":"p{color:red}"}`)
	if svc.got.CSS != "" {
		t.Error("/convert must not forward css")
	}

	do(t, app, "/convert?font=Tahoma&landscape=true", "text/html", "<p>raw</p>")
	if svc.got.HTML != "<p>raw</p>" || svc.got.Options.Font != "Tahoma" || !svc.got.Options.Landscape {
		t.Errorf("raw body/query options not forwarded: %+v", svc.got)
	}
}

func TestControllerMapsServiceErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{service.ErrHTMLRequired, 400},
		{errors.New("boom"), 422},
	}
	for _, tc := range cases {
		app := newTestApp(&fakeService{err: tc.err})
		if status, _ := do(t, app, "/convert", "application/json", `{"html":"x"}`); status != tc.want {
			t.Errorf("err %v: status %d, want %d", tc.err, status, tc.want)
		}
	}
}
