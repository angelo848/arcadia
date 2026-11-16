package main

// Tests for T27: .mp4 served with Content-Type video/mp4 and Range support [VID-022, CON-027].
// Assumption: http.ServeFile answers Range; the handler only adds the Content-Type for .mp4 (any
// case) before serving and keeps every restriction of the media root.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestServeMediaVideo(t *testing.T) {
	t.Parallel()

	data := t.TempDir()
	media := filepath.Join(data, "media")
	if err := os.MkdirAll(filepath.Join(media, "7"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4<<20)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(media, "7", "v.mp4"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(media, "7", "UP.MP4"), body[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(media, "7", "h.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "library.db"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{dataRoot: data}

	get := func(url, rng string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		rec := httptest.NewRecorder()
		a.serveMedia(rec, req)
		return rec
	}

	t.Run("VID-022/GET .mp4 responde 200 com Content-Type video/mp4 e Accept-Ranges bytes", func(t *testing.T) {
		t.Parallel()
		rec := get("/media/7/v.mp4", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "video/mp4" {
			t.Errorf("Content-Type = %q, want video/mp4", ct)
		}
		if ar := rec.Header().Get("Accept-Ranges"); ar != "bytes" {
			t.Errorf("Accept-Ranges = %q, want bytes", ar)
		}
		if !bytes.Equal(rec.Body.Bytes(), body) {
			t.Error("full body differs from the file")
		}
	})

	t.Run("VID-022/extensão .MP4 em maiúsculas também é video/mp4", func(t *testing.T) {
		t.Parallel()
		rec := get("/media/7/UP.MP4", "")
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "video/mp4" {
			t.Errorf("status %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
		}
	})

	t.Run("VID-022/Range bytes=0-99 responde 206 com Content-Range e o corpo parcial", func(t *testing.T) {
		t.Parallel()
		rec := get("/media/7/v.mp4", "bytes=0-99")
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", rec.Code)
		}
		want := "bytes 0-99/" + strconv.Itoa(len(body))
		if cr := rec.Header().Get("Content-Range"); cr != want {
			t.Errorf("Content-Range = %q, want %q", cr, want)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "video/mp4" {
			t.Errorf("Content-Type = %q", ct)
		}
		if !bytes.Equal(rec.Body.Bytes(), body[:100]) {
			t.Errorf("body has %d bytes, want the first 100", rec.Body.Len())
		}
	})

	t.Run("VID-022/Range no meio do arquivo devolve só esse trecho (sem corpo inteiro)", func(t *testing.T) {
		t.Parallel()
		rec := get("/media/7/v.mp4", "bytes=1000000-1000009")
		if rec.Code != http.StatusPartialContent || rec.Body.Len() != 10 {
			t.Fatalf("status %d, body %d bytes, want 206 and 10", rec.Code, rec.Body.Len())
		}
		if !bytes.Equal(rec.Body.Bytes(), body[1000000:1000010]) {
			t.Error("partial body differs from the file slice")
		}
	})

	t.Run("VID-022/Range aberto bytes=N- vai até o fim", func(t *testing.T) {
		t.Parallel()
		start := len(body) - 50
		rec := get("/media/7/v.mp4", "bytes="+strconv.Itoa(start)+"-")
		if rec.Code != http.StatusPartialContent || !bytes.Equal(rec.Body.Bytes(), body[start:]) {
			t.Errorf("status %d, body %d bytes", rec.Code, rec.Body.Len())
		}
	})

	t.Run("VID-022/Range fora do intervalo responde 416", func(t *testing.T) {
		t.Parallel()
		rec := get("/media/7/v.mp4", "bytes="+strconv.Itoa(len(body)+10)+"-")
		if rec.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("status = %d, want 416", rec.Code)
		}
	})

	t.Run("VID-022/fora da pasta, pasta e arquivo ausente continuam 404", func(t *testing.T) {
		t.Parallel()
		for _, u := range []string{"/media/../library.db", "/media/7/..%2F..%2Flibrary.db", "/media/7", "/media/7/nao.mp4"} {
			if rec := get(u, "bytes=0-9"); rec.Code != http.StatusNotFound {
				t.Errorf("%s: status %d, want 404", u, rec.Code)
			}
		}
	})

	t.Run("VID-022/imagens continuam com o tipo de antes", func(t *testing.T) {
		t.Parallel()
		rec := get("/media/7/h.png", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct == "video/mp4" || ct == "" {
			t.Errorf("Content-Type of png = %q", ct)
		}
	})
}
