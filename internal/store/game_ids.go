package store

import (
	"context"
	"fmt"
)

// GameIDs returns the ids of every game in the library (DEL-010). It only reads: the rows of the
// games and of the search index are not changed. The order is not defined. An empty library gives
// an empty list.
func (s *Store) GameIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM game`)
	if err != nil {
		return nil, fmt.Errorf("store: listar ids dos jogos: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: ler id do jogo: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: listar ids dos jogos: %w", err)
	}
	return ids, nil
}
