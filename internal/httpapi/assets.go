package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/tenant"
)

// Body limits by route, MASTER_PLAN.md §12.
const (
	publicMaxBodyBytes = 64 << 10
	uploadMaxBodyBytes = app.MaxAssetBytes + 64<<10 // the image plus multipart framing
)

var errNoFile = apperr.Invalid("file_required", `send the image as multipart/form-data field "file"`)

func (s *server) assetRoutes(r chi.Router) {
	r.Post("/assets", s.can(tenant.PermManageMenu, s.uploadAsset))
	r.Get("/assets/{id}", s.adminAsset)
}

type assetJSON struct {
	ID          uuid.UUID `json:"id"`
	URL         string    `json:"url"`
	AdminURL    string    `json:"admin_url"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size_bytes"`
}

func (s *server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "multipart/form-data" {
		s.fail(w, r, errNoFile)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		s.fail(w, r, errNoFile)
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			s.fail(w, r, errNoFile)
			return
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				s.fail(w, r, app.ErrAssetTooLarge)
				return
			}
			s.fail(w, r, errNoFile)
			return
		}
		if part.FormName() != "file" {
			continue
		}
		as, err := s.App.UploadAsset(r.Context(), principal(r), part)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				err = app.ErrAssetTooLarge
			}
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, assetJSON{ID: as.ID, URL: assetURL(as.ID), AdminURL: "/api/admin/assets/" + as.ID.String(),
			ContentType: as.ContentType, Size: as.Size})
		return
	}
}

func (s *server) adminAsset(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	as, f, err := s.App.AdminAsset(r.Context(), principal(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	serveAsset(w, r, as, f, "private, max-age=300")
}

// publicAsset serves an image a live menu item shows.
func (s *server) publicAsset(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	as, f, err := s.App.PublicAsset(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	// Assets never change once uploaded; a new image is a new id.
	serveAsset(w, r, as, f, "public, max-age=86400")
}

func serveAsset(w http.ResponseWriter, r *http.Request, as *app.Asset, f io.ReadSeeker, cache string) {
	h := w.Header()
	h.Set("Content-Type", as.ContentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Disposition", "inline")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", cache)
	h.Set("ETag", strconv.Quote(as.SHA256))
	http.ServeContent(w, r, "", as.CreatedAt.Time, f)
}
