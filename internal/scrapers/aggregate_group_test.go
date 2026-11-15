package scrapers

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Casos da T16 (phase-12-metadados-unificados): agrupamento e ordenação do agregador (buildGroups).
// Pacote interno porque buildGroups não é exportada. Vetores de contracts/match-rules.md §1, §2.4 e §4.1.
// Cada t.Run começa pelo ID do requisito.
//
// Premissas assumidas onde a spec não fecha (ver o relatório da T16):
//   - empate de pontuação e de provedores é desfeito pelo título em ordem crescente;
//   - a trava da 2ª consulta do IGDB (match-rules §1, "ou o grupo veio da segunda consulta") vale mesmo que
//     um membro da Steam Store confirme a plataforma da biblioteca e mesmo sem chaves de plataforma.

func grpCand(provider, id, title string, year int) Candidate {
	return Candidate{Provider: provider, ProviderID: id, Title: title, Year: year}
}

func grpBuild(q domain.Query, byProvider map[string][]Candidate) []Group {
	return buildGroups(q, byProvider, "", 10)
}

func grpTitles(groups []Group) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.Title)
	}
	return out
}

func grpNear(got, want, tol int) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func TestBuildGroupsSameGame(t *testing.T) {
	t.Parallel()

	t.Run("MET-005/Hades em IGDB e SteamGridDB: um grupo com 2 provedores", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"igdb":        {grpCand("igdb", "1", "Hades", 2020)},
			"steamgriddb": {grpCand("steamgriddb", "9", "Hades", 2020)},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if n := len(groups[0].Members); n != 2 {
			t.Fatalf("membros = %d, want 2", n)
		}
		if n := len(groups[0].Refs); n != 2 {
			t.Fatalf("refs = %d, want 2", n)
		}
	})

	t.Run("MET-005/Resident Evil 1996 e 2002 do mesmo provedor: 2 grupos", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Resident Evil"}, map[string][]Candidate{
			"igdb": {grpCand("igdb", "1", "Resident Evil", 1996), grpCand("igdb", "2", "Resident Evil", 2002)},
		})
		if len(groups) != 2 {
			t.Fatalf("grupos = %d, want 2", len(groups))
		}
	})

	t.Run("MET-005/SteamGridDB sem ano entra no grupo mais antigo (empate)", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Resident Evil"}, map[string][]Candidate{
			"igdb":        {grpCand("igdb", "1", "Resident Evil", 1996), grpCand("igdb", "2", "Resident Evil", 2002)},
			"steamgriddb": {grpCand("steamgriddb", "9", "Resident Evil", 0)},
		})
		if len(groups) != 2 {
			t.Fatalf("grupos = %d, want 2", len(groups))
		}
		for _, g := range groups {
			switch g.Year {
			case 1996:
				if len(g.Members) != 2 {
					t.Fatalf("grupo de 1996 tem %d membros, want 2 (com o SteamGridDB)", len(g.Members))
				}
			case 2002:
				if len(g.Members) != 1 {
					t.Fatalf("grupo de 2002 tem %d membros, want 1", len(g.Members))
				}
			default:
				t.Fatalf("ano de grupo inesperado %d", g.Year)
			}
		}
	})

	t.Run("MET-005/Resident Evil 2002 e SteamGridDB 1996: 2 grupos", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Resident Evil"}, map[string][]Candidate{
			"igdb":        {grpCand("igdb", "1", "Resident Evil", 2002)},
			"steamgriddb": {grpCand("steamgriddb", "9", "Resident Evil", 1996)},
		})
		if len(groups) != 2 {
			t.Fatalf("grupos = %d, want 2 (diferença de 6 anos)", len(groups))
		}
	})

	t.Run("MET-005/Sonic e Sonic Mania: 2 grupos", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Sonic"}, map[string][]Candidate{
			"igdb":       {grpCand("igdb", "1", "Sonic", 0)},
			"steamstore": {grpCand("steamstore", "71", "Sonic Mania", 0)},
		})
		if len(groups) != 2 {
			t.Fatalf("grupos = %d, want 2", len(groups))
		}
	})

	t.Run("MET-005/título e ano exibidos são os do primeiro membro na ordem igdb, steamstore, steamgriddb", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"steamgriddb": {grpCand("steamgriddb", "9", "HADES (grid)", 2021)},
			"steamstore":  {grpCand("steamstore", "1145360", "HADES", 2021)},
			"igdb":        {grpCand("igdb", "1", "Hades", 2020)},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if groups[0].Title != "Hades" {
			t.Fatalf("título = %q, want %q (primeiro membro na ordem igdb)", groups[0].Title, "Hades")
		}
		if groups[0].Year != 2020 {
			t.Fatalf("ano = %d, want 2020 (do IGDB)", groups[0].Year)
		}
	})

	t.Run("MET-005/ano é o primeiro não zero", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"igdb":        {grpCand("igdb", "1", "Hades", 0)},
			"steamgriddb": {grpCand("steamgriddb", "9", "Hades", 2020)},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if groups[0].Year != 2020 {
			t.Fatalf("ano = %d, want 2020 (primeiro não zero)", groups[0].Year)
		}
	})

	t.Run("MET-005/três provedores no mesmo grupo, refs na ordem igdb, steamstore, steamgriddb", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"steamgriddb": {grpCand("steamgriddb", "9", "Hades", 2020)},
			"steamstore":  {grpCand("steamstore", "1145360", "Hades", 2020)},
			"igdb":        {grpCand("igdb", "1", "Hades", 2020)},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		g := groups[0]
		if len(g.Members) != 3 {
			t.Fatalf("membros = %d, want 3", len(g.Members))
		}
		want := []Ref{
			{Provider: "igdb", ProviderID: "1"},
			{Provider: "steamstore", ProviderID: "1145360"},
			{Provider: "steamgriddb", ProviderID: "9"},
		}
		if !reflect.DeepEqual(g.Refs, want) {
			t.Fatalf("refs = %+v, want %+v", g.Refs, want)
		}
	})

	t.Run("MET-005/ordem fixa: mesma entrada dá o mesmo resultado em 30 chamadas (sem depender do mapa)", func(t *testing.T) {
		t.Parallel()
		q := domain.Query{Title: "Resident Evil"}
		byProvider := map[string][]Candidate{
			"igdb":        {grpCand("igdb", "1", "Resident Evil", 1996), grpCand("igdb", "2", "Resident Evil 2", 1998)},
			"steamstore":  {grpCand("steamstore", "5", "Resident Evil", 1996)},
			"steamgriddb": {grpCand("steamgriddb", "9", "Resident Evil", 0)},
		}
		first := grpBuild(q, byProvider)
		for i := 0; i < 30; i++ {
			if got := grpBuild(q, byProvider); !reflect.DeepEqual(got, first) {
				t.Fatalf("chamada %d difere da primeira", i)
			}
		}
	})
}

func TestBuildGroupsScore(t *testing.T) {
	t.Parallel()

	t.Run("MET-006/pontuação do grupo vem de ScoreGroup com a chave da biblioteca", func(t *testing.T) {
		t.Parallel()
		q := domain.Query{Title: "Pokemon Red", Year: 1998, LibraryKey: "gba"}
		igdb := Candidate{
			Provider: "igdb", ProviderID: "3", Title: "Pokemon Red Version", Year: 1998,
			PlatformKeys: []string{"gba"}, PlatformConfirmed: true,
		}
		sgdb := grpCand("steamgriddb", "9", "Pokemon Red Version", 1998)
		groups := grpBuild(q, map[string][]Candidate{
			"igdb":        {igdb},
			"steamgriddb": {sgdb},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		members := []domain.Member{
			{Provider: "igdb", Title: "Pokemon Red Version", Year: 1998, PlatformKeys: []string{"gba"}, PlatformConfirmed: true},
			{Provider: "steamgriddb", Title: "Pokemon Red Version", Year: 1998},
		}
		want := domain.ScoreGroup(q, members)
		if groups[0].Score != want {
			t.Fatalf("Score = %+v, want o de ScoreGroup %+v", groups[0].Score, want)
		}
		if !grpNear(groups[0].Score.Score, 830, 5) {
			t.Fatalf("Score = %d, want ≈ 830 (0,83 de match-rules §2.3)", groups[0].Score.Score)
		}
	})

	t.Run("MET-007/IGDB da 2ª consulta com plataforma da biblioteca trava o grupo em 0,79 e ele continua na lista", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Hades", LibraryKey: "gba"}, map[string][]Candidate{
			"igdb": {{
				Provider: "igdb", ProviderID: "1", Title: "Hades",
				PlatformKeys: []string{"gba"}, PlatformConfirmed: false,
			}},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1 (a trava não tira o grupo da lista)", len(groups))
		}
		if groups[0].Score.Score != 790 || !groups[0].Score.Capped {
			t.Fatalf("Score = %+v, want Score 790 e Capped", groups[0].Score)
		}
	})

	t.Run("MET-007/IGDB da 1ª consulta com a plataforma confirmada não trava", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Hades", LibraryKey: "gba"}, map[string][]Candidate{
			"igdb": {{
				Provider: "igdb", ProviderID: "1", Title: "Hades",
				PlatformKeys: []string{"gba"}, PlatformConfirmed: true,
			}},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if groups[0].Score.Score != 1000 || groups[0].Score.Capped {
			t.Fatalf("Score = %+v, want Score 1000 sem trava", groups[0].Score)
		}
	})

	trapCases := []struct {
		name       string
		q          domain.Query
		byProvider map[string][]Candidate
		wantScore  int
		wantCapped bool
	}{
		{
			name: "MET-007/2ª consulta do IGDB sem chaves de plataforma também trava (match-rules §1)",
			q:    domain.Query{Title: "Hades", LibraryKey: "gba"},
			byProvider: map[string][]Candidate{
				"igdb": {{Provider: "igdb", ProviderID: "1", Title: "Hades", PlatformConfirmed: false}},
			},
			wantScore:  790,
			wantCapped: true,
		},
		{
			name: "MET-007/2ª consulta do IGDB não é salva por Steam Store que lista a biblioteca (match-rules §1)",
			q:    domain.Query{Title: "Hades", LibraryKey: "gba"},
			byProvider: map[string][]Candidate{
				"igdb": {{Provider: "igdb", ProviderID: "1", Title: "Hades", PlatformKeys: []string{"gba"}, PlatformConfirmed: false}},
				"steamstore": {{
					Provider: "steamstore", ProviderID: "7", Title: "Hades",
					PlatformKeys: []string{"gba"}, PlatformConfirmed: true,
				}},
			},
			wantScore:  790,
			wantCapped: true,
		},
		{
			name: "MET-007/1ª consulta com plataformas que não incluem a da biblioteca trava",
			q:    domain.Query{Title: "Hades", LibraryKey: "gba"},
			byProvider: map[string][]Candidate{
				"igdb": {{Provider: "igdb", ProviderID: "1", Title: "Hades", PlatformKeys: []string{"snes"}, PlatformConfirmed: true}},
			},
			wantScore:  790,
			wantCapped: true,
		},
		{
			name: "MET-007/membro só do SteamGridDB (sem plataforma) não trava",
			q:    domain.Query{Title: "Hades", LibraryKey: "snes"},
			byProvider: map[string][]Candidate{
				"steamgriddb": {grpCand("steamgriddb", "9", "Hades", 0)},
			},
			wantScore:  1000,
			wantCapped: false,
		},
		{
			name: "MET-007/marcas diferentes travam em 0,79 e o grupo continua",
			q:    domain.Query{Title: "Resident Evil"},
			byProvider: map[string][]Candidate{
				"igdb": {grpCand("igdb", "2", "Resident Evil 2", 0)},
			},
			wantScore:  790,
			wantCapped: true,
		},
	}
	for _, tc := range trapCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			groups := grpBuild(tc.q, tc.byProvider)
			if len(groups) != 1 {
				t.Fatalf("grupos = %d, want 1", len(groups))
			}
			if groups[0].Score.Score != tc.wantScore || groups[0].Score.Capped != tc.wantCapped {
				t.Fatalf("Score = %+v, want Score %d Capped %v", groups[0].Score, tc.wantScore, tc.wantCapped)
			}
		})
	}
}

func TestBuildGroupsOrder(t *testing.T) {
	t.Parallel()

	t.Run("MET-008/ordem: pontuação decrescente, empate por mais provedores e depois título", func(t *testing.T) {
		t.Parallel()
		// "Resident Evil" exato = 1000. "Resident Evil 3" tem 2 provedores e 790 (trava); "Resident Evil 2" e
		// "Resident Evil 4" têm 1 provedor e 790. Empate em 790: o de 2 provedores antes; depois o título crescente.
		groups := grpBuild(domain.Query{Title: "Resident Evil"}, map[string][]Candidate{
			"igdb": {
				grpCand("igdb", "1", "Resident Evil", 0),
				grpCand("igdb", "2", "Resident Evil 2", 0),
				grpCand("igdb", "4", "Resident Evil 4", 0),
				grpCand("igdb", "3", "Resident Evil 3", 0),
			},
			"steamgriddb": {grpCand("steamgriddb", "9", "Resident Evil 3", 0)},
		})
		want := []string{"Resident Evil", "Resident Evil 3", "Resident Evil 2", "Resident Evil 4"}
		if got := grpTitles(groups); !reflect.DeepEqual(got, want) {
			t.Fatalf("ordem = %q, want %q", got, want)
		}
	})

	t.Run("MET-008/corta em 10 grupos e mantém o melhor", func(t *testing.T) {
		t.Parallel()
		cands := []Candidate{grpCand("igdb", "0", "Doom", 0)}
		for i := 2; i <= 12; i++ {
			title := "Doom " + strconv.Itoa(i)
			cands = append(cands, grpCand("igdb", title, title, 0))
		}
		groups := grpBuild(domain.Query{Title: "Doom"}, map[string][]Candidate{"igdb": cands})
		if len(groups) != 10 {
			t.Fatalf("grupos = %d, want 10", len(groups))
		}
		if groups[0].Title != "Doom" {
			t.Fatalf("primeiro = %q, want %q", groups[0].Title, "Doom")
		}
		for _, g := range groups {
			if g.Score.Score < domain.MatchAlternative {
				t.Fatalf("grupo %q com %d abaixo de 0,40", g.Title, g.Score.Score)
			}
		}
	})

	t.Run("MET-008/limite recebido é respeitado (max 2 de 3 grupos)", func(t *testing.T) {
		t.Parallel()
		groups := buildGroups(domain.Query{Title: "Doom"}, map[string][]Candidate{
			"igdb": {grpCand("igdb", "1", "Doom", 0), grpCand("igdb", "2", "Doom 2", 0), grpCand("igdb", "3", "Doom 3", 0)},
		}, "", 2)
		if len(groups) != 2 {
			t.Fatalf("grupos = %d, want 2", len(groups))
		}
	})

	t.Run("MET-008/descarta pontuação abaixo de 0,40", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Tetris"}, map[string][]Candidate{
			"igdb": {grpCand("igdb", "1", "Zelda", 0)},
		})
		if len(groups) != 0 {
			t.Fatalf("grupos = %d, want 0 (Dice 0 < 0,40)", len(groups))
		}
	})

	t.Run("MET-008/pontuação de 0,47 (Sonic) fica na lista", func(t *testing.T) {
		t.Parallel()
		groups := grpBuild(domain.Query{Title: "Sonic"}, map[string][]Candidate{
			"igdb": {grpCand("igdb", "1", "Sonic the Hedgehog", 0)},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
	})

	t.Run("MET-008/lista vazia é válida", func(t *testing.T) {
		t.Parallel()
		if got := buildGroups(domain.Query{Title: "Hades"}, nil, "", 10); len(got) != 0 {
			t.Fatalf("grupos com mapa nulo = %d, want 0", len(got))
		}
		if got := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{}); len(got) != 0 {
			t.Fatalf("grupos com mapa vazio = %d, want 0", len(got))
		}
	})
}

func TestBuildGroupsPlatform(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		q       domain.Query
		libName string
		cand    Candidate
		want    string
	}{
		{
			name:    "MET-008/nome da plataforma da biblioteca quando confere",
			q:       domain.Query{Title: "Hades", LibraryKey: "gba"},
			libName: "Game Boy Advance (biblioteca)",
			cand: Candidate{Provider: "igdb", ProviderID: "1", Title: "Hades",
				PlatformKeys: []string{"gba"}, PlatformConfirmed: true},
			want: "Game Boy Advance (biblioteca)",
		},
		{
			name:    "MET-008/sem biblioteca confirmada: nome canônico da primeira chave conhecida",
			q:       domain.Query{Title: "Hades"},
			libName: "Nome da biblioteca",
			cand: Candidate{Provider: "igdb", ProviderID: "1", Title: "Hades",
				PlatformKeys: []string{"ps2"}, PlatformConfirmed: true},
			want: "Sony Playstation 2",
		},
		{
			name:    "MET-008/chave desconhecida antes da conhecida: usa a primeira conhecida",
			q:       domain.Query{Title: "Hades"},
			libName: "",
			cand: Candidate{Provider: "igdb", ProviderID: "1", Title: "Hades",
				PlatformKeys: []string{"zz-desconhecida", "n64"}, PlatformConfirmed: true},
			want: "Nintendo 64",
		},
		{
			name:    "MET-008/sem plataforma: vazio",
			q:       domain.Query{Title: "Hades"},
			libName: "Nome da biblioteca",
			cand:    Candidate{Provider: "igdb", ProviderID: "1", Title: "Hades"},
			want:    "",
		},
		{
			name:    "MET-008/plataforma da biblioteca de um membro não confirmado não vira o nome da biblioteca",
			q:       domain.Query{Title: "Hades", LibraryKey: "gba"},
			libName: "Game Boy Advance (biblioteca)",
			cand: Candidate{Provider: "igdb", ProviderID: "1", Title: "Hades",
				PlatformKeys: []string{"gba"}, PlatformConfirmed: false},
			want: "Nintendo Game Boy Advance",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			groups := buildGroups(tc.q, map[string][]Candidate{tc.cand.Provider: {tc.cand}}, tc.libName, 10)
			if len(groups) != 1 {
				t.Fatalf("grupos = %d, want 1", len(groups))
			}
			if groups[0].Platform != tc.want {
				t.Fatalf("Platform = %q, want %q", groups[0].Platform, tc.want)
			}
		})
	}

	t.Run("MET-003/Steam Store confirma a biblioteca mesmo com PlatformConfirmed no zero value", func(t *testing.T) {
		t.Parallel()
		// A Steam Store sempre confirma (T15); o zero value de PlatformConfirmed não pode travar nem perder o nome.
		groups := buildGroups(domain.Query{Title: "Hades", LibraryKey: "windows"}, map[string][]Candidate{
			"steamstore": {{Provider: "steamstore", ProviderID: "730", Title: "Hades", PlatformKeys: []string{"windows"}}},
		}, "PC (Windows)", 10)
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if groups[0].Score.Capped || groups[0].Score.Score != 1000 {
			t.Fatalf("Score = %+v, want 1000 sem trava", groups[0].Score)
		}
		if groups[0].Platform != "PC (Windows)" {
			t.Fatalf("Platform = %q, want %q", groups[0].Platform, "PC (Windows)")
		}
	})
}

func TestBuildGroupsFields(t *testing.T) {
	t.Parallel()

	t.Run("MET-012/Meta e Images de cada provedor vêm do candidato; provedor sem eles não aparece", func(t *testing.T) {
		t.Parallel()
		meta := &Metadata{Title: "Hades", Description: "Roguelike"}
		imgs := []RemoteMedia{{Kind: KindCover, URL: "https://images.igdb.com/igdb/image/upload/t_cover_big/abc.jpg"}}
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"igdb":        {{Provider: "igdb", ProviderID: "1", Title: "Hades", Meta: meta, Images: imgs}},
			"steamgriddb": {grpCand("steamgriddb", "9", "Hades", 0)},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		got, ok := groups[0].Meta["igdb"]
		if !ok || !reflect.DeepEqual(got, *meta) {
			t.Fatalf("Meta[igdb] = %+v (ok=%v), want %+v", got, ok, *meta)
		}
		if !reflect.DeepEqual(groups[0].Images["igdb"], imgs) {
			t.Fatalf("Images[igdb] = %+v, want %+v", groups[0].Images["igdb"], imgs)
		}
		if _, ok := groups[0].Meta["steamgriddb"]; ok {
			t.Fatalf("Meta[steamgriddb] presente sem o candidato trazer texto")
		}
	})

	t.Run("MET-068/miniatura é a do primeiro membro com miniatura, na ordem de junção", func(t *testing.T) {
		t.Parallel()
		steamThumb := "https://cdn.cloudflare.steamstatic.com/steam/apps/1145360/capsule_231x87.jpg"
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"igdb":       {grpCand("igdb", "1", "Hades", 2020)},
			"steamstore": {{Provider: "steamstore", ProviderID: "1145360", Title: "Hades", ThumbURL: steamThumb}},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if groups[0].ThumbURL != steamThumb {
			t.Fatalf("ThumbURL = %q, want a da Steam Store (IGDB sem miniatura)", groups[0].ThumbURL)
		}
	})

	t.Run("MET-068/com miniatura nos dois, vale a do IGDB (primeiro na ordem)", func(t *testing.T) {
		t.Parallel()
		igdbThumb := "https://images.igdb.com/igdb/image/upload/t_cover_small/abc.jpg"
		steamThumb := "https://cdn.cloudflare.steamstatic.com/steam/apps/1145360/capsule_231x87.jpg"
		groups := grpBuild(domain.Query{Title: "Hades"}, map[string][]Candidate{
			"igdb":       {{Provider: "igdb", ProviderID: "1", Title: "Hades", Year: 2020, ThumbURL: igdbThumb}},
			"steamstore": {{Provider: "steamstore", ProviderID: "1145360", Title: "Hades", Year: 2020, ThumbURL: steamThumb}},
		})
		if len(groups) != 1 {
			t.Fatalf("grupos = %d, want 1", len(groups))
		}
		if groups[0].ThumbURL != igdbThumb {
			t.Fatalf("ThumbURL = %q, want %q", groups[0].ThumbURL, igdbThumb)
		}
	})
}
