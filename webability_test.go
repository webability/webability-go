// Test de aceptación de la librería cliente github.com/webability/webability-go.
//
// Organización: un subtest por producto (wa, dns, image, mail, video,
// marketing), y dentro de cada uno un subtest por entrada de la API (cada
// función pública del paquete). Cada subtest levanta un servidor HTTP de
// prueba que:
//  1. Verifica la firma HMAC-SHA256 (X-WA-Client/X-WA-Timestamp/X-WA-Digest)
//     de forma independiente al código del cliente (recalculando el digest
//     "a mano" con crypto/hmac), para no probar la librería contra sí misma.
//  2. Verifica método HTTP, path y body del request saliente.
//  3. Responde con el JSON que la API real (o, para video/marketing, la
//     especificación publicada) devolvería, y se valida que el cliente lo
//     decodifique correctamente.
//
// video y marketing todavía no tienen API real (ver advertencias en sus
// paquetes); estos tests fijan el contrato *tal como está definido hoy en el
// cliente*, para detectar cambios accidentales de forma temprana.
package api_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/webability/webability-go/dns"
	"github.com/webability/webability-go/image"
	"github.com/webability/webability-go/mail"
	"github.com/webability/webability-go/marketing"
	"github.com/webability/webability-go/video"
	"github.com/webability/webability-go/wa"
)

const (
	testClientID = "cli_test123"
	testSecret   = "s3cr3t-de-prueba-para-tests"
)

// ─────────────────────────────────────────────────────────────────────────
// Helpers de prueba
// ─────────────────────────────────────────────────────────────────────────

// signHMAC recalcula el digest de forma independiente al cliente, siguiendo
// el esquema documentado: hex(HMAC-SHA256(secret, "{METHOD}|{PATH}|{TIMESTAMP}|{CLIENTID}")).
func signHMAC(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// withAuthCheck envuelve un mux verificando, para cada request entrante, que
// los 3 headers de autenticación estén presentes, que el timestamp sea
// reciente y que el digest coincida con el HMAC recalculado de forma
// independiente. No usa t.Fatal (se ejecuta en la goroutine del servidor de
// prueba, no en la del test) para no arriesgar una parada insegura.
func withAuthCheck(t *testing.T, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("X-WA-Client")
		timestamp := r.Header.Get("X-WA-Timestamp")
		digest := r.Header.Get("X-WA-Digest")

		if clientID != testClientID {
			t.Errorf("X-WA-Client = %q, want %q", clientID, testClientID)
		}
		if timestamp == "" {
			t.Errorf("falta el header X-WA-Timestamp")
		}
		if digest == "" {
			t.Errorf("falta el header X-WA-Digest")
		}
		// El Token (secreto) nunca debe viajar en el request.
		if r.Header.Get("X-WA-Token") != "" {
			t.Errorf("X-WA-Token no debería enviarse (el secreto nunca viaja en el request)")
		}

		if ts, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
			t.Errorf("X-WA-Timestamp no es un unix timestamp válido: %q", timestamp)
		} else if diff := time.Now().Unix() - ts; diff > 5 || diff < -5 {
			t.Errorf("X-WA-Timestamp no es reciente (diferencia: %ds)", diff)
		}

		message := wa.BuildMessage(r.Method, r.URL.Path, timestamp, clientID)
		want := signHMAC(testSecret, message)
		if !hmac.Equal([]byte(want), []byte(digest)) {
			t.Errorf("X-WA-Digest inválido para %s %s: got %s, want %s", r.Method, r.URL.Path, digest, want)
		}

		next.ServeHTTP(w, r)
	})
}

// newTestAPI levanta un servidor de prueba a partir de mux (con verificación
// de autenticación de por medio) y devuelve un wa.API apuntando a él.
func newTestAPI(t *testing.T, mux *http.ServeMux) *wa.API {
	t.Helper()
	srv := httptest.NewServer(withAuthCheck(t, mux))
	t.Cleanup(srv.Close)
	return wa.NewWithURL(srv.URL, testClientID, testSecret)
}

// jsonHandler responde con status y body codificado en JSON.
func jsonHandler(t *testing.T, status int, body interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("codificando respuesta mock: %v", err)
		}
	}
}

// decodeBody decodifica el body JSON del request entrante en v.
func decodeBody(t *testing.T, r *http.Request, v interface{}) {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Errorf("decodificando body del request: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// wa — firma y transporte (independiente de cualquier producto)
// ─────────────────────────────────────────────────────────────────────────

func TestWA(t *testing.T) {
	t.Run("BuildMessage y Digest siguen el esquema documentado", func(t *testing.T) {
		msg := wa.BuildMessage("POST", "/v1/dns/zone", "1718000000", "cli_abc")
		wantMsg := "POST|/v1/dns/zone|1718000000|cli_abc"
		if msg != wantMsg {
			t.Fatalf("BuildMessage = %q, want %q", msg, wantMsg)
		}

		api := wa.New(testClientID, testSecret)
		digest := api.Digest(msg)
		wantDigest := signHMAC(testSecret, msg)
		if digest != wantDigest {
			t.Fatalf("Digest = %q, want %q", digest, wantDigest)
		}
	})

	t.Run("Request firma correctamente un GET real", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/ping", jsonHandler(t, http.StatusOK, map[string]string{"status": "ok"}))
		api := newTestAPI(t, mux)

		resp, err := api.Get("/v1/ping")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("errores 4xx se decodifican como APIError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone", jsonHandler(t, http.StatusUnauthorized, map[string]interface{}{
			"status":  "error",
			"code":    4003,
			"message": "Cliente no encontrado",
		}))
		api := newTestAPI(t, mux)

		_, err := api.Get("/v1/dns/zone")
		if err == nil {
			t.Fatal("se esperaba error")
		}
		apiErr, ok := err.(*wa.APIError)
		if !ok {
			t.Fatalf("error = %T, want *wa.APIError", err)
		}
		if apiErr.Code != 4003 || apiErr.StatusCode != http.StatusUnauthorized || apiErr.Message != "Cliente no encontrado" {
			t.Fatalf("APIError = %+v, want code=4003 status=401 message=\"Cliente no encontrado\"", apiErr)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// dns — API real /v1/dns
// ─────────────────────────────────────────────────────────────────────────

func TestDNS(t *testing.T) {
	t.Run("ListZones", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok",
				"count":  1,
				"zones": []map[string]interface{}{
					{"key": 42, "name": "midominio.com", "status": 1, "primaryns": "ns1.webability.info"},
				},
			})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))

		out, err := d.ListZones()
		if err != nil {
			t.Fatalf("ListZones: %v", err)
		}
		if out.Count != 1 || len(out.Zones) != 1 || out.Zones[0].Name != "midominio.com" {
			t.Fatalf("ListZones result = %+v", out)
		}
	})

	t.Run("GetZone por clave numérica", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone/42", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok",
				"zone":   map[string]interface{}{"key": 42, "name": "midominio.com"},
				"records": []map[string]interface{}{
					{"key": 101, "zone": 42, "name": "@", "rrtype": 1, "rrtypename": "A", "data": "1.2.3.4"},
				},
				"ns": []string{"ns1.webability.info", "ns2.webability.info"},
			})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))

		out, err := d.GetZone("42")
		if err != nil {
			t.Fatalf("GetZone: %v", err)
		}
		if out.Zone.Key != 42 || len(out.Records) != 1 || out.Records[0].RRTypeName != "A" || len(out.NS) != 2 {
			t.Fatalf("GetZone result = %+v", out)
		}
	})

	t.Run("GetZone por nombre de dominio", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone/midominio.com", jsonHandler(t, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"zone":   map[string]interface{}{"key": 42, "name": "midominio.com"},
		}))
		d := dns.New(newTestAPI(t, mux))

		out, err := d.GetZone("midominio.com")
		if err != nil {
			t.Fatalf("GetZone: %v", err)
		}
		if out.Zone.Name != "midominio.com" {
			t.Fatalf("GetZone result = %+v", out)
		}
	})

	t.Run("AddZone", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			var body map[string]string
			decodeBody(t, r, &body)
			if body["name"] != "midominio.com" {
				t.Errorf("body = %+v, want name=midominio.com", body)
			}
			jsonHandler(t, http.StatusCreated, map[string]interface{}{
				"status": "ok", "key": 42, "name": "midominio.com",
			})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))

		out, err := d.AddZone("midominio.com")
		if err != nil {
			t.Fatalf("AddZone: %v", err)
		}
		if out.Key != 42 || out.Name != "midominio.com" {
			t.Fatalf("AddZone result = %+v", out)
		}
	})

	t.Run("AddRecord", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone/42/record", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			var body dns.RecordInput
			decodeBody(t, r, &body)
			if body.Name != "@" || body.RRType != "A" || body.Data != "1.2.3.4" {
				t.Errorf("body = %+v", body)
			}
			jsonHandler(t, http.StatusCreated, map[string]interface{}{
				"status": "ok", "key": 101, "zone": 42,
			})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))

		out, err := d.AddRecord(42, dns.RecordInput{Name: "@", RRType: "A", TTL: 1800, Data: "1.2.3.4"})
		if err != nil {
			t.Fatalf("AddRecord: %v", err)
		}
		if out.Key != 101 || out.Zone != 42 {
			t.Fatalf("AddRecord result = %+v", out)
		}
	})

	t.Run("UpdateRecord solo serializa los campos seteados", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/record/101", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("method = %s, want PUT", r.Method)
			}
			raw, _ := io.ReadAll(r.Body)
			var body map[string]interface{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("decodificando body: %v", err)
			}
			if _, ok := body["ttl"]; !ok {
				t.Errorf("body debería incluir 'ttl': %s", raw)
			}
			if _, ok := body["data"]; ok {
				t.Errorf("body no debería incluir 'data' (campo no seteado): %s", raw)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok", "key": 101})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))

		ttl := 3600
		if err := d.UpdateRecord(101, dns.RecordUpdate{TTL: &ttl}); err != nil {
			t.Fatalf("UpdateRecord: %v", err)
		}
	})

	t.Run("DeleteRecord", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/record/101", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok", "key": 101})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))
		if err := d.DeleteRecord(101); err != nil {
			t.Fatalf("DeleteRecord: %v", err)
		}
	})

	t.Run("DeleteZone", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/dns/zone/42", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok", "key": 42, "name": "midominio.com"})(w, r)
		})
		d := dns.New(newTestAPI(t, mux))
		if err := d.DeleteZone(42); err != nil {
			t.Fatalf("DeleteZone: %v", err)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// image — API real /v1/image
// ─────────────────────────────────────────────────────────────────────────

func TestImage(t *testing.T) {
	t.Run("Get devuelve el binario y el Content-Type", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/image/productos/800x600/zapatilla.jpg.webp", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			w.Header().Set("Content-Type", "image/webp")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("fake-webp-bytes"))
		})
		img := image.New(newTestAPI(t, mux))

		resp, err := img.Get("productos/800x600/zapatilla.jpg.webp")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if resp.Header.Get("Content-Type") != "image/webp" {
			t.Fatalf("Content-Type = %q, want image/webp", resp.Header.Get("Content-Type"))
		}
		if string(resp.Body) != "fake-webp-bytes" {
			t.Fatalf("Body = %q", resp.Body)
		}
	})

	t.Run("Upload envía multipart/form-data con el campo file", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/image/productos/zapatilla.jpg", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("FormFile: %v", err)
			}
			defer file.Close()
			content, _ := io.ReadAll(file)
			if string(content) != "contenido-de-prueba" {
				t.Errorf("contenido subido = %q", content)
			}
			if header.Filename != "zapatilla.jpg" {
				t.Errorf("filename = %q, want zapatilla.jpg", header.Filename)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "path": "productos/zapatilla.jpg", "filename": "zapatilla.jpg",
				"size": len(content), "cache_purged": 3,
			})(w, r)
		})
		img := image.New(newTestAPI(t, mux))

		out, err := img.Upload("productos/zapatilla.jpg", "zapatilla.jpg", strings.NewReader("contenido-de-prueba"))
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if out.CachePurged != 3 || out.Filename != "zapatilla.jpg" {
			t.Fatalf("Upload result = %+v", out)
		}
	})

	t.Run("PurgeCache", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/image", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "deleted_firstcache": 0, "deleted_secondcache": 12, "total": 12,
			})(w, r)
		})
		img := image.New(newTestAPI(t, mux))

		out, err := img.PurgeCache()
		if err != nil {
			t.Fatalf("PurgeCache: %v", err)
		}
		if out.Total != 12 || out.DeletedSecondcache != 12 {
			t.Fatalf("PurgeCache result = %+v", out)
		}
	})

	t.Run("Delete imagen específica", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/image/productos/zapatilla.jpg", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("method = %s, want DELETE", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "deleted_firstcache": 1, "deleted_secondcache": 8, "total": 9,
			})(w, r)
		})
		img := image.New(newTestAPI(t, mux))

		out, err := img.Delete("productos/zapatilla.jpg")
		if err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if out.Total != 9 {
			t.Fatalf("Delete result = %+v", out)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// mail — API real /v1/mail (transaccional)
// ─────────────────────────────────────────────────────────────────────────

func TestMail(t *testing.T) {
	t.Run("Send", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			var body mail.SendRequest
			decodeBody(t, r, &body)
			if body.From.Email != "no-reply@tuempresa.com" || body.To.Email != "cliente@ejemplo.com" {
				t.Errorf("body = %+v", body)
			}
			if body.To.Vars["nombre"] != "Ana" {
				t.Errorf("vars no llegaron: %+v", body.To.Vars)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "queue_key": 42, "to": body.To.Email,
			})(w, r)
		})
		m := mail.New(newTestAPI(t, mux))

		out, err := m.Send(mail.SendRequest{
			From:    mail.Address{Email: "no-reply@tuempresa.com", Name: "Tu Empresa"},
			To:      mail.Recipient{Email: "cliente@ejemplo.com", Name: "Ana", Vars: map[string]interface{}{"nombre": "Ana"}},
			Subject: "Confirma tu compra",
			HTML:    "<p>Hola {{nombre}}</p>",
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueKey != 42 || out.To != "cliente@ejemplo.com" {
			t.Fatalf("Send result = %+v", out)
		}
		if out.QueueStatus != "" {
			t.Fatalf("QueueStatus = %q, el mock no lo envió, debería quedar vacío", out.QueueStatus)
		}
	})

	t.Run("Send con Template envía el id y las vars, sin subject/html/text", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", func(w http.ResponseWriter, r *http.Request) {
			var body mail.SendRequest
			decodeBody(t, r, &body)
			if body.Template != "bienvenida" {
				t.Errorf("Template = %q, want %q", body.Template, "bienvenida")
			}
			if body.Subject != "" || body.HTML != "" || body.Text != "" {
				t.Errorf("Subject/HTML/Text deberían ir vacíos cuando se usa Template, body = %+v", body)
			}
			if body.To.Vars["nombre"] != "Ana" {
				t.Errorf("vars no llegaron: %+v", body.To.Vars)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "queue_key": 50, "queue_status": "pending", "to": body.To.Email,
			})(w, r)
		})
		m := mail.New(newTestAPI(t, mux))

		out, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@tuempresa.com", Name: "Tu Empresa"},
			To:       mail.Recipient{Email: "cliente@ejemplo.com", Name: "Ana", Vars: map[string]interface{}{"nombre": "Ana"}},
			Template: "bienvenida",
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueKey != 50 {
			t.Fatalf("Send result = %+v", out)
		}
	})

	t.Run("Send con Template inexistente devuelve APIError 3025", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", jsonHandler(t, http.StatusNotFound, map[string]interface{}{
			"status": "error", "code": 3025, "message": "Plantilla 'no-existe' no encontrada para esta cuenta",
		}))
		m := mail.New(newTestAPI(t, mux))

		_, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@tuempresa.com"},
			To:       mail.Recipient{Email: "cliente@ejemplo.com"},
			Template: "no-existe",
		})
		if err == nil {
			t.Fatal("se esperaba error")
		}
		apiErr, ok := err.(*wa.APIError)
		if !ok {
			t.Fatalf("error = %T, want *wa.APIError", err)
		}
		if apiErr.Code != 3025 || apiErr.StatusCode != http.StatusNotFound {
			t.Fatalf("APIError = %+v, want code=3025 status=404", apiErr)
		}
	})

	t.Run("Send con Template inactiva devuelve APIError 3026", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", jsonHandler(t, http.StatusBadRequest, map[string]interface{}{
			"status": "error", "code": 3026, "message": "Plantilla 'borrador' no está activa",
		}))
		m := mail.New(newTestAPI(t, mux))

		_, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@tuempresa.com"},
			To:       mail.Recipient{Email: "cliente@ejemplo.com"},
			Template: "borrador",
		})
		if err == nil {
			t.Fatal("se esperaba error")
		}
		apiErr, ok := err.(*wa.APIError)
		if !ok {
			t.Fatalf("error = %T, want *wa.APIError", err)
		}
		if apiErr.Code != 3026 || apiErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("APIError = %+v, want code=3026 status=400", apiErr)
		}
	})

	t.Run("Send sin WaitSend responde pending de inmediato", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", func(w http.ResponseWriter, r *http.Request) {
			var body mail.SendRequest
			decodeBody(t, r, &body)
			if body.WaitSend {
				t.Errorf("wait_send no debería viajar en true, body = %+v", body)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "queue_key": 100, "queue_status": "pending", "to": body.To.Email,
			})(w, r)
		})
		m := mail.New(newTestAPI(t, mux))

		out, err := m.Send(mail.SendRequest{
			From:    mail.Address{Email: "no-reply@tuempresa.com"},
			To:      mail.Recipient{Email: "cliente@ejemplo.com"},
			Subject: "Asunto",
			Text:    "cuerpo",
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueStatus != mail.QueueStatusPending {
			t.Fatalf("QueueStatus = %q, want %q", out.QueueStatus, mail.QueueStatusPending)
		}
	})

	t.Run("Send con WaitSend=true resuelto como sent", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", func(w http.ResponseWriter, r *http.Request) {
			var body mail.SendRequest
			decodeBody(t, r, &body)
			if !body.WaitSend {
				t.Errorf("wait_send debería viajar en true, body = %+v", body)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "queue_key": 101, "queue_status": "sent", "to": body.To.Email,
			})(w, r)
		})
		m := mail.New(newTestAPI(t, mux))

		out, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@tuempresa.com"},
			To:       mail.Recipient{Email: "cliente@ejemplo.com"},
			Subject:  "Asunto",
			Text:     "cuerpo",
			WaitSend: true,
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueStatus != mail.QueueStatusSent {
			t.Fatalf("QueueStatus = %q, want %q", out.QueueStatus, mail.QueueStatusSent)
		}
		if out.ErrorDetail != "" {
			t.Fatalf("ErrorDetail = %q, no debería venir en un envío exitoso", out.ErrorDetail)
		}
	})

	t.Run("Send con WaitSend=true resuelto como error (correo inexistente)", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send", func(w http.ResponseWriter, r *http.Request) {
			var body mail.SendRequest
			decodeBody(t, r, &body)
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "queue_key": 102, "queue_status": "error",
				"error_detail": "550 5.1.1 The email account does not exist", "to": body.To.Email,
			})(w, r)
		})
		m := mail.New(newTestAPI(t, mux))

		out, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@tuempresa.com"},
			To:       mail.Recipient{Email: "no-existe@dominio-invalido.example"},
			Subject:  "Asunto",
			Text:     "cuerpo",
			WaitSend: true,
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueStatus != mail.QueueStatusError {
			t.Fatalf("QueueStatus = %q, want %q", out.QueueStatus, mail.QueueStatusError)
		}
		if out.ErrorDetail == "" {
			t.Fatalf("ErrorDetail vacío, se esperaba el detalle del rechazo SMTP")
		}
	})

	t.Run("Status pending/processing/sent/error", func(t *testing.T) {
		cases := []struct {
			name       string
			mockStatus string
			mockDetail string
			want       string
		}{
			{"pendiente", "pending", "", mail.QueueStatusPending},
			{"procesando", "processing", "", mail.QueueStatusProcessing},
			{"enviado", "sent", "", mail.QueueStatusSent},
			{"error", "error", "550 mailbox unavailable", mail.QueueStatusError},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				mux := http.NewServeMux()
				mux.HandleFunc("/v1/mail/status/200", func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet {
						t.Errorf("method = %s, want GET", r.Method)
					}
					resp := map[string]interface{}{
						"status": "ok", "queue_key": 200, "queue_status": c.mockStatus,
					}
					if c.mockDetail != "" {
						resp["error_detail"] = c.mockDetail
					}
					jsonHandler(t, http.StatusOK, resp)(w, r)
				})
				m := mail.New(newTestAPI(t, mux))

				out, err := m.Status(200)
				if err != nil {
					t.Fatalf("Status: %v", err)
				}
				if out.QueueStatus != c.want {
					t.Fatalf("QueueStatus = %q, want %q", out.QueueStatus, c.want)
				}
				if out.ErrorDetail != c.mockDetail {
					t.Fatalf("ErrorDetail = %q, want %q", out.ErrorDetail, c.mockDetail)
				}
			})
		}
	})

	t.Run("Status de una cola inexistente devuelve APIError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/status/999999", jsonHandler(t, http.StatusNotFound, map[string]interface{}{
			"status": "error", "code": 3024, "message": "Cola no encontrada",
		}))
		m := mail.New(newTestAPI(t, mux))

		_, err := m.Status(999999)
		if err == nil {
			t.Fatal("se esperaba error")
		}
		apiErr, ok := err.(*wa.APIError)
		if !ok {
			t.Fatalf("error = %T, want *wa.APIError", err)
		}
		if apiErr.Code != 3024 || apiErr.StatusCode != http.StatusNotFound {
			t.Fatalf("APIError = %+v, want code=3024 status=404", apiErr)
		}
	})

	t.Run("SendBulk con resultados mixtos (queued y error)", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/mail/send-bulk", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			var body mail.SendBulkRequest
			decodeBody(t, r, &body)
			if len(body.Recipients) != 2 {
				t.Errorf("recipients = %+v", body.Recipients)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"status": "ok", "total": 2, "queued": 1, "failed": 1,
				"results": []map[string]interface{}{
					{"email": "ana@ejemplo.com", "queue_key": 10, "status": "queued"},
					{"email": "malformado", "status": "error", "error": "email inválido"},
				},
			})(w, r)
		})
		m := mail.New(newTestAPI(t, mux))

		out, err := m.SendBulk(mail.SendBulkRequest{
			From:    mail.Address{Email: "newsletter@webability.info", Name: "Webability"},
			Subject: "Asunto",
			Recipients: []mail.Recipient{
				{Email: "ana@ejemplo.com", Vars: map[string]interface{}{"nombre": "Ana"}},
				{Email: "malformado"},
			},
		})
		if err != nil {
			t.Fatalf("SendBulk: %v", err)
		}
		if out.Total != 2 || out.Queued != 1 || out.Failed != 1 || len(out.Results) != 2 {
			t.Fatalf("SendBulk result = %+v", out)
		}
		if out.Results[1].Status != "error" || out.Results[1].Error == "" {
			t.Fatalf("resultado con error mal parseado: %+v", out.Results[1])
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// video — borrador (sin API real todavía, ver ADVERTENCIA en el paquete)
// ─────────────────────────────────────────────────────────────────────────

func TestVideo(t *testing.T) {
	t.Run("CreateJob y GetJob", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/video/jobs", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			var body video.CreateJobRequest
			decodeBody(t, r, &body)
			if body.SourceURL != "https://tusitio.com/video.mp4" || body.Output.Type != "hls" {
				t.Errorf("body = %+v", body)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"id": "job_123", "status": "queued",
			})(w, r)
		})
		mux.HandleFunc("/v1/video/jobs/job_123", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{
				"id": "job_123", "status": "finished",
				"outputs": map[string]string{"hls_master": "https://cdn.webability.info/videos/abc123/master.m3u8"},
			})(w, r)
		})
		v := video.New(newTestAPI(t, mux))

		job, err := v.CreateJob(video.CreateJobRequest{
			SourceURL: "https://tusitio.com/video.mp4",
			Profile:   "hls_1080_720_480",
			Output:    video.JobOutput{Type: "hls", Path: "videos/abc123/"},
		})
		if err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
		if job.ID != "job_123" || job.Status != "queued" {
			t.Fatalf("CreateJob result = %+v", job)
		}

		got, err := v.GetJob("job_123")
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if got.Status != "finished" || got.Outputs["hls_master"] == "" {
			t.Fatalf("GetJob result = %+v", got)
		}
	})

	t.Run("ListJobs", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/video/jobs", jsonHandler(t, http.StatusOK, map[string]interface{}{
			"status": "ok", "count": 1,
			"jobs": []map[string]interface{}{{"id": "job_123", "status": "processing"}},
		}))
		v := video.New(newTestAPI(t, mux))

		out, err := v.ListJobs()
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if out.Count != 1 || len(out.Jobs) != 1 || out.Jobs[0].ID != "job_123" {
			t.Fatalf("ListJobs result = %+v", out)
		}
	})

	t.Run("CancelJob", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/video/jobs/job_123/cancel", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok"})(w, r)
		})
		v := video.New(newTestAPI(t, mux))
		if err := v.CancelJob("job_123"); err != nil {
			t.Fatalf("CancelJob: %v", err)
		}
	})

	t.Run("Perfiles: crear, listar, ver y eliminar", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/video/profiles", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				var p video.Profile
				decodeBody(t, r, &p)
				if len(p.Renditions) != 1 {
					t.Errorf("renditions = %+v", p.Renditions)
				}
				jsonHandler(t, http.StatusOK, p)(w, r)
			case http.MethodGet:
				jsonHandler(t, http.StatusOK, map[string]interface{}{
					"status": "ok", "count": 1,
					"profiles": []video.Profile{{Name: "hls_720", Type: "hls"}},
				})(w, r)
			default:
				t.Errorf("method inesperado: %s", r.Method)
				jsonHandler(t, http.StatusOK, map[string]interface{}{})(w, r)
			}
		})
		mux.HandleFunc("/v1/video/profiles/hls_720", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				jsonHandler(t, http.StatusOK, video.Profile{Name: "hls_720", Type: "hls"})(w, r)
			case http.MethodDelete:
				jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok"})(w, r)
			default:
				t.Errorf("method inesperado: %s", r.Method)
				jsonHandler(t, http.StatusOK, map[string]interface{}{})(w, r)
			}
		})
		v := video.New(newTestAPI(t, mux))

		created, err := v.CreateProfile(video.Profile{
			Name: "hls_720", Type: "hls",
			Renditions: []video.Rendition{{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3000, AudioBitrate: 128, FPS: 30}},
		})
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		if created.Name != "hls_720" {
			t.Fatalf("CreateProfile result = %+v", created)
		}

		list, err := v.ListProfiles()
		if err != nil {
			t.Fatalf("ListProfiles: %v", err)
		}
		if list.Count != 1 || len(list.Profiles) != 1 {
			t.Fatalf("ListProfiles result = %+v", list)
		}

		got, err := v.GetProfile("hls_720")
		if err != nil {
			t.Fatalf("GetProfile: %v", err)
		}
		if got.Name != "hls_720" {
			t.Fatalf("GetProfile result = %+v", got)
		}

		if err := v.DeleteProfile("hls_720"); err != nil {
			t.Fatalf("DeleteProfile: %v", err)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────
// marketing — borrador (sin API real todavía, ver ADVERTENCIA en el paquete)
// ─────────────────────────────────────────────────────────────────────────

func TestMarketing(t *testing.T) {
	t.Run("Listas: crear, listar, ver y eliminar", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/marketing/lists", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				var body map[string]string
				decodeBody(t, r, &body)
				if body["name"] != "Clientes Pro" {
					t.Errorf("body = %+v", body)
				}
				jsonHandler(t, http.StatusOK, map[string]interface{}{
					"id": "lst_123", "name": body["name"], "description": body["description"],
				})(w, r)
			case http.MethodGet:
				jsonHandler(t, http.StatusOK, map[string]interface{}{
					"status": "ok", "count": 1,
					"lists": []map[string]interface{}{{"id": "lst_123", "name": "Clientes Pro"}},
				})(w, r)
			default:
				t.Errorf("method inesperado: %s", r.Method)
				jsonHandler(t, http.StatusOK, map[string]interface{}{})(w, r)
			}
		})
		mux.HandleFunc("/v1/marketing/lists/lst_123", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				jsonHandler(t, http.StatusOK, map[string]interface{}{"id": "lst_123", "name": "Clientes Pro"})(w, r)
			case http.MethodDelete:
				jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok"})(w, r)
			default:
				t.Errorf("method inesperado: %s", r.Method)
				jsonHandler(t, http.StatusOK, map[string]interface{}{})(w, r)
			}
		})
		mkt := marketing.New(newTestAPI(t, mux))

		created, err := mkt.CreateList("Clientes Pro", "Lista principal")
		if err != nil {
			t.Fatalf("CreateList: %v", err)
		}
		if created.ID != "lst_123" {
			t.Fatalf("CreateList result = %+v", created)
		}

		list, err := mkt.ListLists()
		if err != nil {
			t.Fatalf("ListLists: %v", err)
		}
		if list.Count != 1 {
			t.Fatalf("ListLists result = %+v", list)
		}

		got, err := mkt.GetList("lst_123")
		if err != nil {
			t.Fatalf("GetList: %v", err)
		}
		if got.Name != "Clientes Pro" {
			t.Fatalf("GetList result = %+v", got)
		}

		if err := mkt.DeleteList("lst_123"); err != nil {
			t.Fatalf("DeleteList: %v", err)
		}
	})

	t.Run("Contactos: crear y (des)asociar a una lista", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/marketing/contacts", func(w http.ResponseWriter, r *http.Request) {
			var c marketing.Contact
			decodeBody(t, r, &c)
			if c.Email != "cliente@dominio.com" || len(c.Tags) != 2 {
				t.Errorf("body = %+v", c)
			}
			jsonHandler(t, http.StatusOK, c)(w, r)
		})
		mux.HandleFunc("/v1/marketing/lists/lst_123/contacts", func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			decodeBody(t, r, &body)
			if body["email"] != "cliente@dominio.com" {
				t.Errorf("body = %+v", body)
			}
			jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok"})(w, r)
		})
		mkt := marketing.New(newTestAPI(t, mux))

		created, err := mkt.CreateContact(marketing.Contact{
			Email: "cliente@dominio.com", FirstName: "Ana", Tags: []string{"pro", "mx"},
		})
		if err != nil {
			t.Fatalf("CreateContact: %v", err)
		}
		if created.Email != "cliente@dominio.com" {
			t.Fatalf("CreateContact result = %+v", created)
		}

		if err := mkt.AddContactToList("lst_123", "cliente@dominio.com"); err != nil {
			t.Fatalf("AddContactToList: %v", err)
		}
		if err := mkt.RemoveContactFromList("lst_123", "cliente@dominio.com"); err != nil {
			t.Fatalf("RemoveContactFromList: %v", err)
		}
	})

	t.Run("CreateSegment", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/v1/marketing/segments", func(w http.ResponseWriter, r *http.Request) {
			var s marketing.Segment
			decodeBody(t, r, &s)
			if len(s.Filters) != 1 || s.Filters[0].Field != "country" {
				t.Errorf("body = %+v", s)
			}
			s.ID = "seg_1"
			jsonHandler(t, http.StatusOK, s)(w, r)
		})
		mkt := marketing.New(newTestAPI(t, mux))

		out, err := mkt.CreateSegment(marketing.Segment{
			Name:    "México",
			Filters: []marketing.SegmentFilter{{Field: "country", Op: "eq", Value: "MX"}},
		})
		if err != nil {
			t.Fatalf("CreateSegment: %v", err)
		}
		if out.ID != "seg_1" {
			t.Fatalf("CreateSegment result = %+v", out)
		}
	})

	t.Run("Campañas: crear, ver, enviar (inmediato y programado), métricas y eliminar", func(t *testing.T) {
		var lastSchedule interface{}

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/marketing/campaigns", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			var c marketing.Campaign
			decodeBody(t, r, &c)
			if c.ListID != "lst_123" {
				t.Errorf("body = %+v", c)
			}
			c.ID = "cmp_123"
			jsonHandler(t, http.StatusOK, c)(w, r)
		})
		mux.HandleFunc("/v1/marketing/campaigns/cmp_123", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				jsonHandler(t, http.StatusOK, marketing.Campaign{ID: "cmp_123", Name: "Newsletter Enero"})(w, r)
			case http.MethodDelete:
				jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok"})(w, r)
			default:
				t.Errorf("method inesperado: %s", r.Method)
				jsonHandler(t, http.StatusOK, map[string]interface{}{})(w, r)
			}
		})
		mux.HandleFunc("/v1/marketing/campaigns/cmp_123/send", func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			decodeBody(t, r, &body)
			lastSchedule = body["schedule_at"]
			jsonHandler(t, http.StatusOK, map[string]interface{}{"status": "ok"})(w, r)
		})
		mux.HandleFunc("/v1/marketing/campaigns/cmp_123/stats", jsonHandler(t, http.StatusOK, map[string]interface{}{
			"status": "ok", "delivered": 100, "opens": 40, "clicks": 10, "bounces": 2, "unsubscribes": 1,
		}))
		mkt := marketing.New(newTestAPI(t, mux))

		created, err := mkt.CreateCampaign(marketing.Campaign{
			Name: "Newsletter Enero", FromName: "Webability", FromEmail: "news@tudominio.com",
			Subject: "Novedades de enero", ListID: "lst_123", HTML: "<h1>Hola</h1>",
		})
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		if created.ID != "cmp_123" {
			t.Fatalf("CreateCampaign result = %+v", created)
		}

		got, err := mkt.GetCampaign("cmp_123")
		if err != nil {
			t.Fatalf("GetCampaign: %v", err)
		}
		if got.Name != "Newsletter Enero" {
			t.Fatalf("GetCampaign result = %+v", got)
		}

		if err := mkt.SendCampaign("cmp_123", ""); err != nil {
			t.Fatalf("SendCampaign (inmediato): %v", err)
		}
		if lastSchedule != nil {
			t.Fatalf("schedule_at debería ser null para envío inmediato, fue %v", lastSchedule)
		}

		if err := mkt.SendCampaign("cmp_123", "2026-01-10T15:00:00-06:00"); err != nil {
			t.Fatalf("SendCampaign (programado): %v", err)
		}
		if lastSchedule != "2026-01-10T15:00:00-06:00" {
			t.Fatalf("schedule_at = %v, want 2026-01-10T15:00:00-06:00", lastSchedule)
		}

		stats, err := mkt.GetCampaignStats("cmp_123")
		if err != nil {
			t.Fatalf("GetCampaignStats: %v", err)
		}
		if stats.Delivered != 100 || stats.Opens != 40 || stats.Clicks != 10 || stats.Bounces != 2 || stats.Unsubscribes != 1 {
			t.Fatalf("GetCampaignStats result = %+v", stats)
		}

		if err := mkt.DeleteCampaign("cmp_123"); err != nil {
			t.Fatalf("DeleteCampaign: %v", err)
		}
	})
}
