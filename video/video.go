// Package video implementa el objeto Video, que enlaza un wa.API ya autenticado
// y expone las funciones del servicio de video (transcodificación y streaming)
// de la API de WebAbility.
//
// ADVERTENCIA: a diferencia de dns/image/mail, el servidor aún NO implementa
// /v1/video — este paquete sigue la especificación publicada en
// /documentacion/video (jobs, perfiles) como borrador adelantado. Los paths y
// los payloads pueden cambiar cuando se construya la API real; revisar contra
// sites/api.webability.info/v1/video cuando exista antes de usarlo en producción.
package video

import (
	"fmt"

	"github.com/webability/webability-go/wa"
)

// Video enlaza un objeto wa.API para hacer las llamadas al servicio de video.
type Video struct {
	API *wa.API
}

// New crea un objeto Video a partir de un wa.API ya autenticado.
func New(api *wa.API) *Video {
	return &Video{API: api}
}

// JobOutput describe la salida deseada de un job (tipo y ruta destino).
type JobOutput struct {
	Type string `json:"type"` // "hls", "mp4", etc.
	Path string `json:"path"`
}

// CreateJobRequest son los campos para crear un job de transcodificación.
type CreateJobRequest struct {
	SourceURL  string    `json:"source_url"`
	Profile    string    `json:"profile"`
	Output     JobOutput `json:"output"`
	WebhookURL string    `json:"webhook_url,omitempty"`
}

// Job representa un trabajo de transcodificación (asíncrono).
type Job struct {
	ID       string            `json:"id"`
	Status   string            `json:"status"` // queued | processing | finished | failed
	Outputs  map[string]string `json:"outputs,omitempty"`
	Progress int               `json:"progress,omitempty"`
}

// JobList es la respuesta de ListJobs.
type JobList struct {
	Status string `json:"status"`
	Jobs   []Job  `json:"jobs"`
	Count  int    `json:"count"`
}

// ListJobs lista los jobs de transcodificación. GET /v1/video/jobs
func (v *Video) ListJobs() (*JobList, error) {
	resp, err := v.API.Get("/v1/video/jobs")
	if err != nil {
		return nil, err
	}
	var out JobList
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// CreateJob crea un job de transcodificación. POST /v1/video/jobs
func (v *Video) CreateJob(req CreateJobRequest) (*Job, error) {
	resp, err := v.API.Post("/v1/video/jobs", req)
	if err != nil {
		return nil, err
	}
	var out Job
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// GetJob obtiene el detalle y estado de un job. GET /v1/video/jobs/{id}
func (v *Video) GetJob(id string) (*Job, error) {
	resp, err := v.API.Get("/v1/video/jobs/" + id)
	if err != nil {
		return nil, err
	}
	var out Job
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// CancelJob cancela un job en curso (si es posible). POST /v1/video/jobs/{id}/cancel
func (v *Video) CancelJob(id string) error {
	_, err := v.API.Post("/v1/video/jobs/"+id+"/cancel", nil)
	return err
}

// Rendition define una salida (resolución/bitrate/fps) dentro de un perfil.
type Rendition struct {
	Name         string `json:"name"`
	Width        int    `json:"w"`
	Height       int    `json:"h"`
	VideoBitrate int    `json:"video_bitrate"`
	AudioBitrate int    `json:"audio_bitrate"`
	FPS          int    `json:"fps"`
}

// Segments define la duración de segmento (en segundos) para salidas HLS.
type Segments struct {
	Duration int `json:"duration"`
}

// Profile define las salidas (renditions) que produce un job.
type Profile struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"` // "hls", "mp4", etc.
	Renditions []Rendition `json:"renditions"`
	Segments   Segments    `json:"segments,omitempty"`
}

// ProfileList es la respuesta de ListProfiles.
type ProfileList struct {
	Status   string    `json:"status"`
	Profiles []Profile `json:"profiles"`
	Count    int       `json:"count"`
}

// ListProfiles lista los perfiles de transcodificación. GET /v1/video/profiles
func (v *Video) ListProfiles() (*ProfileList, error) {
	resp, err := v.API.Get("/v1/video/profiles")
	if err != nil {
		return nil, err
	}
	var out ProfileList
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// CreateProfile crea un perfil de transcodificación. POST /v1/video/profiles
func (v *Video) CreateProfile(p Profile) (*Profile, error) {
	resp, err := v.API.Post("/v1/video/profiles", p)
	if err != nil {
		return nil, err
	}
	var out Profile
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// GetProfile obtiene el detalle de un perfil. GET /v1/video/profiles/{id}
func (v *Video) GetProfile(id string) (*Profile, error) {
	resp, err := v.API.Get("/v1/video/profiles/" + id)
	if err != nil {
		return nil, err
	}
	var out Profile
	if err := resp.Decode(&out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w", err)
	}
	return &out, nil
}

// DeleteProfile elimina un perfil. DELETE /v1/video/profiles/{id}
func (v *Video) DeleteProfile(id string) error {
	_, err := v.API.Delete("/v1/video/profiles/" + id)
	return err
}
