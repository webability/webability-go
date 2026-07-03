// Package dns implementa el objeto DNS, que enlaza un wa.API ya autenticado
// y expone las funciones del servicio DNS de la API cliente de WebAbility
// (https://api.webability.info/v1/dns).
package dns

import (
	"fmt"
	"net/url"

	"github.com/webability/webability-go/wa"
)

// DNS enlaza un objeto wa.API para hacer las llamadas al servicio DNS de la API.
type DNS struct {
	API *wa.API
}

// New crea un objeto DNS a partir de un wa.API ya autenticado.
func New(api *wa.API) *DNS {
	return &DNS{API: api}
}

// Zone representa una zona DNS del cliente.
type Zone struct {
	Key          int    `json:"key"`
	Name         string `json:"name"`
	Status       int    `json:"status"`
	PrimaryNS    string `json:"primaryns"`
	AdminEmail   string `json:"adminemail"`
	Serial       int    `json:"serial"`
	Refresh      int    `json:"refresh"`
	Retry        int    `json:"retry"`
	Expire       int    `json:"expire"`
	Minimum      int    `json:"minimum"`
	DefaultTTL   int    `json:"defaultttl"`
	DNSSEC       int    `json:"dnssec"`
	CreationDate string `json:"creationdate"`
}

// Record representa un registro DNS de una zona del cliente.
type Record struct {
	Key        int    `json:"key"`
	Zone       int    `json:"zone"`
	Name       string `json:"name"`
	RRType     int    `json:"rrtype"`
	RRTypeName string `json:"rrtypename"`
	TTL        int    `json:"ttl"`
	Status     int    `json:"status"`
	Priority   int    `json:"priority"`
	Weight     int    `json:"weight"`
	Port       int    `json:"port"`
	Tag        string `json:"tag"`
	Data       string `json:"data"`
}

// ListZonesResult es la respuesta de ListZones.
type ListZonesResult struct {
	Status string `json:"status"`
	Zones  []Zone `json:"zones"`
	Count  int    `json:"count"`
}

// ListZones lista las zonas del cliente. GET /v1/dns/zone
func (d *DNS) ListZones() (*ListZonesResult, error) {
	resp, err := d.API.Get("/v1/dns/zone")
	if err != nil {
		return nil, err
	}
	var out ListZonesResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// GetZoneResult es la respuesta de GetZone.
type GetZoneResult struct {
	Status  string   `json:"status"`
	Zone    Zone     `json:"zone"`
	Records []Record `json:"records"`
	NS      []string `json:"ns"`
}

// GetZone obtiene una zona (por clave o por dominio) junto con sus registros. GET /v1/dns/zone/{key|domain}
func (d *DNS) GetZone(keyOrDomain string) (*GetZoneResult, error) {
	resp, err := d.API.Get("/v1/dns/zone/" + url.PathEscape(keyOrDomain))
	if err != nil {
		return nil, err
	}
	var out GetZoneResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// AddZoneResult es la respuesta de AddZone.
type AddZoneResult struct {
	Status string `json:"status"`
	Key    int    `json:"key"`
	Name   string `json:"name"`
}

// AddZone crea una nueva zona. POST /v1/dns/zone
func (d *DNS) AddZone(name string) (*AddZoneResult, error) {
	resp, err := d.API.Post("/v1/dns/zone", map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	var out AddZoneResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// RecordInput son los campos para crear un registro nuevo.
type RecordInput struct {
	Name     string `json:"name"`
	RRType   string `json:"rrtype"`
	TTL      int    `json:"ttl"`
	Data     string `json:"data"`
	Priority int    `json:"priority,omitempty"`
	Weight   int    `json:"weight,omitempty"`
	Port     int    `json:"port,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

// AddRecordResult es la respuesta de AddRecord.
type AddRecordResult struct {
	Status string `json:"status"`
	Key    int    `json:"key"`
	Zone   int    `json:"zone"`
}

// AddRecord agrega un registro a una zona. POST /v1/dns/zone/{key}/record
func (d *DNS) AddRecord(zoneKey int, rec RecordInput) (*AddRecordResult, error) {
	resp, err := d.API.Post(fmt.Sprintf("/v1/dns/zone/%d/record", zoneKey), rec)
	if err != nil {
		return nil, err
	}
	var out AddRecordResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// RecordUpdate son los campos opcionales para modificar un registro existente.
// Solo los campos no nulos se envían y se modifican.
type RecordUpdate struct {
	Name     *string `json:"name,omitempty"`
	TTL      *int    `json:"ttl,omitempty"`
	Data     *string `json:"data,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	Weight   *int    `json:"weight,omitempty"`
	Port     *int    `json:"port,omitempty"`
	Tag      *string `json:"tag,omitempty"`
	Status   *int    `json:"status,omitempty"`
}

// UpdateRecord modifica un registro existente. PUT /v1/dns/record/{key}
func (d *DNS) UpdateRecord(recordKey int, fields RecordUpdate) error {
	_, err := d.API.Put(fmt.Sprintf("/v1/dns/record/%d", recordKey), fields)
	return err
}

// DeleteRecord elimina un registro. DELETE /v1/dns/record/{key}
func (d *DNS) DeleteRecord(recordKey int) error {
	_, err := d.API.Delete(fmt.Sprintf("/v1/dns/record/%d", recordKey))
	return err
}

// DeleteZone elimina una zona y sus registros. DELETE /v1/dns/zone/{key}
func (d *DNS) DeleteZone(zoneKey int) error {
	_, err := d.API.Delete(fmt.Sprintf("/v1/dns/zone/%d", zoneKey))
	return err
}
