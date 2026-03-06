package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"sessionsbridge/auth"
	calendarSvc "sessionsbridge/calendar"
	"sessionsbridge/models"
)

// SessionHandler maneja las solicitudes HTTP para agendar sesiones.
type SessionHandler struct {
	oauthService    *auth.OAuthService
	calendarService *calendarSvc.Service
	adminSecret     string
}

// NewSessionHandler crea un nuevo handler de sesiones.
func NewSessionHandler(oauth *auth.OAuthService, cal *calendarSvc.Service, adminSecret string) *SessionHandler {
	return &SessionHandler{
		oauthService:    oauth,
		calendarService: cal,
		adminSecret:     adminSecret,
	}
}

// requireAdmin valida que la solicitud incluya el secreto de admin correcto.
func (h *SessionHandler) requireAdmin(r *http.Request) bool {
	secret := r.URL.Query().Get("secret")
	return secret == h.adminSecret
}

// GetAuthURL redirige al usuario a la página de autorización de Google OAuth2.
// GET /auth/url?secret=TU_SECRETO
func (h *SessionHandler) GetAuthURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}
	if !h.requireAdmin(r) {
		respondError(w, http.StatusForbidden, "Acceso denegado")
		return
	}

	url := h.oauthService.GetAuthURL()
	log.Printf("Redirigiendo a URL de autorización: %s", url)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// GetAuthURLJSON retorna la URL de autorización como JSON (para consumo del frontend).
// GET /auth/url/json?secret=TU_SECRETO
func (h *SessionHandler) GetAuthURLJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}
	if !h.requireAdmin(r) {
		respondError(w, http.StatusForbidden, "Acceso denegado")
		return
	}

	url := h.oauthService.GetAuthURL()

	respondJSON(w, http.StatusOK, map[string]string{
		"auth_url": url,
	})
}

// HandleAuthCallback procesa el callback de OAuth2.
// GET /auth/callback
func (h *SessionHandler) HandleAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "Falta el parámetro 'code'")
		return
	}

	if err := h.oauthService.HandleCallback(r.Context(), code); err != nil {
		log.Printf("Error en callback OAuth2: %v", err)
		respondError(w, http.StatusInternalServerError, "Error al procesar la autorización")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "¡Autorización completada exitosamente! Ya puedes crear sesiones de Meet.",
	})
}

// GetAuthStatus retorna el estado de autorización actual.
// GET /auth/status
func (h *SessionHandler) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"authorized": h.oauthService.IsAuthorized(),
	})
}

// GetAvailability retorna los horarios disponibles para un mes completo.
// GET /api/availability?month=2026-03&timezone=America/Bogota
func (h *SessionHandler) GetAvailability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	if !h.oauthService.IsAuthorized() {
		respondError(w, http.StatusUnauthorized, "No autorizado.")
		return
	}

	month := r.URL.Query().Get("month")
	if month == "" {
		respondError(w, http.StatusBadRequest, "El parámetro 'month' es requerido (formato: YYYY-MM)")
		return
	}

	timezone := r.URL.Query().Get("timezone")
	if timezone == "" {
		timezone = "America/Bogota"
	}

	client, err := h.oauthService.GetClient(r.Context())
	if err != nil {
		log.Printf("Error al obtener cliente OAuth2: %v", err)
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	availability, err := h.calendarService.GetAvailableSlots(r.Context(), client, month, timezone)
	if err != nil {
		log.Printf("Error al consultar disponibilidad: %v", err)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, availability)
}

// CreateSession crea una nueva sesión de Meet en Google Calendar.
// POST /api/sessions
func (h *SessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "Método no permitido")
		return
	}

	if !h.oauthService.IsAuthorized() {
		respondError(w, http.StatusUnauthorized, "No autorizado. Visita /auth/url para iniciar la autenticación.")
		return
	}

	var req models.SessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido en el cuerpo de la solicitud")
		return
	}
	defer r.Body.Close()

	if req.Title == "" {
		respondError(w, http.StatusBadRequest, "El campo 'title' es requerido")
		return
	}
	if req.StartTime == "" {
		respondError(w, http.StatusBadRequest, "El campo 'start_time' es requerido")
		return
	}
	if req.EndTime == "" {
		respondError(w, http.StatusBadRequest, "El campo 'end_time' es requerido")
		return
	}

	client, err := h.oauthService.GetClient(r.Context())
	if err != nil {
		log.Printf("Error al obtener cliente OAuth2: %v", err)
		respondError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Verificar disponibilidad antes de crear
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Formato inválido para start_time")
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Formato inválido para end_time")
		return
	}

	available, err := h.calendarService.IsSlotAvailable(r.Context(), client, startTime, endTime)
	if err != nil {
		log.Printf("Error al verificar disponibilidad: %v", err)
		respondError(w, http.StatusInternalServerError, "Error al verificar disponibilidad")
		return
	}
	if !available {
		respondError(w, http.StatusConflict, "El horario seleccionado no está disponible. Por favor elige otro.")
		return
	}

	session, err := h.calendarService.CreateMeetSession(r.Context(), client, req)
	if err != nil {
		log.Printf("Error al crear sesión: %v", err)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, session)
}

// respondJSON envía una respuesta JSON.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error al escribir respuesta JSON: %v", err)
	}
}

// respondError envía una respuesta de error JSON.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, models.ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}
