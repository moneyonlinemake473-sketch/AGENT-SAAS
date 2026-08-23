package config

import (
	"log"
	"os"
	"strconv"
)

// Config regroupe toute la configuration lue depuis les variables d'environnement.
// Sur Render, ces variables se configurent dans l'onglet "Environment" du service.
type Config struct {
	Port                  string
	AdminAPIKey           string // clé secrète pour protéger les routes /admin/*
	GeminiAPIKey          string
	GoogleCredentialsJSON string // contenu JSON du compte de service (Firestore + Sheets)
	FirestoreProjectID    string
	BaseURL               string // ex: https://mon-service.onrender.com
	// Coûts Gemini en USD par 1000 tokens — À VÉRIFIER sur https://ai.google.dev/pricing
	// avant mise en prod, les tarifs changent régulièrement.
	GeminiInputCostPer1K  float64
	GeminiOutputCostPer1K float64
}

func Load() Config {
	cfg := Config{
		Port:                  getEnv("PORT", "8080"),
		AdminAPIKey:           mustGetEnv("ADMIN_API_KEY"),
		GeminiAPIKey:          mustGetEnv("GEMINI_API_KEY"),
		GoogleCredentialsJSON: mustGetEnv("GOOGLE_CREDENTIALS_JSON"),
		FirestoreProjectID:    mustGetEnv("FIRESTORE_PROJECT_ID"),
		BaseURL:               getEnv("BASE_URL", "http://localhost:8080"),
		GeminiInputCostPer1K:  getEnvFloat("GEMINI_INPUT_COST_PER_1K", 0.000075), // exemple Gemini 1.5 Flash, À VÉRIFIER
		GeminiOutputCostPer1K: getEnvFloat("GEMINI_OUTPUT_COST_PER_1K", 0.0003),  // exemple, À VÉRIFIER
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
	// implémentation volontairement simple, voir strconv.ParseFloat
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
