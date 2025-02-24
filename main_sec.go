


package main

import (
	"github.com/golang-jwt/jwt/v5"
	"time"
	"fmt"
	"encoding/json"
	"net/http"
	"strings"
	"net/http"
	"sync"
	"golang.org/x/time/rate"
)

// Rate limiters map
var rateLimiters = make(map[uint]*rate.Limiter)
var mu sync.Mutex

// Get rate limiter for user
func getRateLimiter(userID uint) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	if limiter, exists := rateLimiters[userID]; exists {
		return limiter
	}

	// Limit: 5 requests per minute
	limiter := rate.NewLimiter(5, 5)
	rateLimiters[userID] = limiter
	return limiter
}
// JWT Secret
var jwtSecret = []byte("your-secret-key")


func isJWTValid(jwtID string) (bool, uint) {
	var apiKey APIKey
	if err := db.Where("jwt_id = ?", jwtID).First(&apiKey).Error; err != nil {
		return false, 0 // JWT ID not found
	}

	// Check if revoked
	if apiKey.RevokedAt != nil {
		return false, 0 // Token has been revoked
	}

	return true, apiKey.UserID
}


// Generate JWT for API key
func GenerateJWT(userID uint, resources []string, expiryHours int) (string, string, error) {
	// Set expiry time for the JWT
	expirationTime := time.Now().Add(time.Duration(expiryHours) * time.Hour)

	// Create JWT claims
	claims := jwt.MapClaims{
		"user_id":           userID,
		"allowed_resources": resources,
		"exp":               expirationTime.Unix(),
		"jti":               fmt.Sprintf("key-%d-%d", userID, time.Now().UnixNano()), // Generate unique JWT ID
	}

	// Create the JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", "", err
	}

	// Store the JWT ID in the database
	jwtID := claims["jti"].(string)
	err = storeAPIKey(jwtID, userID, expirationTime)
	if err != nil {
		return "", "", err
	}

	return tokenString, jwtID, nil
}



// Validate JWT
func verifyTokenHandler(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
		return
	}

	// Extract the token from the "Bearer <token>" header
	tokenParts := strings.Split(authHeader, " ")
	if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
		http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
		return
	}

	tokenString := tokenParts[1]
	claims := &jwt.MapClaims{}

	// Parse the JWT token
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	// Check for errors
	if err != nil || !token.Valid {
		http.Error(w, "Invalid token", http.StatusForbidden)
		return
	}

	// Extract the JWT ID (JTI) from the claims
	jwtID := claims["jti"].(string)

	// Check if JWT ID is valid and exists in the database
	isValid, userID := isJWTValid(jwtID)
	if !isValid {
		http.Error(w, "Invalid or expired API key", http.StatusForbidden)
		return
	}

	// Check RBAC: If the user is allowed to access the requested resource
	allowedResources := claims["allowed_resources"].([]interface{})
	requestedResource := r.URL.Query().Get("resource")

	// Check if the requested resource is in the list of allowed resources
	accessAllowed := false
	for _, res := range allowedResources {
		if res == requestedResource {
			accessAllowed = true
			break
		}
	}

	if !accessAllowed {
		http.Error(w, "Access Denied", http.StatusForbidden)
		return
	}

	// JWT is valid, user exists, and access is granted
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Access granted",
		"user_id": fmt.Sprintf("%d", userID),
		"resource": requestedResource,
	})
}