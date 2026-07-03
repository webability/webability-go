// Package image implementa el objeto Image, que enlaza un wa.API ya autenticado
// y expone las funciones del servicio de imágenes de la API cliente de WebAbility
// (https://api.webability.info/v1/image).
package image

import (
	"fmt"
	"io"

	"github.com/webability/webability-go/wa"
)

// Image enlaza un objeto wa.API para hacer las llamadas al servicio de imágenes de la API.
type Image struct {
	API *wa.API
}

// New crea un objeto Image a partir de un wa.API ya autenticado.
func New(api *wa.API) *Image {
	return &Image{API: api}
}

// Get obtiene (y procesa) una imagen. GET /v1/image/{path}/{WxH}/{file.ext}
// Devuelve la respuesta cruda: resp.Body trae el binario y
// resp.Header.Get("Content-Type") el tipo de imagen resultante.
func (i *Image) Get(path string) (*wa.Response, error) {
	return i.API.Get("/v1/image/" + path)
}

// UploadResult es la respuesta de Upload.
type UploadResult struct {
	Status      string `json:"status"`
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	CachePurged int    `json:"cache_purged"`
}

// Upload sube una imagen original al repositorio del cliente. POST /v1/image/{path}
// path es la ruta destino (ej: "productos/zapatilla.jpg"); filename es el nombre
// de archivo enviado en el multipart (usualmente el mismo basename de path).
func (i *Image) Upload(path, filename string, file io.Reader) (*UploadResult, error) {
	resp, err := i.API.PostMultipart("/v1/image/"+path, "file", filename, file)
	if err != nil {
		return nil, err
	}
	var out UploadResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// DeleteResult es la respuesta de Delete/PurgeCache.
type DeleteResult struct {
	Status             string `json:"status"`
	DeletedFirstcache  int    `json:"deleted_firstcache"`
	DeletedSecondcache int    `json:"deleted_secondcache"`
	Total              int    `json:"total"`
}

// PurgeCache purga toda la caché de versiones procesadas (secondcache); los
// originales se conservan. DELETE /v1/image
func (i *Image) PurgeCache() (*DeleteResult, error) {
	return i.delete("/v1/image")
}

// Delete elimina el original y todas sus versiones cacheadas de una imagen
// específica, o una carpeta completa si path termina en "/".
// path puede incluir subcarpetas (ej: "productos/zapatilla.jpg" o "productos/").
// DELETE /v1/image/{path/imagen.ext} o /v1/image/{carpeta/}
func (i *Image) Delete(path string) (*DeleteResult, error) {
	return i.delete("/v1/image/" + path)
}

func (i *Image) delete(path string) (*DeleteResult, error) {
	resp, err := i.API.Delete(path)
	if err != nil {
		return nil, err
	}
	var out DeleteResult
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}
