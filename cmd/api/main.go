package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/vaibhaw/influenzer-backend/config"
	httpDelivery "github.com/vaibhaw/influenzer-backend/internal/delivery/http"
	wsDelivery "github.com/vaibhaw/influenzer-backend/internal/delivery/ws"
	"github.com/vaibhaw/influenzer-backend/internal/repository"
	"github.com/vaibhaw/influenzer-backend/internal/service"
	"github.com/vaibhaw/influenzer-backend/pkg/database"
	"github.com/vaibhaw/influenzer-backend/pkg/razorpay"
	"github.com/vaibhaw/influenzer-backend/pkg/utils"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/vaibhaw/influenzer-backend/docs"
)

func main() {
	// 1. Load Config
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// 2. Init Logger
	utils.InitLogger()
	defer utils.Logger.Sync()

	utils.Logger.Info("Starting Influenzer Backend...")

	// 3. Connect Database
	utils.Logger.Info(fmt.Sprintf("DATABASE_URL length: %d", len(cfg.DatabaseURL)))
	if cfg.DatabaseURL != "" {
		database.Connect(cfg.DatabaseURL)
		database.AutoMigrate()
	} else {
		utils.Logger.Warn("DATABASE_URL not set, skipping DB connection")
	}

	// 4. Setup Router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httpDelivery.RequestLoggerMiddleware())

	// CORS Config
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:8081", "http://localhost:3000"}, // Added potential generic port too
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// 5. Setup Services
	// - Auth
	authRepo := repository.NewAuthRepository(database.DB)
	authService := service.NewAuthService(authRepo, &cfg)

	// - Middleware
	authMiddleware := httpDelivery.AuthMiddleware(&cfg)

	httpDelivery.NewAuthHandler(r, authService, authMiddleware)

	// - Marketplace
	campaignRepo := repository.NewCampaignRepository(database.DB)
	campaignService := service.NewCampaignService(campaignRepo)

	proposalRepo := repository.NewProposalRepository(database.DB)
	proposalService := service.NewProposalService(proposalRepo)

	// - Notifications (init before handlers that trigger them)
	notifRepo := repository.NewNotificationRepository(database.DB)
	notifService := service.NewNotificationService(notifRepo)
	httpDelivery.NewNotificationHandler(r, notifService, authMiddleware)
	httpDelivery.NewCampaignHandler(r, campaignService, authMiddleware, notifService, database.DB)
	httpDelivery.NewProposalHandler(r, proposalService, authMiddleware, notifService, campaignRepo, database.DB)

	// - Payments
	rzpClient := razorpay.NewRazorpayClient(&cfg)
	paymentService := service.NewPaymentService(proposalRepo, campaignRepo, rzpClient)
	httpDelivery.NewPaymentHandler(r, paymentService, authMiddleware, database.DB, rzpClient)

	// - Social
	socialService := service.NewSocialService(proposalRepo, campaignRepo, authRepo)
	httpDelivery.NewSocialHandler(r, socialService, authMiddleware)

	// - New Modules (Refactor)
	httpDelivery.NewJobHandler(r, database.DB, authMiddleware)
	httpDelivery.NewCreatorHandler(r, database.DB, authMiddleware)
	httpDelivery.NewBrandHandler(r, database.DB, authMiddleware)
	httpDelivery.NewChatHTTPHandler(r, database.DB, authMiddleware)

	// - Chat (WS) - with DB for persistence and JWT for auth
	wsDelivery.NewChatHandler(r, database.DB, cfg.JWTSecret)

	// - Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Log all registered routes
	utils.Logger.Info("Registered Routes:")
	for _, route := range r.Routes() {
		utils.Logger.Info(fmt.Sprintf("%s %s", route.Method, route.Path))
	}

	// 6. Start Server
	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	utils.Logger.Info(fmt.Sprintf("Server starting on port %s", port))
	r.Run(":" + port)
}
