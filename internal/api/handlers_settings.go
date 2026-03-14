package api

import (
	"encoding/json"
	"net/http"
	"os"
)

type settingsResponse struct {
	AIProvider      string `json:"aiProvider"`
	AnthropicKeySet bool   `json:"anthropicKeySet"`
	AnthropicModel  string `json:"anthropicModel"`
	OpenAIKeySet    bool   `json:"openAIKeySet"`
	OpenAIModel     string `json:"openAIModel"`
	AIConfigured    bool   `json:"aiConfigured"`
}

type settingsUpdateRequest struct {
	AIProvider     string `json:"aiProvider"`
	AnthropicKey   string `json:"anthropicKey"`
	AnthropicModel string `json:"anthropicModel"`
	OpenAIKey      string `json:"openAIKey"`
	OpenAIModel    string `json:"openAIModel"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	provider := s.cfg.AIProvider
	if provider == "" {
		provider = "anthropic"
	}
	// Determine if actually configured (env override counts too)
	anthropicSet := s.cfg.AnthropicKey != "" || os.Getenv("ANTHROPIC_API_KEY") != ""
	openaiSet := s.cfg.OpenAIKey != "" || os.Getenv("OPENAI_API_KEY") != ""
	configured := (provider == "anthropic" && anthropicSet) || (provider == "openai" && openaiSet)
	writeJSON(w, http.StatusOK, settingsResponse{
		AIProvider:      provider,
		AnthropicKeySet: anthropicSet,
		AnthropicModel:  s.cfg.AnthropicModel,
		OpenAIKeySet:    openaiSet,
		OpenAIModel:     s.cfg.OpenAIModel,
		AIConfigured:    configured,
	})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.AIProvider != "" {
		s.cfg.AIProvider = req.AIProvider
	}
	// Only update keys if a non-empty value is provided (empty = keep existing)
	if req.AnthropicKey != "" {
		s.cfg.AnthropicKey = req.AnthropicKey
	}
	if req.OpenAIKey != "" {
		s.cfg.OpenAIKey = req.OpenAIKey
	}
	if req.AnthropicModel != "" {
		s.cfg.AnthropicModel = req.AnthropicModel
	}
	if req.OpenAIModel != "" {
		s.cfg.OpenAIModel = req.OpenAIModel
	}

	if err := s.store.SaveConfig(r.Context(), s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	// Re-initialize the NL client with new config
	s.reinitNLClient()

	s.getSettings(w, r)
}
