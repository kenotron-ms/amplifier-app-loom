package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TranscriptEntry is one item in the transcript list.
type TranscriptEntry struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`
	Time      string    `json:"time"`
	App       string    `json:"app"`
	HasAudio  bool      `json:"has_audio"`
	CreatedAt time.Time `json:"created_at"`
}

func meetingsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "meetings")
}

func (s *Server) listTranscripts(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(meetingsDir(), "transcripts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, []TranscriptEntry{})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recDir := filepath.Join(meetingsDir(), "recordings")
	var result []TranscriptEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		t := parseTranscriptID(id)
		_, hasAudio := os.Stat(filepath.Join(recDir, id+".wav"))
		t.HasAudio = hasAudio == nil
		fi, _ := e.Info()
		if fi != nil {
			t.CreatedAt = fi.ModTime()
		}
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if result == nil {
		result = []TranscriptEntry{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getTranscriptContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.Contains(id, "..") {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	data, err := os.ReadFile(filepath.Join(meetingsDir(), "transcripts", id+".md"))
	if err != nil {
		writeError(w, http.StatusNotFound, "transcript not found")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (s *Server) getTranscriptAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.Contains(id, "..") {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	path := filepath.Join(meetingsDir(), "recordings", id+".wav")
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "audio not found")
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	w.Header().Set("Content-Type", "audio/wav")
	if fi != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
	}
	io.Copy(w, f)
}

func (s *Server) deleteTranscript(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.Contains(id, "..") {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	os.Remove(filepath.Join(meetingsDir(), "transcripts", id+".md"))
	os.Remove(filepath.Join(meetingsDir(), "recordings", id+".wav")) // best-effort
	w.WriteHeader(http.StatusNoContent)
}

func parseTranscriptID(id string) TranscriptEntry {
	e := TranscriptEntry{ID: id}
	parts := strings.SplitN(id, "_", 3)
	if len(parts) >= 1 {
		e.Date = parts[0]
	}
	if len(parts) >= 2 {
		e.Time = strings.ReplaceAll(parts[1], "-", ":")
	}
	if len(parts) >= 3 {
		e.App = parts[2]
	}
	return e
}
