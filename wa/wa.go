// Package wa implementa el objeto API base para consumir los servicios de la
// API pública de WebAbility (https://api.webability.info).
//
// El objeto API se crea con 2 parámetros: el ClientID (identificador público
// de la cuenta) y el Token (clave secreta de la cuenta), y se encarga de:
//   - Firmar cada request con HMAC-SHA256 (headers X-WA-Client, X-WA-Timestamp, X-WA-Digest)
//   - Enviar el request (GET, POST, PUT, DELETE) con los parámetros dados
//   - Decodificar la respuesta JSON estándar de la API
//
// El mensaje canónico firmado es: "{METODO}|{PATH}|{TIMESTAMP}|{CLIENTID}"
// y el digest es hex(HMAC-SHA256(token, mensaje)).
//
// El Token nunca viaja en el request: solo se usa localmente para calcular
// el digest. El único identificador que se transmite es el ClientID.
package wa

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"
)

// DefaultBaseURL es el host por defecto de la API de WebAbility.
const DefaultBaseURL = "https://api.webability.info"

// API representa la conexión autenticada a la API de WebAbility.
type API struct {
	BaseURL  string
	ClientID string
	Token    string
	Client   *http.Client
}

// New crea un objeto API con el ClientID y la clave secreta (Token) de la cuenta,
// usando el host por defecto.
func New(clientID, token string) *API {
	return NewWithURL(DefaultBaseURL, clientID, token)
}

// NewWithURL crea un objeto API con el ClientID, la clave secreta (Token) de la cuenta
// y un host alternativo (útil para entornos de prueba).
func NewWithURL(baseURL, clientID, token string) *API {
	return &API{
		BaseURL:  baseURL,
		ClientID: clientID,
		Token:    token,
		Client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// BuildMessage construye el mensaje canónico a firmar: "{METODO}|{PATH}|{TIMESTAMP}|{CLIENTID}".
// PATH debe ser la ruta del request sin query string.
func BuildMessage(method, path, timestamp, clientID string) string {
	return fmt.Sprintf("%s|%s|%s|%s", method, path, timestamp, clientID)
}

// Digest retorna hex(HMAC-SHA256(a.Token, message)).
func (a *API) Digest(message string) string {
	mac := hmac.New(sha256.New, []byte(a.Token))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// Response es la respuesta cruda de un request a la API.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Decode decodifica el cuerpo JSON de la respuesta en v.
func (r *Response) Decode(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// APIError representa un error devuelto por la API en formato {status, code, message}.
type APIError struct {
	StatusCode int    `json:"-"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wa api error %d: %s", e.Code, e.Message)
}

// Request firma y envía un request HTTP a la API.
// path debe ser la ruta absoluta (ej: "/v1/dns/zone"), sin el host y sin query string.
// body, si no es nil, se codifica como JSON y se envía como cuerpo del request.
func (a *API) Request(method, path string, body interface{}) (*Response, error) {
	var reader io.Reader
	contentType := ""
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("codificando body: %w", err)
		}
		reader = bytes.NewReader(data)
		contentType = "application/json"
	}
	return a.do(method, path, reader, contentType)
}

// RequestRaw firma y envía un request HTTP con un cuerpo ya preparado (sin
// codificar a JSON) y un Content-Type explícito. Útil para uploads (multipart)
// o cualquier payload que no sea JSON.
func (a *API) RequestRaw(method, path string, body io.Reader, contentType string) (*Response, error) {
	return a.do(method, path, body, contentType)
}

// do firma y envía el request HTTP subyacente, y arma la Response.
func (a *API) do(method, path string, body io.Reader, contentType string) (*Response, error) {
	req, err := http.NewRequest(method, a.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("creando request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	message := BuildMessage(method, path, timestamp, a.ClientID)
	req.Header.Set("X-WA-Client", a.ClientID)
	req.Header.Set("X-WA-Timestamp", timestamp)
	req.Header.Set("X-WA-Digest", a.Digest(message))

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("enviando request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("leyendo respuesta: %w", err)
	}

	result := &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data}

	if resp.StatusCode >= 400 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		if jsonErr := json.Unmarshal(data, apiErr); jsonErr == nil && apiErr.Message != "" {
			return result, apiErr
		}
		return result, fmt.Errorf("error HTTP %d", resp.StatusCode)
	}

	return result, nil
}

// Get envía un GET a path.
func (a *API) Get(path string) (*Response, error) {
	return a.Request(http.MethodGet, path, nil)
}

// Post envía un POST a path con body codificado en JSON.
func (a *API) Post(path string, body interface{}) (*Response, error) {
	return a.Request(http.MethodPost, path, body)
}

// Put envía un PUT a path con body codificado en JSON.
func (a *API) Put(path string, body interface{}) (*Response, error) {
	return a.Request(http.MethodPut, path, body)
}

// Delete envía un DELETE a path.
func (a *API) Delete(path string) (*Response, error) {
	return a.Request(http.MethodDelete, path, nil)
}

// PostMultipart envía un POST multipart/form-data con un único archivo en el
// campo fieldName (ej: "file"), tal como lo espera /v1/image.
func (a *API) PostMultipart(path, fieldName, filename string, file io.Reader) (*Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		return nil, fmt.Errorf("creando form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copiando archivo: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("cerrando multipart: %w", err)
	}

	return a.RequestRaw(http.MethodPost, path, &buf, w.FormDataContentType())
}
