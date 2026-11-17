package importers

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxURIRunes bounds the URI of a game, as the launch target of the library (EDT-010).
const maxURIRunes = 2000

// uriShape is a scheme followed by "://" and something: steam://..., com.epicgames.launcher://..., gog://...
var uriShape = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://\S+$`)

// uriSource turns a URI typed by the user into one review candidate, for stores the launcher has no
// importer for. The title is derived from the URI and the user edits it in the review.
type uriSource struct{}

// URI returns the URI source. ScanInput.Path is the URI.
func URI() Source { return uriSource{} }

func (uriSource) ID() string { return SourceURI }

func (uriSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	uri := strings.TrimSpace(in.Path)
	if !uriShape.MatchString(uri) || utf8.RuneCountInString(uri) > maxURIRunes {
		res.Warnings = append(res.Warnings, "URI inválida: use o formato loja://caminho")
		return res, nil
	}
	if strings.EqualFold(uri[:strings.Index(uri, "://")], "file") {
		res.Warnings = append(res.Warnings, "URI inválida: para um arquivo use a fonte Arquivo")
		return res, nil
	}
	res.Candidates = append(res.Candidates, Candidate{
		Source:       SourceURI,
		Title:        uriTitle(uri),
		LaunchType:   LaunchURI,
		LaunchTarget: uri,
	})
	return res, nil
}

// uriTitle guesses a title: the last segment of the path without the query, or "" when there is none
// (the user must type one then).
func uriTitle(uri string) string {
	rest := uri[strings.Index(uri, "://")+3:]
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimRight(rest, "/")
	if i := strings.LastIndexAny(rest, "/:"); i >= 0 {
		rest = rest[i+1:]
	}
	return strings.TrimSpace(rest)
}
