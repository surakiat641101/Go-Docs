package main

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"gogeneratedocs/config"
	"gogeneratedocs/controller"
	"gogeneratedocs/router"
	"gogeneratedocs/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(newApp(cfg).Listen(":" + cfg.Port))
}

// newApp wires dependencies (service → controller → router).
func newApp(cfg config.Config) *fiber.App {
	docSvc := service.NewDocumentService(service.Config{
		AllowRemoteImages: cfg.AllowRemoteImages,
	})
	return router.Routes(cfg, router.Controllers{
		Document: controller.NewDocumentController(docSvc),
	})
}
