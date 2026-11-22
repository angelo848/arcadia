package agent

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// ttbHandlers returns the six timeToBeat.* handlers [TTB-007, TTB-008, TTB-020..TTB-023, TTB-040].
// Params are validated here; the rules live in the service.
func ttbHandlers(s *timeToBeatService) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodTimeToBeatGet: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.TimeToBeatGetParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			if p.GameID < 1 {
				return nil, badParams()
			}
			v, err := s.Get(ctx, p.GameID, p.Fetch)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
		ipc.MethodTimeToBeatRefresh: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.TimeToBeatGameParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			if p.GameID < 1 {
				return nil, badParams()
			}
			v, err := s.Refresh(ctx, p.GameID)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
		ipc.MethodTimeToBeatSearch: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.TimeToBeatSearchParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			if p.GameID < 1 {
				return nil, badParams()
			}
			term := ""
			if p.Term != nil {
				term = strings.TrimSpace(*p.Term)
				if n := utf8.RuneCountInString(term); n == 0 || n > ttbTermMax {
					return nil, badParams()
				}
			}
			c, err := s.Search(ctx, p.GameID, term)
			if err != nil {
				return nil, err
			}
			if c == nil {
				c = []ipc.TimeToBeatCandidate{}
			}
			return ipc.TimeToBeatSearchResult{Candidates: c}, nil
		},
		ipc.MethodTimeToBeatChoose: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.TimeToBeatChooseParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			// externalId is required, even when empty ("none of these").
			var members map[string]json.RawMessage
			if err := json.Unmarshal(params, &members); err != nil {
				return nil, badParams()
			}
			if _, ok := members["externalId"]; !ok {
				return nil, badParams()
			}
			if p.GameID < 1 {
				return nil, badParams()
			}
			v, err := s.Choose(ctx, p.GameID, p.ExternalID)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
		ipc.MethodTimeToBeatGetSettings: ipc.NoParams(func(ctx context.Context) (any, error) {
			v, err := s.Settings(ctx)
			if err != nil {
				return nil, err
			}
			return v, nil
		}),
		ipc.MethodTimeToBeatSetSettings: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.TimeToBeatSetSettingsParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			v, err := s.SetSettings(ctx, p)
			if err != nil {
				return nil, err
			}
			return v, nil
		},
	}
}
