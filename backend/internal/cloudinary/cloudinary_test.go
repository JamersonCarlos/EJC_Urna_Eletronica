package cloudinary

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSign(t *testing.T) {
	got := sign(map[string]string{"timestamp": "1315060510", "folder": "urna"}, "abcd")
	if got != "a06e6571057fe346f2f074cf85846da77d2eb154" {
		t.Fatalf("assinatura inesperada: %s", got)
	}
}

func TestNewRejectsInvalidURL(t *testing.T) {
	for _, u := range []string{"", "https://a:b@c", "cloudinary://cloud", "cloudinary://key@cloud"} {
		if _, err := New(u, ""); err == nil {
			t.Errorf("deveria rejeitar %q", u)
		}
	}
}

func TestUpload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1_1/minha-nuvem/image/upload" {
			t.Errorf("caminho inesperado: %s", r.URL.Path)
		}
		want := sign(map[string]string{"timestamp": r.FormValue("timestamp"), "folder": r.FormValue("folder")}, "segredo")
		if r.FormValue("api_key") != "chave" || r.FormValue("signature") != want {
			t.Error("credenciais ou assinatura incorretas")
		}
		if _, _, err := r.FormFile("file"); err != nil {
			t.Error("arquivo ausente")
		}
		_, _ = w.Write([]byte(`{"secure_url":"https://res.cloudinary.com/x.jpg"}`))
	}))
	defer srv.Close()

	c, err := New("cloudinary://chave:segredo@minha-nuvem", "urna")
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = srv.URL
	got, err := c.Upload(context.Background(), "foto.jpg", []byte("dados"))
	if err != nil || got != "https://res.cloudinary.com/x.jpg" {
		t.Fatalf("upload: %q %v", got, err)
	}
}
