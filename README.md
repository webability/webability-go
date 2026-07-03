# webability-go

Cliente oficial en Go para conectarse a los servicios de [WebAbility](https://www.webability.info) — la plataforma que ofrece DNS gestionado, procesamiento de imágenes/CDN, envío de correo transaccional y (próximamente) transcodificación de video y email marketing masivo, todos expuestos a través de una única API HTTP en `https://api.webability.info`.

Esta librería es la implementación de referencia: las demás (`webability-php`, `webability-js`, `webability-rust`, `webability-c`, `webability-python`) siguen el mismo contrato de autenticación y de endpoints definido aquí.

## Instalación

```bash
go get github.com/webability/webability-go@latest
```

## Uso rápido

```go
package main

import (
	"fmt"

	"github.com/webability/webability-go/dns"
	"github.com/webability/webability-go/wa"
)

func main() {
	// ClientID y Token se obtienen en la consola de tu cuenta WebAbility
	// (Configuración → API). El Token es secreto y nunca viaja en el request.
	api := wa.New("tu-client-id", "tu-token-secreto")

	d := dns.New(api)
	zones, err := d.ListZones()
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d zonas\n", zones.Count)
}
```

## Servicios disponibles

| Paquete       | Servicio                              | Estado                                    |
|---------------|----------------------------------------|--------------------------------------------|
| `dns`         | Zonas y registros DNS                  | ✅ API real (`/v1/dns`)                     |
| `image`       | Subida, procesamiento y CDN de imágenes| ✅ API real (`/v1/image`)                   |
| `mail`        | Correo transaccional                   | ✅ API real (`/v1/mail`)                    |
| `video`       | Transcodificación y streaming (HLS)    | 🚧 Borrador — sigue el spec publicado, aún sin servidor real |
| `marketing`   | Listas, segmentos y campañas de email  | 🚧 Borrador — sigue el spec publicado, aún sin servidor real |

## Autenticación

Cada cuenta tiene un **ClientID** (público) y un **Token** (secreto). Cada request se firma con HMAC-SHA256:

```
mensaje = "{METODO}|{PATH}|{TIMESTAMP}|{CLIENTID}"
digest  = hex(HMAC-SHA256(Token, mensaje))
```

y se envía con los headers `X-WA-Client`, `X-WA-Timestamp` y `X-WA-Digest`. El Token **nunca** viaja en el request — solo firma localmente en cliente y servidor. La librería (`wa.API`) hace esto automáticamente; no hay que calcular nada a mano.

## Documentación de la API

Referencia completa de cada servicio, con ejemplos de request/respuesta:
- https://www.webability.info/documentacion/dns
- https://www.webability.info/documentacion/imagenes
- https://www.webability.info/documentacion/mailing
- https://www.webability.info/documentacion/video

## Tests

```bash
go test ./...                    # tests de contrato (mocks, sin credenciales)
go test -run Integration -v ./...  # tests de integración (requieren WA_TEST_CLIENT_ID / WA_TEST_TOKEN)
```

## Licencia

MIT — ver [LICENSE](LICENSE).
