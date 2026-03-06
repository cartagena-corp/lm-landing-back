package models

// SessionRequest representa la solicitud del frontend para crear una sesión de Meet.
type SessionRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	Attendees   []string `json:"attendees"`
}

// SessionResponse representa la respuesta con los datos del evento creado.
type SessionResponse struct {
	EventID  string `json:"event_id"`
	MeetLink string `json:"meet_link"`
	HtmlLink string `json:"html_link"`
	Status   string `json:"status"`
}

// ErrorResponse representa una respuesta de error de la API.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// TimeSlot representa un bloque de tiempo disponible.
type TimeSlot struct {
	Start string `json:"start"` // RFC3339
	End   string `json:"end"`   // RFC3339
}

// DayAvailability representa los horarios disponibles de un día específico.
type DayAvailability struct {
	Date           string     `json:"date"`
	AvailableSlots []TimeSlot `json:"available_slots"`
}

// AvailabilityResponse representa los horarios disponibles de un mes.
type AvailabilityResponse struct {
	Month    string            `json:"month"`
	Timezone string            `json:"timezone"`
	Days     []DayAvailability `json:"days"`
}
