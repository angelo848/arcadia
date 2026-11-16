package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// T46..T48 (VID-027, VID-028, VID-029): o resolvedor traduz os erros novos de OpenVideo.
// Assinatura assumida: apiError traduz scrapers.ErrNotVideo em permanente "formato de vídeo não suportado".

func TestVideoResolverOpenErrors(t *testing.T) {
	t.Parallel()
	info := store.VideoGameInfo{Title: "Super Metroid", SeedKey: "snes"}
	newSS := func(openErr error) *fakeSS {
		return &fakeSS{games: []scrapers.SSGame{ssGame("10", 4, nil, "Super Metroid")}, body: mp4Bytes(), openSize: int64(len(mp4Bytes())), openErr: openErr}
	}

	for _, tc := range []struct {
		id, name, msg string
	}{
		{"VID-027", "mídia com URL fora", "URL de vídeo fora do ScreenScraper"},
		{"VID-028", "redirecionamento recusado no download", "redirecionamento para fora do ScreenScraper"},
	} {
		t.Run(tc.id+"/resolvedor: "+tc.name+" vira *media.PermanentError, FetchVideo não é chamado, last_error sem credencial", func(t *testing.T) {
			t.Parallel()
			ss := newSS(fmt.Errorf("%w: %s", scrapers.ErrBadRequest, tc.msg))
			rig := newResolverRig(t, info, ss)
			_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
			var pe *media.PermanentError
			if !errors.As(err, &pe) || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("err = %v, want permanente com %q", err, tc.msg)
			}
			if rig.files.calls != 0 {
				t.Errorf("FetchVideo chamado %d vezes", rig.files.calls)
			}
			assertNoSecrets(t, "erro", err.Error())
			assertNoSecrets(t, "log", rig.logs.String())
		})
	}

	t.Run("VID-029/resolvedor: cota, credencial e API fechada no texto chegam com a identidade do erro e sem gastar permanência", func(t *testing.T) {
		t.Parallel()
		for _, want := range []error{scrapers.ErrQuota, scrapers.ErrCredentials, scrapers.ErrUnavailable} {
			rig := newResolverRig(t, info, newSS(fmt.Errorf("download: %w", want)))
			_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
			var pe *media.PermanentError
			if !errors.Is(err, want) || errors.As(err, &pe) {
				t.Errorf("err = %v, want %v não permanente", err, want)
			}
			if rig.files.calls != 0 {
				t.Errorf("%v: FetchVideo chamado", want)
			}
		}
	})

	t.Run("VID-029/resolvedor: não encontrado vira permanente \"sem vídeo no ScreenScraper\"", func(t *testing.T) {
		t.Parallel()
		rig := newResolverRig(t, info, newSS(fmt.Errorf("download: %w", scrapers.ErrNotFound)))
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if !isPermanent(err, "sem vídeo no ScreenScraper") {
			t.Errorf("err = %v", err)
		}
		if rig.files.calls != 0 {
			t.Error("FetchVideo chamado")
		}
	})

	t.Run("VID-029/resolvedor: ErrNotVideo vira permanente \"formato de vídeo não suportado\"", func(t *testing.T) {
		t.Parallel()
		rig := newResolverRig(t, info, newSS(fmt.Errorf("download: %w", scrapers.ErrNotVideo)))
		_, _, err := rig.r.Fetch(t.Context(), videoJobOf(7))
		if !isPermanent(err, "formato de vídeo não suportado") {
			t.Errorf("err = %v", err)
		}
		if rig.files.calls != 0 {
			t.Error("FetchVideo chamado")
		}
	})
}
