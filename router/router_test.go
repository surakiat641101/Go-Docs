package router

import (
	"net/http/httptest"
	"strings"
	"testing"

	"gogeneratedocs/config"
	"gogeneratedocs/controller"
	"gogeneratedocs/service"
)

func TestRoutes(t *testing.T) {
	cfg := config.Config{BodyLimitMB: 1, CORSAllowOrigins: "*"}
	app := Routes(cfg, Controllers{
		Document: controller.NewDocumentController(service.NewDocumentService(service.Config{})),
	})

	cases := []struct {
		method, path, contentType, body string
		want                            int
	}{
		{"GET", "/", "", "", 200},
		{"GET", "/health", "", "", 200},
		{"POST", "/api/convert", "text/html", "<p>a</p>", 200},
		{"POST", "/api/convert/html-css", "application/json", `{"html":"<p>a</p>"}`, 200},
		{"GET", "/api/convert", "", "", 405},
		{"GET", "/nope", "", "", 404},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
		}
	}
}

func TestBodyLimitFromConfig(t *testing.T) {
	app := Routes(config.Config{BodyLimitMB: 1, CORSAllowOrigins: "*"}, Controllers{
		Document: controller.NewDocumentController(service.NewDocumentService(service.Config{})),
	})
	req := httptest.NewRequest("POST", "/api/convert", strings.NewReader(strings.Repeat("x", 2<<20)))
	req.Header.Set("Content-Type", "text/html")
	// fasthttp rejects the oversized body before any handler runs.
	if _, err := app.Test(req, -1); err == nil || !strings.Contains(err.Error(), "body size exceeds") {
		t.Errorf("2 MB body with BODY_LIMIT_MB=1: err = %v, want body size error", err)
	}
}
