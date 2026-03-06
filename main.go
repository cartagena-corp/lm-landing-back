package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sessionsbridge/auth"
	calendarSvc "sessionsbridge/calendar"
	"sessionsbridge/config"
	"sessionsbridge/handlers"

	"github.com/rs/cors"
)

func main() {
	// Cargar configuración
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error al cargar configuración: %v", err)
	}

	// Inicializar servicios
	oauthService := auth.NewOAuthService(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.RedirectURL)
	calendarService := calendarSvc.NewService()

	// Inicializar handlers
	sessionHandler := handlers.NewSessionHandler(oauthService, calendarService, cfg.AdminSecret)

	// Configurar rutas
	mux := http.NewServeMux()

	// Rutas de autenticación
	mux.HandleFunc("/auth/url", sessionHandler.GetAuthURL)
	mux.HandleFunc("/auth/url/json", sessionHandler.GetAuthURLJSON)
	mux.HandleFunc("/auth/callback", sessionHandler.HandleAuthCallback)
	mux.HandleFunc("/auth/status", sessionHandler.GetAuthStatus)

	// Rutas de la API
	mux.HandleFunc("/api/sessions", sessionHandler.CreateSession)
	mux.HandleFunc("/api/availability", sessionHandler.GetAvailability)

	// Ruta de health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Configurar CORS
	origins := strings.Split(cfg.AllowedOrigins, ",")
	c := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	handler := c.Handler(mux)

	// Configurar servidor
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Iniciar servidor en goroutine
	go func() {
		log.Printf("🚀 Servidor iniciado en http://localhost:%s", cfg.Port)
		if !oauthService.IsAuthorized() {
			log.Printf("⚠️  No autorizado. Visita http://localhost:%s/auth/url para obtener la URL de autorización", cfg.Port)
		} else {
			log.Println("✅ Token OAuth2 cargado. Listo para crear sesiones.")
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error del servidor: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Apagando servidor...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Error al apagar el servidor: %v", err)
	}

	log.Println("👋 Servidor apagado correctamente")
}
