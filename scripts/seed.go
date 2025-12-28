package main

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/config"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"github.com/vaibhaw/influenzer-backend/pkg/database"
	"gorm.io/gorm"
)

func main() {
	cfg, _ := config.LoadConfig(".")
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	database.Connect(cfg.DatabaseURL)
	db := database.DB

	// Seed Brands
	brands := []struct {
		Email string
		Name  string
	}{
		{"brand1@example.com", "Cool Brand Inc"},
		{"brand2@example.com", "Tech Solutions"},
	}

	for _, b := range brands {
		createBrand(db, b.Email, b.Name)
	}

	// Seed Creators
	creators := []struct {
		Email string
		Niche string
	}{
		{"creator1@example.com", "Tech,Gaming"},
		{"creator2@example.com", "Lifestyle,Travel"},
	}

	for _, c := range creators {
		createCreator(db, c.Email, c.Niche)
	}
}

func createBrand(db *gorm.DB, email, companyName string) {
	userId := uuid.New()
	user := domain.User{
		ID:    userId,
		Email: email,
		Role:  domain.RoleBrand,
		BrandProfile: &domain.BrandProfile{
			UserID:      userId,
			CompanyName: companyName,
		},
	}
	if err := db.Create(&user).Error; err != nil {
		fmt.Printf("Might already exist: %s\n", email)
	} else {
		fmt.Printf("Created Brand: %s\n", email)
	}
}

func createCreator(db *gorm.DB, email, niche string) {
	userId := uuid.New()
	user := domain.User{
		ID:    userId,
		Email: email,
		Role:  domain.RoleCreator,
		CreatorProfile: &domain.CreatorProfile{
			UserID: userId,
			Niche:  niche,
		},
	}
	if err := db.Create(&user).Error; err != nil {
		fmt.Printf("Might already exist: %s\n", email)
	} else {
		fmt.Printf("Created Creator: %s\n", email)
	}
}
