package main

import (
	"time"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID    uint   `gorm:"primaryKey"`
	Name  string `gorm:"unique"`
	Email string
	Keys  []APIKey // One user can have many API keys
}

// APIKey model (Stores JWT ID and associated user)
type APIKey struct {
	ID        uint       `gorm:"primaryKey"`
	UserID    uint       `gorm:"index"`   // Foreign key to User
	JWTID     string     `gorm:"unique"`  // JWT ID (tracks API key)
	CreatedAt time.Time
	Expiry    time.Time  // Expiry date of the JWT
	RevokedAt *time.Time // NULL if active, set when revoked
}

// Store JWT ID and user mapping
func storeAPIKey(jwtID string, userID uint, expiry time.Time) error {
	apiKey := APIKey{
		UserID: userID,
		JWTID:  jwtID,
		Expiry: expiry,
	}

	return db.Create(&apiKey).Error
}

// Check if JWT ID is valid and belongs to a user
func isJWTValid(jwtID string) (bool, uint) {
	var apiKey APIKey
	if err := db.Where("jwt_id = ?", jwtID).First(&apiKey).Error; err != nil {
		return false, 0 // JWT ID not found
	}
	return true, apiKey.UserID
}