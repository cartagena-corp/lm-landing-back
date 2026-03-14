package calendar

import _ "embed"

// EmailTemplate contiene el HTML del correo de invitación,
// incrustado directamente en el binario durante la compilación.
//
//go:embed template.html
var EmailTemplate string
