package database

import (
	"log"
	"time"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(dsn string) {
	var err error
	// Disable prepared statements to avoid cache invalidation issues with Neon serverless
	DB, err = gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true, // Disables implicit prepared statement usage
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Connection Pool for Serverless (Neon)
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Failed to get generic database object:", err)
	}

	// Neon serverless handles connection scaling, but good to have limits
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("Database connected successfully")
}

func AutoMigrate() {
	err := DB.AutoMigrate(
		&domain.User{},
		&domain.BrandProfile{},
		&domain.CreatorProfile{},
		&domain.Campaign{},
		&domain.Proposal{},
		&domain.Message{},
		&domain.Transaction{},
		&domain.SubscriptionPlan{},
		&domain.Subscription{},
	)
	if err != nil {
		log.Fatal("Migration failed:", err)
	}
	log.Println("Database migration completed")
	SeedPlans()
}

func SeedPlans() {
	var count int64
	DB.Model(&domain.SubscriptionPlan{}).Count(&count)
	if count > 0 {
		return
	}

	plans := []domain.SubscriptionPlan{
		{
			Name:           "Pro Annual",
			Description:    "Yearly Subscription",
			Amount:         10000,
			Currency:       "INR",
			Duration:       365,
			RazorpayPlanID: "", // To be filled by user
			IsActive:       true,
		},
		{
			Name:           "Pro Monthly",
			Description:    "Monthly Subscription",
			Amount:         499,
			Currency:       "INR",
			Duration:       30,
			RazorpayPlanID: "", // To be filled by user
			IsActive:       true,
		},
	}

	if err := DB.Create(&plans).Error; err != nil {
		log.Printf("Failed to seed plans: %v", err)
	} else {
		log.Println("Seeded default subscription plans")
	}
}
