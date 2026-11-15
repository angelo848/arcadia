package scrapers_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// offer builds one provider's answer: the URLs of one kind, in the order the provider gave them.
func offer(provider, kind string, urls ...string) scrapers.Offer {
	media := make([]scrapers.RemoteMedia, 0, len(urls))
	for _, u := range urls {
		media = append(media, scrapers.RemoteMedia{Kind: kind, URL: u})
	}
	return scrapers.Offer{Provider: provider, Media: media}
}

// pick is one expected choice of SelectImages.
func pick(provider, kind, url string) scrapers.Pick {
	return scrapers.Pick{Provider: provider, Kind: kind, URL: url}
}

// sameSelection compares two selections element by element, so nil and empty slices are equal.
func sameSelection(got, want []scrapers.Pick) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSelectImages(t *testing.T) {
	t.Parallel()

	const (
		igdb        = "igdb"
		steamgriddb = "steamgriddb"
		steamstore  = "steamstore"
	)

	type testCase struct {
		name        string
		offers      []scrapers.Offer
		have        map[string]int
		active      map[string]int
		windows     bool
		want        []scrapers.Pick
		wantSkipped int
	}

	// MET-020: prioridade entre provedores por tipo e classe de plataforma (match-rules §4.2).
	priority := []testCase{
		{
			name: "MET-020/jogo vazio Windows: 1 capa do SteamGridDB, skipped 3 (alterado em 2026-11-15, capa = 1)",
			offers: []scrapers.Offer{
				offer(igdb, scrapers.KindCover, "i1"),
				offer(steamgriddb, scrapers.KindCover, "s1", "s2", "s3"),
			},
			windows:     true,
			want:        []scrapers.Pick{pick(steamgriddb, scrapers.KindCover, "s1")},
			wantSkipped: 3,
		},
		{
			name: "MET-020/jogo vazio console: IGDB primeiro, o SteamGridDB fica de fora (alterado em 2026-11-15, capa = 1)",
			offers: []scrapers.Offer{
				offer(steamgriddb, scrapers.KindCover, "s1", "s2", "s3"),
				offer(igdb, scrapers.KindCover, "i1"),
			},
			windows:     false,
			want:        []scrapers.Pick{pick(igdb, scrapers.KindCover, "i1")},
			wantSkipped: 3,
		},
		{
			name: "MET-020/screenshot: IGDB antes da Steam Store (Windows)",
			offers: []scrapers.Offer{
				offer(steamstore, scrapers.KindScreenshot, "st1", "st2", "st3"),
				offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5"),
			},
			have:        map[string]int{scrapers.KindScreenshot: 8},
			windows:     true,
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig1"), pick(igdb, scrapers.KindScreenshot, "ig2")},
			wantSkipped: 6,
		},
		{
			name: "MET-020/screenshot: IGDB antes da Steam Store (console)",
			offers: []scrapers.Offer{
				offer(steamstore, scrapers.KindScreenshot, "st1", "st2", "st3"),
				offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5"),
			},
			have:        map[string]int{scrapers.KindScreenshot: 8},
			windows:     false,
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig1"), pick(igdb, scrapers.KindScreenshot, "ig2")},
			wantSkipped: 6,
		},
		{
			name: "MET-020/background: SteamGridDB antes do IGDB (Windows)",
			offers: []scrapers.Offer{
				offer(igdb, scrapers.KindBackground, "ig1", "ig2"),
				offer(steamgriddb, scrapers.KindBackground, "sg1", "sg2"),
			},
			have:        map[string]int{scrapers.KindBackground: 1},
			windows:     true,
			want:        []scrapers.Pick{pick(steamgriddb, scrapers.KindBackground, "sg1")},
			wantSkipped: 3,
		},
		{
			name: "MET-020/background: SteamGridDB antes do IGDB (console)",
			offers: []scrapers.Offer{
				offer(igdb, scrapers.KindBackground, "ig1", "ig2"),
				offer(steamgriddb, scrapers.KindBackground, "sg1", "sg2"),
			},
			have:        map[string]int{scrapers.KindBackground: 1},
			windows:     false,
			want:        []scrapers.Pick{pick(steamgriddb, scrapers.KindBackground, "sg1")},
			wantSkipped: 3,
		},
		{
			name: "MET-020/mesma URL em IGDB e Steam entra uma vez",
			offers: []scrapers.Offer{
				offer(igdb, scrapers.KindScreenshot, "u1"),
				offer(steamstore, scrapers.KindScreenshot, "u1", "u2"),
			},
			windows:     false,
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "u1"), pick(steamstore, scrapers.KindScreenshot, "u2")},
			wantSkipped: 0,
		},
		{
			name: "MET-020/mesma URL repetida no mesmo provedor entra uma vez",
			offers: []scrapers.Offer{
				offer(steamgriddb, scrapers.KindCover, "s1", "s1", "s2"),
			},
			windows:     false,
			want:        []scrapers.Pick{pick(steamgriddb, scrapers.KindCover, "s1")},
			wantSkipped: 1,
		},
		{
			name: "MET-020/ordem de cada provedor preservada",
			offers: []scrapers.Offer{
				offer(igdb, scrapers.KindScreenshot, "ig3", "ig1", "ig2"),
			},
			have:        map[string]int{scrapers.KindScreenshot: 8},
			windows:     false,
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig3"), pick(igdb, scrapers.KindScreenshot, "ig1")},
			wantSkipped: 1,
		},
	}

	// MET-021: o limite conta as imagens atuais (have) e os jobs pendentes ou em andamento (active).
	limits := []testCase{
		{
			name:        "MET-021/9 de 10 screenshots: 1 vaga, skipped 4",
			offers:      []scrapers.Offer{offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5")},
			have:        map[string]int{scrapers.KindScreenshot: 9},
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig1")},
			wantSkipped: 4,
		},
		{
			name:        "MET-021/2 capas: nenhuma vaga",
			offers:      []scrapers.Offer{offer(steamgriddb, scrapers.KindCover, "s1", "s2")},
			have:        map[string]int{scrapers.KindCover: 2},
			want:        nil,
			wantSkipped: 2,
		},
		{
			name:        "MET-021/8 screenshots e 2 jobs ativos: 0, skipped 5",
			offers:      []scrapers.Offer{offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5")},
			have:        map[string]int{scrapers.KindScreenshot: 8},
			active:      map[string]int{scrapers.KindScreenshot: 2},
			want:        nil,
			wantSkipped: 5,
		},
		{
			name:        "MET-021/active conta: 8 em have e 1 ativa dão 1 vaga, skipped 4",
			offers:      []scrapers.Offer{offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5")},
			have:        map[string]int{scrapers.KindScreenshot: 8},
			active:      map[string]int{scrapers.KindScreenshot: 1},
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig1")},
			wantSkipped: 4,
		},
		{
			name:        "MET-021/concluído já está em have: 9 em have e nenhum ativo dão 1 vaga, skipped 4",
			offers:      []scrapers.Offer{offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5")},
			have:        map[string]int{scrapers.KindScreenshot: 9},
			active:      map[string]int{},
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig1")},
			wantSkipped: 4,
		},
		{
			name:        "MET-021/have e active ausentes tratam como 0: 3 capas e 1 vaga, skipped 2 (alterado em 2026-11-15, capa = 1)",
			offers:      []scrapers.Offer{offer(steamgriddb, scrapers.KindCover, "s1", "s2", "s3")},
			have:        nil,
			active:      nil,
			want:        []scrapers.Pick{pick(steamgriddb, scrapers.KindCover, "s1")},
			wantSkipped: 2,
		},
		{
			name:        "MET-021/have e active negativos tratam como 0: não somam vaga, 1 vaga, skipped 2 (alterado em 2026-11-15, capa = 1)",
			offers:      []scrapers.Offer{offer(steamgriddb, scrapers.KindCover, "s1", "s2", "s3")},
			have:        map[string]int{scrapers.KindCover: -5},
			active:      map[string]int{scrapers.KindCover: -1},
			want:        []scrapers.Pick{pick(steamgriddb, scrapers.KindCover, "s1")},
			wantSkipped: 2,
		},
		{
			name:        "MET-021/have negativo de screenshot: as 5 do IGDB entram, skipped 0",
			offers:      []scrapers.Offer{offer(igdb, scrapers.KindScreenshot, "ig1", "ig2", "ig3", "ig4", "ig5")},
			have:        map[string]int{scrapers.KindScreenshot: -3},
			want:        []scrapers.Pick{pick(igdb, scrapers.KindScreenshot, "ig1"), pick(igdb, scrapers.KindScreenshot, "ig2"), pick(igdb, scrapers.KindScreenshot, "ig3"), pick(igdb, scrapers.KindScreenshot, "ig4"), pick(igdb, scrapers.KindScreenshot, "ig5")},
			wantSkipped: 0,
		},
		{
			name:        "MET-021/have acima do limite não gera vaga negativa",
			offers:      []scrapers.Offer{offer(steamgriddb, scrapers.KindCover, "s1", "s2")},
			have:        map[string]int{scrapers.KindCover: 5},
			active:      map[string]int{scrapers.KindCover: 1},
			want:        nil,
			wantSkipped: 2,
		},
	}

	// MET-025: só cover, screenshot e background são escolhidos; os outros tipos não entram nem contam.
	kinds := []testCase{
		{
			name: "MET-025/tipo title, banner e select oferecidos não entram e não contam em skipped",
			offers: []scrapers.Offer{
				offer(igdb, scrapers.KindTitle, "t1"),
				offer(igdb, scrapers.KindBanner, "b1"),
				offer(steamgriddb, scrapers.KindSelect, "x1"),
			},
			windows:     true,
			want:        nil,
			wantSkipped: 0,
		},
		{
			name: "MET-025/tipos fora dos três não contam em skipped (capa cheia: só a capa conta)",
			offers: []scrapers.Offer{
				offer(steamgriddb, scrapers.KindCover, "s1"),
				offer(igdb, scrapers.KindTitle, "t1"),
				offer(igdb, scrapers.KindBanner, "b1"),
				offer(steamgriddb, scrapers.KindSelect, "x1"),
			},
			have:        map[string]int{scrapers.KindCover: 2},
			windows:     false,
			want:        nil,
			wantSkipped: 1,
		},
	}

	run := func(t *testing.T, cases []testCase) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				got, skipped := scrapers.SelectImages(c.offers, c.have, c.active, c.windows)
				if !sameSelection(got, c.want) {
					t.Errorf("SelectImages escolheu %v; want %v", got, c.want)
				}
				if skipped != c.wantSkipped {
					t.Errorf("skipped = %d; want %d", skipped, c.wantSkipped)
				}
			})
		}
	}

	t.Run("MET-020", func(t *testing.T) { t.Parallel(); run(t, priority) })
	t.Run("MET-021", func(t *testing.T) { t.Parallel(); run(t, limits) })
	t.Run("MET-025", func(t *testing.T) { t.Parallel(); run(t, kinds) })
}
