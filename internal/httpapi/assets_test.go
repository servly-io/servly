package httpapi

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAssets(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		e := newEnv(t, db)
		c := e.owner()
		img := pngBytes(t)

		as := mustCode(t, c.upload("file", img), http.StatusCreated)
		if as["content_type"] != "image/png" || as["size_bytes"].(float64) != float64(len(img)) {
			t.Fatalf("upload: %v", as)
		}
		// Not attached to an item yet: private.
		mustCode(t, e.client().do(http.MethodGet, as["url"].(string), nil), http.StatusNotFound)
		adm := c.do(http.MethodGet, as["admin_url"].(string), nil)
		if adm.Code != http.StatusOK || !bytes.Equal(adm.Body.Bytes(), img) {
			t.Fatalf("admin asset: %d", adm.Code)
		}

		cat := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Pizza"}), http.StatusCreated)
		item := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{
			"category_id": cat["id"], "name": "Margherita", "price_minor": 950, "image_asset_id": as["id"]}), http.StatusCreated)
		if item["image_url"] != as["url"] {
			t.Fatalf("item image: %v", item)
		}
		pub := e.client().do(http.MethodGet, as["url"].(string), nil)
		if pub.Code != http.StatusOK || !bytes.Equal(pub.Body.Bytes(), img) || pub.Header().Get("Content-Type") != "image/png" ||
			pub.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("public asset: %d %v", pub.Code, pub.Header())
		}
		// Removing the image from the item makes it private again.
		upd := mustCode(t, c.do(http.MethodPatch, "/api/admin/menu/items/"+item["id"].(string), map[string]any{"image_asset_id": nil}), http.StatusOK)
		if upd["image_url"] != nil {
			t.Fatalf("image not removed: %v", upd)
		}
		mustCode(t, e.client().do(http.MethodGet, as["url"].(string), nil), http.StatusNotFound)

		// Another restaurant can neither read nor attach roma's asset.
		if _, err := e.app.CreateTenant(context.Background(), app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
			OwnerEmail: "mallory@example.com", OwnerPassword: "another long pw"}); err != nil {
			t.Fatal(err)
		}
		m := e.client()
		mustCode(t, m.login("mallory@example.com", "another long pw"), http.StatusOK)
		mustCode(t, m.do(http.MethodGet, as["admin_url"].(string), nil), http.StatusNotFound)
		mcat := mustCode(t, m.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "X"}), http.StatusCreated)
		r := m.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": mcat["id"], "name": "Thief", "price_minor": 1, "image_asset_id": as["id"]})
		if r.Code != http.StatusNotFound {
			t.Fatalf("attach foreign asset: %d %s", r.Code, r.Body.String())
		}
	})
}

func TestAssetRejections(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	if r := c.upload("file", []byte("<html><script>alert(1)</script></html>")); r.Code != http.StatusUnprocessableEntity || r.code() != "unsupported_media_type" {
		t.Errorf("html upload: %d %s", r.Code, r.Body.String())
	}
	if r := c.upload("picture", pngBytes(t)); r.Code != http.StatusUnprocessableEntity || r.code() != "file_required" {
		t.Errorf("wrong field: %d %s", r.Code, r.Body.String())
	}
	big := append(pngBytes(t), make([]byte, app.DefaultMaxImageBytes)...)
	if r := c.upload("file", big); r.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized: %d %s", r.Code, r.Body.String())
	}
	if r := c.do(http.MethodPost, "/api/admin/assets", map[string]any{"file": "x"}); r.Code != http.StatusUnprocessableEntity || r.code() != "file_required" {
		t.Errorf("json body: %d %s", r.Code, r.Body.String())
	}
	mustCode(t, c.do(http.MethodPost, "/api/admin/staff", map[string]any{"email": "bob@example.com", "role": "staff", "password": "bob's long password"}), http.StatusCreated)
	bc := e.client()
	mustCode(t, bc.login("bob@example.com", "bob's long password"), http.StatusOK)
	if r := bc.upload("file", pngBytes(t)); r.Code != http.StatusForbidden {
		t.Errorf("staff upload: %d", r.Code)
	}
}
