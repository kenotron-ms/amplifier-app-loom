package amplifier

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// AmplifierSession represents a session from Amplifier's on-disk store.
type AmplifierSession struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProjectPath  string    `json:"projectPath"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	IsActive     bool      `json:"isActive"`
}

// ListProjectSessions reads Amplifier's on-disk session store and returns
// sessions for the given project path, sorted by recency (newest first),
// limited to the top 10. Returns an empty slice (not nil) when no sessions
// are found.
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
		info, _ := e.Info()
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		lastActive := m.Created
		if info != nil && info.ModTime().After(lastActive) {
			lastActive = info.ModTime()
		}
		sessions = append(sessions, AmplifierSession{
			ID:           m.SessionID,
			Name:         m.Name,
			ProjectPath:  m.WorkingDir,
			CreatedAt:    m.Created,
			LastActiveAt: lastActive,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActiveAt.After(sessions[j].LastActiveAt)
	})

	// Limit to top 10 most recent sessions.
	if len(sessions) > 10 {
		sessions = sessions[:10]
	}

	// Detect which sessions are currently running.
	for i := range sessions {
		check := exec.Command("bash", "-c",
			fmt.Sprintf("ps aux | grep '%s' | grep -v grep", sessions[i].ID))
		sessions[i].IsActive = check.Run() == nil
	}

	if sessions == nil {
		sessions = []AmplifierSession{}
	}
	return sessions, nil
}
