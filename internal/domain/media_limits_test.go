package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestMediaKindLimit(t *testing.T) {
	t.Parallel()

	t.Run("AJU-020/cover 1 (alterado em 2026-11-15, capa = 1), screenshot 10, background 2, title 2, select 2, banner 2", func(t *testing.T) {
		t.Parallel()
		want := map[string]int{
			"cover":      1,
			"screenshot": 10,
			"background": 2,
			"title":      2,
			"select":     2,
			"banner":     2,
		}
		for kind, n := range want {
			if got := domain.MediaKindLimit(kind); got != n {
				t.Errorf("MediaKindLimit(%q) = %d, want %d", kind, got, n)
			}
		}
	})

	t.Run("AJU-020/tipo desconhecido (logo, vazio) dá 0", func(t *testing.T) {
		t.Parallel()
		for _, kind := range []string{"logo", ""} {
			if got := domain.MediaKindLimit(kind); got != 0 {
				t.Errorf("MediaKindLimit(%q) = %d, want 0", kind, got)
			}
		}
	})

	t.Run("VID-002/MediaKindLimit(video) = 1 e os demais valores não mudam (cover 1, screenshot 10, background/title/select/banner 2, desconhecido 0)", func(t *testing.T) {
		t.Parallel()
		want := map[string]int{
			"video":      1,
			"cover":      1,
			"screenshot": 10,
			"background": 2,
			"title":      2,
			"select":     2,
			"banner":     2,
			"Video":      0,
			"logo":       0,
			"":           0,
		}
		for kind, n := range want {
			if got := domain.MediaKindLimit(kind); got != n {
				t.Errorf("MediaKindLimit(%q) = %d, want %d", kind, got, n)
			}
		}
	})
}
