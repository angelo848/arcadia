package importers

import "testing"

func TestParseVDF(t *testing.T) {
	t.Parallel()

	t.Run("IMP-002/aceita comentários, escapes de barra e chaves repetidas", func(t *testing.T) {
		t.Parallel()
		text := "\xef\xbb\xbf// biblioteca\n\"root\"\n{\n\t\"path\"\t\"C:\\\\Games\\\\Steam\"\n\t\"path\"\t\"D:\\\\Jogos\"\n\t\"sub\"\n\t{\n\t\t\"name\"\t\"a \\\"quoted\\\" name\"\n\t}\n}\n"
		obj, err := parseVDF([]byte(text))
		if err != nil {
			t.Fatalf("parseVDF: %v", err)
		}
		root, ok := obj.child("root")
		if !ok {
			t.Fatal("root object missing")
		}
		first, _ := root.get("path")
		if first != `C:\Games\Steam` {
			t.Errorf("first path = %q", first)
		}
		sub, _ := root.child("sub")
		name, _ := sub.get("name")
		if name != `a "quoted" name` {
			t.Errorf("name = %q", name)
		}
	})

	t.Run("IMP-002/texto truncado é erro", func(t *testing.T) {
		t.Parallel()
		for _, text := range []string{`"a" {`, `"a" "b`, `"a"`, `}`} {
			if _, err := parseVDF([]byte(text)); err == nil {
				t.Errorf("parseVDF(%q) = nil error, want error", text)
			}
		}
	})
}
