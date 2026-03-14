package calendar

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"lm-landing-back/models"

	"github.com/google/uuid"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Service gestiona la creación de eventos en Google Calendar.
type Service struct{}

// NewService crea una nueva instancia del servicio de Calendar.
func NewService() *Service {
	return &Service{}
}

// CreateMeetSession crea un evento en Google Calendar con un link de Google Meet.
func (s *Service) CreateMeetSession(ctx context.Context, client *http.Client, req models.SessionRequest) (*models.SessionResponse, error) {
	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el servicio de Calendar: %w", err)
	}

	// Parsear las fechas
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("formato inválido para start_time (use RFC3339, ej: 2026-02-20T10:00:00-05:00): %w", err)
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("formato inválido para end_time (use RFC3339, ej: 2026-02-20T11:00:00-05:00): %w", err)
	}

	if endTime.Before(startTime) || endTime.Equal(startTime) {
		return nil, fmt.Errorf("end_time debe ser posterior a start_time")
	}

	// Crear lista de asistentes
	var attendees []*calendar.EventAttendee
	for _, email := range req.Attendees {
		attendees = append(attendees, &calendar.EventAttendee{Email: email})
	}

	// Agregar el correo del organizador
	organizerEmail := os.Getenv("ORGANIZER_EMAIL")
	if organizerEmail != "" {
		attendees = append(attendees, &calendar.EventAttendee{Email: organizerEmail})
	}

	// Crear el evento con conferencia de Meet (sin descripción aún)
	requestID := uuid.New().String()

	event := &calendar.Event{
		Summary: req.Title,
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		Attendees: attendees,
		ConferenceData: &calendar.ConferenceData{
			CreateRequest: &calendar.CreateConferenceRequest{
				RequestId: requestID,
				ConferenceSolutionKey: &calendar.ConferenceSolutionKey{
					Type: "hangoutsMeet",
				},
			},
		},
	}

	// Insertar el evento con soporte para conferencias
	createdEvent, err := srv.Events.Insert("primary", event).
		ConferenceDataVersion(1).
		SendUpdates("all").
		Do()
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el evento: %w", err)
	}

	// Extraer el link de Meet
	meetLink := ""
	if createdEvent.ConferenceData != nil {
		meetLink = createdEvent.ConferenceData.EntryPoints[0].Uri
	}
	if meetLink == "" && createdEvent.HangoutLink != "" {
		meetLink = createdEvent.HangoutLink
	}

	// Construir el HTML final con los links reales y actualizar la descripción del evento
	htmlDescription := buildEmailHTML(req, startTime, endTime, meetLink, createdEvent.HtmlLink)
	patch := &calendar.Event{Description: htmlDescription}
	_, err = srv.Events.Patch("primary", createdEvent.Id, patch).
		SendUpdates("none").
		Do()
	if err != nil {
		// No es un error fatal: el evento ya existe, solo falla la descripción HTML
		fmt.Printf("advertencia: no se pudo actualizar la descripción HTML del evento: %v\n", err)
	}

	return &models.SessionResponse{
		EventID:  createdEvent.Id,
		MeetLink: meetLink,
		HtmlLink: createdEvent.HtmlLink,
		Status:   createdEvent.Status,
	}, nil
}

// GetAvailableSlots consulta Google Calendar FreeBusy y devuelve los slots disponibles
// para un mes completo, entre las 8 AM y 6 PM de lunes a viernes, en bloques de 1 hora.
func (s *Service) GetAvailableSlots(ctx context.Context, client *http.Client, month string, timezone string) (*models.AvailabilityResponse, error) {
	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el servicio de Calendar: %w", err)
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("zona horaria inválida '%s': %w", timezone, err)
	}

	monthStart, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil {
		return nil, fmt.Errorf("formato de mes inválido (use YYYY-MM): %w", err)
	}

	monthEnd := monthStart.AddDate(0, 1, 0)

	freeBusyReq := &calendar.FreeBusyRequest{
		TimeMin: monthStart.Format(time.RFC3339),
		TimeMax: monthEnd.Format(time.RFC3339),
		Items: []*calendar.FreeBusyRequestItem{
			{Id: "primary"},
		},
	}

	freeBusyResp, err := srv.Freebusy.Query(freeBusyReq).Do()
	if err != nil {
		return nil, fmt.Errorf("error al consultar disponibilidad: %w", err)
	}

	var busyPeriods []models.TimeSlot
	if cal, ok := freeBusyResp.Calendars["primary"]; ok {
		for _, busy := range cal.Busy {
			busyPeriods = append(busyPeriods, models.TimeSlot{
				Start: busy.Start,
				End:   busy.End,
			})
		}
	}

	now := time.Now().In(loc)
	slotDuration := 1 * time.Hour
	var days []models.DayAvailability

	for day := monthStart; day.Before(monthEnd); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}

		workStart := day.Add(8 * time.Hour)
		workEnd := day.Add(18 * time.Hour)

		var availableSlots []models.TimeSlot

		for slotStart := workStart; slotStart.Add(slotDuration).Before(workEnd) || slotStart.Add(slotDuration).Equal(workEnd); slotStart = slotStart.Add(slotDuration) {
			slotEnd := slotStart.Add(slotDuration)

			if slotStart.Before(now) {
				continue
			}

			if !overlapsAny(slotStart, slotEnd, busyPeriods) {
				availableSlots = append(availableSlots, models.TimeSlot{
					Start: slotStart.Format(time.RFC3339),
					End:   slotEnd.Format(time.RFC3339),
				})
			}
		}

		if len(availableSlots) > 0 {
			days = append(days, models.DayAvailability{
				Date:           day.Format("2006-01-02"),
				AvailableSlots: availableSlots,
			})
		}
	}

	return &models.AvailabilityResponse{
		Month:    month,
		Timezone: timezone,
		Days:     days,
	}, nil
}

// IsSlotAvailable verifica si un rango de tiempo específico está disponible.
func (s *Service) IsSlotAvailable(ctx context.Context, client *http.Client, startTime, endTime time.Time) (bool, error) {
	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return false, fmt.Errorf("no se pudo crear el servicio de Calendar: %w", err)
	}

	freeBusyReq := &calendar.FreeBusyRequest{
		TimeMin: startTime.Format(time.RFC3339),
		TimeMax: endTime.Format(time.RFC3339),
		Items: []*calendar.FreeBusyRequestItem{
			{Id: "primary"},
		},
	}

	freeBusyResp, err := srv.Freebusy.Query(freeBusyReq).Do()
	if err != nil {
		return false, fmt.Errorf("error al verificar disponibilidad: %w", err)
	}

	if cal, ok := freeBusyResp.Calendars["primary"]; ok {
		if len(cal.Busy) > 0 {
			return false, nil
		}
	}

	return true, nil
}

// overlapsAny verifica si un slot se solapa con algún período ocupado.
func overlapsAny(start, end time.Time, busyPeriods []models.TimeSlot) bool {
	for _, busy := range busyPeriods {
		busyStart, _ := time.Parse(time.RFC3339, busy.Start)
		busyEnd, _ := time.Parse(time.RFC3339, busy.End)

		// Hay solapamiento si el slot empieza antes de que termine el busy
		// Y el slot termina después de que empiece el busy
		if start.Before(busyEnd) && end.After(busyStart) {
			return true
		}
	}
	return false
}

// buildEmailHTML genera una descripción elegante y compatible con la sanitización de Google Calendar.
func buildEmailHTML(req models.SessionRequest, startTime, endTime time.Time, meetLink, calendarLink string) string {
	// Nombre del primer asistente (destinatario principal)
	nombreDestinatario := "Cliente"
	if len(req.Attendees) > 0 {
		nombreDestinatario = req.Attendees[0]
	}

	// Datos del organizador desde variables de entorno
	nombreOrganizador := os.Getenv("ORGANIZER_NAME")
	if nombreOrganizador == "" {
		nombreOrganizador = "Equipo LA MURALLA"
	}
	emailSoporte := os.Getenv("ORGANIZER_EMAIL")
	if emailSoporte == "" {
		emailSoporte = "soporte@cartagenacorporation.com"
	}

	// Formatear fecha y horas
	fecha := startTime.Format("02 Jan 2006")
	horaInicio := startTime.Format("15:04")
	horaFin := endTime.Format("15:04")
	zonaHoraria := startTime.Location().String()
	duracionMin := int(endTime.Sub(startTime).Minutes())

	// Nota adicional: usar req.Description si existe
	notaAdicional := req.Description
	if notaAdicional == "" {
		notaAdicional = "Por favor, únete a la sesión unos minutos antes para verificar audio y video."
	}

	html := EmailTemplate

	html = strings.ReplaceAll(html, "{{NOMBRE_DESTINATARIO}}", nombreDestinatario)
	html = strings.ReplaceAll(html, "{{TEMA_REUNION}}", req.Title)
	html = strings.ReplaceAll(html, "{{FECHA_REUNION}}", fecha)
	html = strings.ReplaceAll(html, "{{HORA_INICIO}}", horaInicio)
	html = strings.ReplaceAll(html, "{{HORA_FIN}}", horaFin)
	html = strings.ReplaceAll(html, "{{ZONA_HORARIA}}", zonaHoraria)
	html = strings.ReplaceAll(html, "{{DURACION}}", fmt.Sprintf("%d", duracionMin))
	html = strings.ReplaceAll(html, "{{LINK_GOOGLE_MEET}}", meetLink)
	html = strings.ReplaceAll(html, "{{NOTA_ADICIONAL}}", notaAdicional)

	return html
}
