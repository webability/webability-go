// Tests de integración: corren contra la API real de WebAbility (o contra el
// host indicado en WA_TEST_BASE_URL) usando credenciales reales. Se saltan
// automáticamente si no hay credenciales configuradas, así que no rompen CI
// ni corridas locales normales (`go test ./...` sigue usando solo los mocks
// de webability_test.go).
//
// Para ejecutarlos:
//
//	export WA_TEST_CLIENT_ID=tu_client_id
//	export WA_TEST_TOKEN=tu_token_secreto
//	# opcional, para apuntar a un entorno distinto al de producción:
//	export WA_TEST_BASE_URL=https://staging.api.webability.info
//	go test -run Integration -v ./...
//
// video y marketing no tienen API real todavía (ver ADVERTENCIA en sus
// paquetes), así que no hay test de integración para ellos.
//
// Los tests de DNS e Image son autocontenidos: crean sus propios recursos con
// nombres únicos y los eliminan al final (t.Cleanup), incluso si el test
// falla a medias. El test de Mail envía un correo real y por eso requiere un
// gate adicional (WA_TEST_MAIL_TO) para no disparar envíos por accidente.
//
// El caso de "template válido" requiere además WA_TEST_MAIL_TEMPLATE: el id
// de una plantilla ya creada y activa para la cuenta de prueba desde
// Consola → Correos → Plantillas (no hay forma de crearla por API, es el
// comportamiento esperado — ver /documentacion/mail#template). Sin esa
// variable, ese subtest se salta; el de "template inexistente" (3025) corre
// siempre que corre TestIntegration_Mail, porque no depende de datos
// preexistentes en la cuenta.
package api_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/webability/webability-go/dns"
	"github.com/webability/webability-go/image"
	"github.com/webability/webability-go/mail"
	"github.com/webability/webability-go/wa"
)

// integrationAPI construye un wa.API con credenciales reales tomadas del
// entorno, o salta el test si no están configuradas.
func integrationAPI(t *testing.T) *wa.API {
	t.Helper()

	clientID := os.Getenv("WA_TEST_CLIENT_ID")
	token := os.Getenv("WA_TEST_TOKEN")
	if clientID == "" || token == "" {
		t.Skip("WA_TEST_CLIENT_ID / WA_TEST_TOKEN no configurados — saltando test de integración")
	}

	if baseURL := os.Getenv("WA_TEST_BASE_URL"); baseURL != "" {
		return wa.NewWithURL(baseURL, clientID, token)
	}
	return wa.New(clientID, token)
}

// uniqueSuffix da un sufijo corto y suficientemente único para no chocar con
// recursos de otras corridas (incluso en paralelo).
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// ─────────────────────────────────────────────────────────────────────────
// dns — integración real: crear zona → agregar registro → leer → modificar
// → eliminar registro → eliminar zona
// ─────────────────────────────────────────────────────────────────────────

func TestIntegration_DNS(t *testing.T) {
	api := integrationAPI(t)
	d := dns.New(api)

	zoneName := fmt.Sprintf("wa-client-test-%s.example.com", uniqueSuffix())

	added, err := d.AddZone(zoneName)
	if err != nil {
		t.Fatalf("AddZone(%q): %v", zoneName, err)
	}
	if added.Key == 0 || added.Name != zoneName {
		t.Fatalf("AddZone result = %+v", added)
	}

	// Limpieza garantizada aunque el resto del test falle a medias.
	t.Cleanup(func() {
		if err := d.DeleteZone(added.Key); err != nil {
			t.Logf("cleanup: DeleteZone(%d) falló: %v", added.Key, err)
		}
	})

	rec, err := d.AddRecord(added.Key, dns.RecordInput{
		Name: "@", RRType: "A", TTL: 1800, Data: "203.0.113.10",
	})
	if err != nil {
		t.Fatalf("AddRecord: %v", err)
	}
	if rec.Key == 0 || rec.Zone != added.Key {
		t.Fatalf("AddRecord result = %+v", rec)
	}

	zone, err := d.GetZone(zoneName)
	if err != nil {
		t.Fatalf("GetZone(%q): %v", zoneName, err)
	}
	if zone.Zone.Key != added.Key {
		t.Fatalf("GetZone.Zone.Key = %d, want %d", zone.Zone.Key, added.Key)
	}
	found := false
	for _, r := range zone.Records {
		if r.Key == rec.Key {
			found = true
			if r.Data != "203.0.113.10" {
				t.Fatalf("registro leído con data = %q, want 203.0.113.10", r.Data)
			}
		}
	}
	if !found {
		t.Fatalf("el registro recién creado (key=%d) no aparece en GetZone: %+v", rec.Key, zone.Records)
	}

	newTTL := 3600
	newData := "203.0.113.20"
	if err := d.UpdateRecord(rec.Key, dns.RecordUpdate{TTL: &newTTL, Data: &newData}); err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}

	zone2, err := d.GetZone(zoneName)
	if err != nil {
		t.Fatalf("GetZone tras update: %v", err)
	}
	for _, r := range zone2.Records {
		if r.Key == rec.Key && r.Data != newData {
			t.Fatalf("después de UpdateRecord, data = %q, want %q", r.Data, newData)
		}
	}

	if err := d.DeleteRecord(rec.Key); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}

	zones, err := d.ListZones()
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	present := false
	for _, z := range zones.Zones {
		if z.Key == added.Key {
			present = true
		}
	}
	if !present {
		t.Fatalf("la zona recién creada (key=%d) no aparece en ListZones", added.Key)
	}
	// La eliminación de la zona la hace t.Cleanup.
}

// ─────────────────────────────────────────────────────────────────────────
// image — integración real: subir → obtener → eliminar
// ─────────────────────────────────────────────────────────────────────────

func TestIntegration_Image(t *testing.T) {
	api := integrationAPI(t)
	img := image.New(api)

	path := fmt.Sprintf("wa-client-test/%s.jpg", uniqueSuffix())
	content := "contenido de prueba — wa-client-test " + uniqueSuffix()

	uploaded, err := img.Upload(path, path[strings.LastIndex(path, "/")+1:], strings.NewReader(content))
	if err != nil {
		t.Fatalf("Upload(%q): %v", path, err)
	}
	if uploaded.Path != path {
		t.Fatalf("Upload result = %+v, want path=%q", uploaded, path)
	}

	t.Cleanup(func() {
		if _, err := img.Delete(path); err != nil {
			t.Logf("cleanup: Delete(%q) falló: %v", path, err)
		}
	})

	// El GET real procesa/transcodifica la imagen (webp/avif/jpg), así que no
	// podemos comparar bytes con el original — solo confirmamos que responde
	// 200 con contenido no vacío para una salida chica.
	resp, err := img.Get(strings.TrimSuffix(path, ".jpg") + "/100x100/" + path[strings.LastIndex(path, "/")+1:] + ".jpg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.StatusCode != 200 || len(resp.Body) == 0 {
		t.Fatalf("Get status=%d, body_len=%d, want 200 y body no vacío", resp.StatusCode, len(resp.Body))
	}
}

// ─────────────────────────────────────────────────────────────────────────
// mail — integración real: envía un correo real, por eso requiere un gate
// adicional (WA_TEST_MAIL_TO) para no disparar envíos por accidente.
// ─────────────────────────────────────────────────────────────────────────

func TestIntegration_Mail(t *testing.T) {
	api := integrationAPI(t)

	to := os.Getenv("WA_TEST_MAIL_TO")
	if to == "" {
		t.Skip("WA_TEST_MAIL_TO no configurado — saltando tests de envío real de correo")
	}

	m := mail.New(api)

	t.Run("Send a un correo existente termina en sent", func(t *testing.T) {
		out, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@webability.info", Name: "WA Client Test"},
			To:       mail.Recipient{Email: to},
			Subject:  "[wa-client-test] " + uniqueSuffix(),
			Text:     "Este es un correo de prueba de integración de github.com/webability/webability-go.",
			WaitSend: true,
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueKey == 0 || out.To != to {
			t.Fatalf("Send result = %+v", out)
		}

		status := pollMailStatus(t, m, out.QueueKey)
		if status.QueueStatus != mail.QueueStatusSent {
			t.Fatalf("QueueStatus final = %q (detalle: %q), want %q",
				status.QueueStatus, status.ErrorDetail, mail.QueueStatusSent)
		}
	})

	t.Run("Send a un dominio inexistente termina en error", func(t *testing.T) {
		badTo := fmt.Sprintf("no-existe-%s@dominio-invalido-wa-client-test.invalid", uniqueSuffix())
		out, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@webability.info", Name: "WA Client Test"},
			To:       mail.Recipient{Email: badTo},
			Subject:  "[wa-client-test] " + uniqueSuffix(),
			Text:     "Este correo se espera que rebote — dominio inexistente a propósito.",
			WaitSend: true,
		})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if out.QueueKey == 0 {
			t.Fatalf("Send result = %+v", out)
		}

		status := pollMailStatus(t, m, out.QueueKey)
		if status.QueueStatus != mail.QueueStatusError {
			t.Fatalf("QueueStatus final = %q, want %q (se esperaba que el envío a un dominio inexistente fallara)",
				status.QueueStatus, mail.QueueStatusError)
		}
		if status.ErrorDetail == "" {
			t.Fatal("ErrorDetail vacío, se esperaba el detalle del rechazo SMTP")
		}
	})

	t.Run("Status de una clave de cola inexistente", func(t *testing.T) {
		_, err := m.Status(999999999)
		if err == nil {
			t.Fatal("se esperaba error para una clave de cola inexistente")
		}
	})

	t.Run("Send con template inexistente devuelve APIError 3025 sin encolar", func(t *testing.T) {
		badTemplate := "wa-client-test-no-existe-" + uniqueSuffix()
		_, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@webability.info", Name: "WA Client Test"},
			To:       mail.Recipient{Email: to},
			Template: badTemplate,
		})
		if err == nil {
			t.Fatalf("se esperaba error para el template inexistente %q", badTemplate)
		}
		apiErr, ok := err.(*wa.APIError)
		if !ok {
			t.Fatalf("error = %T (%v), want *wa.APIError", err, err)
		}
		if apiErr.Code != 3025 {
			t.Fatalf("APIError = %+v, want code=3025", apiErr)
		}
	})

	t.Run("Send con template real y activa termina en sent", func(t *testing.T) {
		template := os.Getenv("WA_TEST_MAIL_TEMPLATE")
		if template == "" {
			t.Skip("WA_TEST_MAIL_TEMPLATE no configurado — saltando test de envío con plantilla registrada")
		}

		out, err := m.Send(mail.SendRequest{
			From:     mail.Address{Email: "no-reply@webability.info", Name: "WA Client Test"},
			To:       mail.Recipient{Email: to, Vars: map[string]interface{}{"nombre": "WA Client Test"}},
			Template: template,
			WaitSend: true,
		})
		if err != nil {
			t.Fatalf("Send con template=%q: %v", template, err)
		}
		if out.QueueKey == 0 {
			t.Fatalf("Send result = %+v", out)
		}

		status := pollMailStatus(t, m, out.QueueKey)
		if status.QueueStatus != mail.QueueStatusSent {
			t.Fatalf("QueueStatus final = %q (detalle: %q), want %q",
				status.QueueStatus, status.ErrorDetail, mail.QueueStatusSent)
		}
	})
}

// pollMailStatus consulta Status repetidamente hasta que el envío se resuelva
// (sent/error) o se agote un tiempo máximo del lado del cliente — un colchón
// extra sobre el timeout de wait_send del servidor, por si el mailer está
// momentáneamente saturado.
func pollMailStatus(t *testing.T, m *mail.Mail, queueKey int) *mail.StatusResult {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		status, err := m.Status(queueKey)
		if err != nil {
			t.Fatalf("Status(%d): %v", queueKey, err)
		}
		if status.QueueStatus != mail.QueueStatusPending && status.QueueStatus != mail.QueueStatusProcessing {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("Status(%d) no se resolvió a tiempo, quedó en %q", queueKey, status.QueueStatus)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
