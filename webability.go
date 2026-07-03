// Copyright Philippe Thomassigny 2026.
// Use of this source code is governed by a MIT licence.
// license that can be found in the LICENSE file.
//
// Package api es la librería cliente en Go para consumir la API pública de
// WebAbility (https://api.webability.info).
//
// La librería se organiza en:
//
//   - wa: objeto API base, encargado de la autenticación (firma HMAC-SHA256)
//     y del transporte HTTP (GET, POST, PUT, DELETE) hacia la API.
//   - dns: objeto DNS, que utiliza un wa.API para exponer las funciones del
//     servicio DNS de la API cliente (zonas y registros).
//
// Cada nuevo servicio de la API (mailing, imagery, etc.) se agrega como un
// subdirectorio/paquete independiente que recibe un *wa.API ya autenticado.
package api

// VERSION es la versión actual de la librería cliente de la API de WebAbility.
const VERSION = "0.1.0"
