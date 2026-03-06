package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

const tokenFile = "data/token.json"

// OAuthService gestiona el flujo de autenticación OAuth2 con Google.
type OAuthService struct {
	config *oauth2.Config
	token  *oauth2.Token
	mu     sync.RWMutex
}

// NewOAuthService crea un nuevo servicio OAuth2.
func NewOAuthService(clientID, clientSecret, redirectURL string) *OAuthService {
	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{calendar.CalendarScope},
		Endpoint:     google.Endpoint,
	}

	svc := &OAuthService{config: cfg}

	// Intentar cargar token existente
	if token, err := svc.loadToken(); err == nil {
		svc.token = token
	}

	return svc
}

// GetAuthURL devuelve la URL de autorización de Google OAuth2.
func (s *OAuthService) GetAuthURL() string {
	return s.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
}

// HandleCallback procesa el callback de OAuth2 y guarda el token.
func (s *OAuthService) HandleCallback(ctx context.Context, code string) error {
	token, err := s.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("no se pudo intercambiar el código: %w", err)
	}

	s.mu.Lock()
	s.token = token
	s.mu.Unlock()

	return s.saveToken(token)
}

// GetClient devuelve un http.Client autenticado.
// Retorna error si no se ha completado el flujo de autorización.
func (s *OAuthService) GetClient(ctx context.Context) (*http.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.token == nil {
		return nil, fmt.Errorf("no autorizado: visita /auth/url para iniciar el flujo de autenticación")
	}

	// TokenSource renueva automáticamente el token si ha expirado
	tokenSource := s.config.TokenSource(ctx, s.token)

	// Verificar si el token fue renovado y guardarlo
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("error al renovar token: %w", err)
	}
	if newToken.AccessToken != s.token.AccessToken {
		s.mu.RUnlock()
		s.mu.Lock()
		s.token = newToken
		_ = s.saveToken(newToken)
		s.mu.Unlock()
		s.mu.RLock()
	}

	return oauth2.NewClient(ctx, tokenSource), nil
}

// IsAuthorized indica si el servicio tiene un token válido.
func (s *OAuthService) IsAuthorized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token != nil
}

// saveToken guarda el token OAuth2 en un archivo JSON.
func (s *OAuthService) saveToken(token *oauth2.Token) error {
	f, err := os.Create(tokenFile)
	if err != nil {
		return fmt.Errorf("no se pudo crear archivo de token: %w", err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

// loadToken carga el token OAuth2 desde un archivo JSON.
func (s *OAuthService) loadToken() (*oauth2.Token, error) {
	f, err := os.Open(tokenFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var token oauth2.Token
	if err := json.NewDecoder(f).Decode(&token); err != nil {
		return nil, err
	}

	return &token, nil
}
