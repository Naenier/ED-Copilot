package riches

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	thargoidStatesURL = "https://spansh.co.uk/api/systems/field_values/thargoid_war_state"
	unsafeSearchURL   = "https://spansh.co.uk/api/systems/search"
	maxUnsafePages    = 8
)

var errThargoidSystems = errors.New("Не удалось проверить системы с таргоидами по данным Spansh.")

type unsafeSystems map[string]struct{}

func (systems unsafeSystems) add(system reference) {
	if name := cleanSystemName(system.Name); name != "" {
		systems[strings.ToLower(name)] = struct{}{}
	}
	if address := id64(system.Address); address != "" {
		systems["id:"+address] = struct{}{}
	}
}

func (systems unsafeSystems) contains(name, address string) bool {
	if _, found := systems["id:"+address]; found {
		return true
	}
	_, found := systems[strings.ToLower(cleanSystemName(name))]
	return found
}

// Spansh indexes the war state and controlling allegiance separately. Both
// searches are required since systems controlled by Thargoids can have no war state.
func (p *Planner) thargoidSystems(ctx context.Context) (unsafeSystems, error) {
	var index struct {
		Values []string `json:"values"`
	}
	if err := p.request(ctx, http.MethodGet, thargoidStatesURL, nil, &index); err != nil || index.Values == nil {
		return nil, thargoidError(ctx)
	}
	states := make([]string, 0, len(index.Values))
	for _, state := range index.Values {
		if state = strings.TrimSpace(state); state != "" && !strings.EqualFold(state, "None") {
			states = append(states, state)
		}
	}
	systemIDs := make(unsafeSystems)
	if err := p.collectUnsafeSystems(ctx, "allegiance", []string{"Thargoid"}, systemIDs); err != nil {
		return nil, err
	}
	if len(states) != 0 {
		if err := p.collectUnsafeSystems(ctx, "thargoid_war_state", states, systemIDs); err != nil {
			return nil, err
		}
	}
	return systemIDs, nil
}

func (p *Planner) collectUnsafeSystems(ctx context.Context, field string, values []string, unsafe unsafeSystems) error {
	query := bodySearch{
		Filters: map[string]any{field: map[string][]string{"value": values}},
		Sort:    []map[string]any{{"name": map[string]string{"direction": "asc"}}},
		Size:    pageSize,
	}
	for page := 0; page < maxUnsafePages; page++ {
		query.Page = page
		payload, err := json.Marshal(query)
		if err != nil {
			return err
		}
		var result struct {
			Count   int         `json:"count"`
			Results []reference `json:"results"`
		}
		if err := p.request(ctx, http.MethodPost, unsafeSearchURL, bytes.NewReader(payload), &result); err != nil || result.Results == nil {
			return thargoidError(ctx)
		}
		for _, system := range result.Results {
			unsafe.add(system)
		}
		if (page+1)*pageSize >= result.Count {
			return nil
		}
		if len(result.Results) < pageSize {
			break
		}
	}
	return thargoidError(ctx)
}

func thargoidError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errThargoidSystems
}
