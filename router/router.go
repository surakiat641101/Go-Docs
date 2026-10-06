// Package router creates the Fiber app: middleware and every route.
package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"gogeneratedocs/config"
	"gogeneratedocs/controller"
)

// Controllers groups the handlers the router needs; add new controllers here.
type Controllers struct {
	Document *controller.DocumentController
}

// New builds the Fiber app with middleware and routes registered.
func Routes(cfg config.Config, ctl Controllers) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:   "Go-Docs",
		BodyLimit: cfg.BodyLimitMB << 20,
	})
	app.Use(recover.New(), logger.New(), cors.New(cors.Config{AllowOrigins: cfg.CORSAllowOrigins}))

	app.Get("/", health)

	api := app.Group("/api")
	api.Post("/convert", ctl.Document.Convert)
	api.Post("/convert/html-css", ctl.Document.ConvertHTMLCSS)

	return app
}

func health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
