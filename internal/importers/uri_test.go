package importers

import (
	"context"
	"strings"
	"testing"
)

func TestURIScan(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantTitle string
		wantOK    bool
	}{
		{"epic", "com.epicgames.launcher://apps/ns:item:Sugar?action=launch", "Sugar", true},
		{"steam", "steam://rungameid/730", "730", true},
		{"spaces around are trimmed", "  gog://launch/1207658930  ", "1207658930", true},
		{"no path title", "ea://", "", false},
		{"no scheme", "Sugar", "", false},
		{"spaces inside", "gog://launch/a b", "", false},
		{"file scheme is refused", "file:///C:/Games/a.exe", "", false},
		{"empty", "", "", false},
		{"too long", "gog://" + strings.Repeat("a", maxURIRunes), "", false},
	}
	for _, tc := range cases {
		t.Run("IMP-011/"+tc.name, func(t *testing.T) {
			res, err := URI().Scan(context.Background(), ScanInput{Path: tc.in})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantOK {
				if len(res.Candidates) != 0 || len(res.Warnings) == 0 {
					t.Errorf("res = %+v, want a warning and no candidate", res)
				}
				return
			}
			if len(res.Candidates) != 1 {
				t.Fatalf("candidates = %+v", res.Candidates)
			}
			c := res.Candidates[0]
			if c.Title != tc.wantTitle || c.LaunchType != LaunchURI || c.LaunchTarget != strings.TrimSpace(tc.in) || c.Source != SourceURI {
				t.Errorf("candidate = %+v", c)
			}
		})
	}
}
