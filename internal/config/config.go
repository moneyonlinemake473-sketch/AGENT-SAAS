package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Port                      string
	AdminAPIKey               string
	GeminiAPIKey              string
	GoogleCredentialsJSON     string
	FirestoreProjectID        string
	GoogleMasterSpreadsheetID string
	BaseURL                   string
	GeminiInputCostPer1K      float64
	GeminiOutputCostPer1K     float64
}

func Load() Config {
	cfg := Config{
		Port:                      getEnv("PORT", "8080"),
		AdminAPIKey:               mustGetEnv("ADMIN_API_KEY"),
		GeminiAPIKey:              mustGetEnv("GEMINI_API_KEY"),
		GoogleCredentialsJSON:     mustGetEnv("GOOGLE_CREDENTIALS_JSON"),
		FirestoreProjectID:        mustGetEnv("FIRESTORE_PROJECT_ID"),
		GoogleMasterSpreadsheetID: mustGetEnv("GOOGLE_MASTER_SPREADSHEET_ID"),
		BaseURL:                   getEnv("BASE_URL", "http://localhost:8080"),
		GeminiInputCostPer1K:      getEnvFloat("GEMINI_INPUT_COST_PER_1K", 0.000075),
		GeminiOutputCostPer1K:     getEnvFloat("GEMINI_OUTPUT_COST_PER_1K", 0.0003),
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("variable d'environnement manquante: %s", key)
	}
	return v
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}