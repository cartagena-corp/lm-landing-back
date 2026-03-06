package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config contiene la configuración de la aplicación.
type Config struct {
	GoogleClientID     string
	GoogleClientSecret string
	RedirectURL        string
	Port               string
	AllowedOrigins     string
	AdminSecret        string
}

// Load carga la configuración desde variables de entorno y archivo .env.
func Load() (*Config, error) {
	// Intentar cargar .env (no es error si no existe)
	_ = godotenv.Load()

	cfg := &Config{
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:        os.Getenv("REDIRECT_URL"),
		Port:               os.Getenv("PORT"),
		AllowedOrigins:     os.Getenv("ALLOWED_ORIGINS"),
		AdminSecret:        os.Getenv("ADMIN_SECRET"),
	}

	if cfg.GoogleClientID == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID es requerido")
	}
	if cfg.GoogleClientSecret == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_SECRET es requerido")
	}
	if cfg.AdminSecret == "" {
		return nil, fmt.Errorf("ADMIN_SECRET es requerido: define un secreto para proteger los endpoints de autenticación")
	}

	if cfg.RedirectURL == "" {
		cfg.RedirectURL = "http://localhost:8080/auth/callback"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}

	return cfg, nil
}
