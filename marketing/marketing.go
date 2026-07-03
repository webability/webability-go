// Package marketing implementa el objeto Marketing, que enlaza un wa.API ya
// autenticado y expone las funciones del servicio de email marketing masivo
// (listas, contactos, segmentos y campañas) de la API de WebAbility.
//
// Este es un producto distinto al correo transaccional (ver paquete mail):
// marketing agrupa envíos masivos a listas/segmentos con plantillas,
// programación y métricas por campaña.
//
// ADVERTENCIA: el servidor aún NO implementa /v1/marketing — este paquete
// sigue la especificación publicada en /documentacion/mailing (sección
// Listas/Campañas) como borrador adelantado. Esa documentación usa el prefijo
// genérico "/mail/..."; aquí se usa "/v1/marketing/..." para no chocar con el
// /v1/mail transaccional ya implementado. Revisar contra
// sites/api.webability.info/v1/marketing cuando exista antes de usarlo en producción.
package marketing

import (
	"fmt"
	"net/http"

	"github.com/webability/webability-go/wa"
)

// Marketing enlaza un objeto wa.API para hacer las llamadas al servicio de marketing.
type Marketing struct {
	API *wa.API
}

// New crea un objeto Marketing a partir de un wa.API ya autenticado.
func New(api *wa.API) *Marketing {
	return &Marketing{API: api}
}

// ── Listas ──────────────────────────────────────────────────────────────

// List representa una lista de contactos.
type List struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ListListsResult es la respuesta de ListLists.
type ListListsResult struct {
	Status string `json:"status"`
	Lists  []List `json:"lists"`
	Count  int    `json:"count"`
}

// ListLists lista las listas de contactos. GET /v1/marketing/lists
func (m *Marketing) ListLists() (*ListListsResult, error) {
	resp, err := m.API.Get("/v1/marketing/lists")
	if err != nil {
		return nil, err
	}
	var out ListListsResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// CreateList crea una lista de contactos. POST /v1/marketing/lists
func (m *Marketing) CreateList(name, description string) (*List, error) {
	resp, err := m.API.Post("/v1/marketing/lists", map[string]string{
		"name":        name,
		"description": description,
	})
	if err != nil {
		return nil, err
	}
	var out List
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// GetList obtiene el detalle de una lista. GET /v1/marketing/lists/{id}
func (m *Marketing) GetList(id string) (*List, error) {
	resp, err := m.API.Get("/v1/marketing/lists/" + id)
	if err != nil {
		return nil, err
	}
	var out List
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// DeleteList elimina una lista. DELETE /v1/marketing/lists/{id}
func (m *Marketing) DeleteList(id string) error {
	_, err := m.API.Delete("/v1/marketing/lists/" + id)
	return err
}

// ── Contactos ───────────────────────────────────────────────────────────

// Contact representa un contacto de una o varias listas.
type Contact struct {
	Email     string                 `json:"email"`
	FirstName string                 `json:"first_name,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// CreateContact agrega un contacto. POST /v1/marketing/contacts
func (m *Marketing) CreateContact(c Contact) (*Contact, error) {
	resp, err := m.API.Post("/v1/marketing/contacts", c)
	if err != nil {
		return nil, err
	}
	var out Contact
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// AddContactToList agrega un contacto existente a una lista.
// POST /v1/marketing/lists/{id}/contacts
func (m *Marketing) AddContactToList(listID, email string) error {
	_, err := m.API.Post("/v1/marketing/lists/"+listID+"/contacts", map[string]string{"email": email})
	return err
}

// RemoveContactFromList quita un contacto de una lista.
// DELETE /v1/marketing/lists/{id}/contacts
func (m *Marketing) RemoveContactFromList(listID, email string) error {
	_, err := m.API.Request(http.MethodDelete, "/v1/marketing/lists/"+listID+"/contacts", map[string]string{"email": email})
	return err
}

// ── Segmentos ───────────────────────────────────────────────────────────

// SegmentFilter es una condición de filtrado para un segmento (ej. tag, campo, actividad).
type SegmentFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Segment representa un filtro dinámico de contactos.
type Segment struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Filters []SegmentFilter `json:"filters"`
}

// CreateSegment crea un segmento con filtros. POST /v1/marketing/segments
func (m *Marketing) CreateSegment(s Segment) (*Segment, error) {
	resp, err := m.API.Post("/v1/marketing/segments", s)
	if err != nil {
		return nil, err
	}
	var out Segment
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// ── Campañas ────────────────────────────────────────────────────────────

// Campaign representa el contenido y la audiencia de una campaña.
type Campaign struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FromName  string `json:"from_name"`
	FromEmail string `json:"from_email"`
	Subject   string `json:"subject"`
	ListID    string `json:"list_id,omitempty"`
	SegmentID string `json:"segment_id,omitempty"`
	HTML      string `json:"html,omitempty"`
	Text      string `json:"text,omitempty"`
	Status    string `json:"status,omitempty"`
}

// CreateCampaign crea una campaña. POST /v1/marketing/campaigns
func (m *Marketing) CreateCampaign(c Campaign) (*Campaign, error) {
	resp, err := m.API.Post("/v1/marketing/campaigns", c)
	if err != nil {
		return nil, err
	}
	var out Campaign
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// GetCampaign obtiene el detalle de una campaña. GET /v1/marketing/campaigns/{id}
func (m *Marketing) GetCampaign(id string) (*Campaign, error) {
	resp, err := m.API.Get("/v1/marketing/campaigns/" + id)
	if err != nil {
		return nil, err
	}
	var out Campaign
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// DeleteCampaign elimina una campaña. DELETE /v1/marketing/campaigns/{id}
func (m *Marketing) DeleteCampaign(id string) error {
	_, err := m.API.Delete("/v1/marketing/campaigns/" + id)
	return err
}

// SendCampaign envía la campaña de inmediato, o la programa si scheduleAt no
// está vacío (RFC3339). POST /v1/marketing/campaigns/{id}/send
func (m *Marketing) SendCampaign(id string, scheduleAt string) error {
	var scheduleField interface{}
	if scheduleAt != "" {
		scheduleField = scheduleAt
	}
	_, err := m.API.Post("/v1/marketing/campaigns/"+id+"/send", map[string]interface{}{
		"schedule_at": scheduleField,
	})
	return err
}

// CampaignStats son las métricas de una campaña.
type CampaignStats struct {
	Status       string `json:"status"`
	Delivered    int    `json:"delivered"`
	Opens        int    `json:"opens"`
	Clicks       int    `json:"clicks"`
	Bounces      int    `json:"bounces"`
	Unsubscribes int    `json:"unsubscribes"`
}

// GetCampaignStats consulta las métricas de una campaña. GET /v1/marketing/campaigns/{id}/stats
func (m *Marketing) GetCampaignStats(id string) (*CampaignStats, error) {
	resp, err := m.API.Get("/v1/marketing/campaigns/" + id + "/stats")
	if err != nil {
		return nil, err
	}
	var out CampaignStats
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}
