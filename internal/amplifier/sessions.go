package amplifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AmplifierSession represents a session from Amplifier's on-disk store.
type AmplifierSession struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ProjectPath string    `json:"projectPath"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ListProjectSessions reads Amplifier's on-disk session store and returns
// sessions for the given project path, sorted by recency (newest first).
// Returns an empty slice (not nil) when no sessions are found.
func ListProjectSessions(projectPath string) ([]AmplifierSession, error) {
	dir, err := sessionsDir(projectPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []AmplifierSession
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		sessions = append(sessions, AmplifierSession{
			ID:          m.SessionID,
			Name:        m.Name,
			ProjectPath: m.WorkingDir,
			CreatedAt:   m.Created,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})

	if sessions == nil {
		sessions = []AmplifierSession{}
	}
	return sessions, nil
}
