// Package mail implementa el objeto Mail, que enlaza un wa.API ya autenticado
// y expone las funciones del servicio de correo transaccional de la API cliente
// de WebAbility (https://api.webability.info/v1/mail).
//
// Para envíos masivos de campañas (listas, segmentos, plantillas), ver el
// paquete marketing en su lugar — son productos distintos.
package mail

import (
	"fmt"

	"github.com/webability/webability-go/wa"
)

// Mail enlaza un objeto wa.API para hacer las llamadas al servicio de correo.
type Mail struct {
	API *wa.API
}

// New crea un objeto Mail a partir de un wa.API ya autenticado.
func New(api *wa.API) *Mail {
	return &Mail{API: api}
}

// Address es un remitente o destinatario simple.
type Address struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// Recipient es un destinatario con variables de personalización propias.
type Recipient struct {
	Email string                 `json:"email"`
	Name  string                 `json:"name,omitempty"`
	Vars  map[string]interface{} `json:"vars,omitempty"`
}

// SendRequest son los campos para POST /v1/mail/send.
type SendRequest struct {
	From Address   `json:"from"`
	To   Recipient `json:"to"`
	// Template, si viene, es el id de una plantilla ya registrada y activa del
	// lado de WebAbility (bajo la cuenta que autentica este request) — el
	// servidor arma el correo con esa plantilla en vez de Subject/HTML/Text
	// (que se ignoran si Template viene). La personalización se hace con las
	// Vars de To, sin ningún prefijo en los nombres — dentro del contenido de
	// la plantilla (definido en Consola → Correos → Plantillas) se acceden
	// como {{vars>clave}}, no {{clave}} a secas (eso último solo aplica al
	// envío ad-hoc, sin Template). El servidor valida que la plantilla exista
	// y esté activa ANTES de encolar el correo — si no, Send devuelve un
	// *wa.APIError síncrono (no un envío "pending" fallido).
	Template    string   `json:"template,omitempty"`
	Subject     string   `json:"subject"`
	HTML        string   `json:"html,omitempty"`
	Text        string   `json:"text,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	TrackOpens  bool     `json:"track_opens,omitempty"`
	TrackClicks bool     `json:"track_clicks,omitempty"`
	// WaitSend, si es true, espera (hasta ~20s del lado del servidor) el
	// resultado real del envío antes de responder, en vez de responder de
	// inmediato con QueueStatus="pending". Si el envío no se resuelve dentro
	// de ese tiempo, la respuesta degrada a QueueStatus="pending" de todas
	// formas — usa Status(queueKey) para consultarlo después.
	WaitSend bool `json:"wait_send,omitempty"`
}

// Estados posibles de QueueStatus en SendResult y Status().
const (
	QueueStatusPending    = "pending"
	QueueStatusProcessing = "processing"
	QueueStatusSent       = "sent"
	QueueStatusError      = "error"
)

// SendResult es la respuesta de Send.
type SendResult struct {
	Status      string `json:"status"`
	QueueKey    int    `json:"queue_key"`
	QueueStatus string `json:"queue_status"`
	ErrorDetail string `json:"error_detail,omitempty"`
	To          string `json:"to"`
}

// Send envía un correo a un solo destinatario. POST /v1/mail/send
//
// Sin WaitSend, QueueStatus siempre viene "pending" — el envío real ocurre
// asíncronamente; usa Status(out.QueueKey) para conocer el resultado. Con
// WaitSend=true, QueueStatus puede venir ya resuelto ("sent"/"error") si el
// servidor lo confirmó a tiempo.
func (m *Mail) Send(req SendRequest) (*SendResult, error) {
	resp, err := m.API.Post("/v1/mail/send", req)
	if err != nil {
		return nil, err
	}
	var out SendResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// StatusResult es la respuesta de Status.
type StatusResult struct {
	Status      string `json:"status"`
	QueueKey    int    `json:"queue_key"`
	QueueStatus string `json:"queue_status"`
	ErrorDetail string `json:"error_detail,omitempty"`
}

// Status consulta el estatus real de un envío hecho con Send.
// GET /v1/mail/status/{queue_key}
func (m *Mail) Status(queueKey int) (*StatusResult, error) {
	resp, err := m.API.Get(fmt.Sprintf("/v1/mail/status/%d", queueKey))
	if err != nil {
		return nil, err
	}
	var out StatusResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// SendBulkRequest son los campos para POST /v1/mail/send-bulk.
type SendBulkRequest struct {
	From        Address     `json:"from"`
	Subject     string      `json:"subject"`
	PreviewText string      `json:"preview_text,omitempty"`
	HTML        string      `json:"html,omitempty"`
	Text        string      `json:"text,omitempty"`
	Recipients  []Recipient `json:"recipients"`
	Tags        []string    `json:"tags,omitempty"`
	TrackOpens  bool        `json:"track_opens,omitempty"`
	TrackClicks bool        `json:"track_clicks,omitempty"`
}

// SendBulkResultEntry es el resultado de un destinatario dentro de SendBulkResult.
type SendBulkResultEntry struct {
	Email    string `json:"email"`
	QueueKey int    `json:"queue_key,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// SendBulkResult es la respuesta de SendBulk.
type SendBulkResult struct {
	Status  string                `json:"status"`
	Total   int                   `json:"total"`
	Queued  int                   `json:"queued"`
	Failed  int                   `json:"failed"`
	Results []SendBulkResultEntry `json:"results"`
}

// SendBulk envía el mismo correo (con variables por destinatario) a múltiples
// destinatarios. POST /v1/mail/send-bulk
func (m *Mail) SendBulk(req SendBulkRequest) (*SendBulkResult, error) {
	resp, err := m.API.Post("/v1/mail/send-bulk", req)
	if err != nil {
		return nil, err
	}
	var out SendBulkResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}
