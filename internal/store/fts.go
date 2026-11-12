package store

// The game_fts index (FTS5, rowid = game.id) is maintained by hand in the same
// transaction as each game write (ADR 0020). sqlc does not know virtual tables,
// so these statements are plain SQL. Tests: fts_test.go, fts_query_test.go.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// reindexGame deletes the game_fts row of id and inserts it again with the
// title, description and developer/publisher names of the game. The expression
// is the same as migration 5 (migrations/00005_fts_rebuild.sql). If the game
// does not exist, nothing is inserted. Must run in the transaction that changed
// the game (FTS-002, FTS-003).
func reindexGame(ctx context.Context, tx *sql.Tx, id int64) error {
	if err := deleteGameIndex(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO game_fts (rowid, title, description, developer, publisher)
		 SELECT g.id, g.title, g.description,
		        COALESCE((SELECT group_concat(c.name, ' ') FROM game_developer gd
		                  JOIN company c ON c.id = gd.company_id WHERE gd.game_id = g.id), ''),
		        COALESCE((SELECT group_concat(c.name, ' ') FROM game_publisher gp
		                  JOIN company c ON c.id = gp.company_id WHERE gp.game_id = g.id), '')
		 FROM game g WHERE g.id = ?`, id); err != nil {
		return fmt.Errorf("store: fts insert game %d: %w", id, err)
	}
	return nil
}

// deleteGameIndex deletes the game_fts row of id (FTS-004). It runs before the
// game row is deleted, and before reindexGame, because game ids can be reused
// (no AUTOINCREMENT) and a stale row would otherwise survive.
func deleteGameIndex(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM game_fts WHERE rowid = ?`, id); err != nil {
		return fmt.Errorf("store: fts delete game %d: %w", id, err)
	}
	return nil
}

// ftsQuery turns free text into an FTS5 MATCH expression (FTS-005). Words are
// the runs of Unicode letters and digits; each becomes "word"* (a quoted prefix
// term) and the terms are joined by spaces, which FTS5 reads as AND. Because
// every term is quoted, AND, OR, NOT, ':', '*', '(', ')', '^' and '-' lose their
// operator meaning. ok is false when no word is left.
func ftsQuery(text string) (query string, ok bool) {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) == 0 {
		return "", false
	}
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	return strings.Join(terms, " "), true
}
