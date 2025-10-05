package main

import (
	"log"
	"os"

	"goviesdeze/internal/config"
	"goviesdeze/internal/handlers"
	"goviesdeze/internal/middleware"
	"goviesdeze/internal/utils"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize storage usage
	if err := utils.LoadUsage(); err != nil {
		log.Printf("Warning: Failed to load usage: %v", err)
	}

	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Setup Gin router
	router := gin.Default()

	// Add middleware
	router.Use(middleware.RequestLogger())
	if cfg.RequireAPIKey {
		router.Use(middleware.APIKeyAuth(cfg.APIKey))
	}

	// Register routes
	handlers.RegisterRoutes(router, cfg)

	// Start server
	log.Printf("Server running on port %s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
