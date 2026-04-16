package config

import (
	"encoding/json"
	"knot/internal/paths"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type RecentEntry struct {
	Alias    string    `json:"alias"`
	LastUsed time.Time `json:"last_used"`
}

type State struct {
	Recent []RecentEntry `json:"recent"`
}

func LoadState() (*State, error) {
	configPath, err := paths.GetConfigPath()
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(filepath.Dir(configPath), "state.json")

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return &State{Recent: []RecentEntry{}}, nil
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *State) Save() error {
	configPath, err := paths.GetConfigPath()
	if err != nil {
		return err
	}
	statePath := filepath.Join(filepath.Dir(configPath), "state.json")

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, data, 0600)
}

func (s *State) UpdateRecent(alias string, limit int) {
	// Find if already exists
	found := -1
	for i, entry := range s.Recent {
		if entry.Alias == alias {
			found = i
			break
		}
	}

	if found != -1 {
		// Update existing
		s.Recent[found].LastUsed = time.Now()
	} else {
		// Add new
		s.Recent = append(s.Recent, RecentEntry{
			Alias:    alias,
			LastUsed: time.Now(),
		})
	}

	// Sort by last used descending
	sort.Slice(s.Recent, func(i, j int) bool {
		return s.Recent[i].LastUsed.After(s.Recent[j].LastUsed)
	})

	// Truncate to limit
	if len(s.Recent) > limit {
		s.Recent = s.Recent[:limit]
	}
}
