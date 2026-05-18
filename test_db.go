package main

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

func main() {
	dsn := "postgresql://neondb_owner:npg_uIcM5kWHUpa2@ep-empty-voice-a1huvij7-pooler.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	var users []domain.User
	db.Find(&users)
	fmt.Printf("Users: %d\n", len(users))

	var campaigns []domain.Campaign
	db.Find(&campaigns)
	fmt.Printf("Campaigns: %d\n", len(campaigns))
}
