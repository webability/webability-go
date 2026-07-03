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
	From        Address   `json:"from"`
	To          Recipient `json:"to"`
	Subject     string    `json:"subject"`
	HTML        string    `json:"html,omitempty"`
	Text        string    `json:"text,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	TrackOpens  bool      `json:"track_opens,omitempty"`
	TrackClicks bool      `json:"track_clicks,omitempty"`
}

// SendResult es la respuesta de Send.
type SendResult struct {
	Status   string `json:"status"`
	QueueKey int    `json:"queue_key"`
	To       string `json:"to"`
}

// Send envía un correo a un solo destinatario. POST /v1/mail/send
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
