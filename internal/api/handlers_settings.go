package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type settingsResponse struct {
	AIProvider        string `json:"aiProvider"`
	AnthropicKeySet   bool   `json:"anthropicKeySet"`
	AnthropicModel    string `json:"anthropicModel"`
	OpenAIKeySet      bool   `json:"openAIKeySet"`
	OpenAIModel       string `json:"openAIModel"`
	AIConfigured      bool   `json:"aiConfigured"`
	PreferredTerminal string `json:"preferredTerminal,omitempty"`
}

type settingsUpdateRequest struct {
	AIProvider         string `json:"aiProvider"`
	AnthropicKey       string `json:"anthropicKey"`
	AnthropicModel     string `json:"anthropicModel"`
	OpenAIKey          string `json:"openAIKey"`
	OpenAIModel        string `json:"openAIModel"`
	OnboardingComplete bool   `json:"onboardingComplete"`
	PreferredTerminal  string `json:"preferredTerminal,omitempty"`
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
		AIProvider:        provider,
		AnthropicKeySet:   anthropicSet,
		AnthropicModel:    s.cfg.AnthropicModel,
		OpenAIKeySet:      openaiSet,
		OpenAIModel:       s.cfg.OpenAIModel,
		AIConfigured:      configured,
		PreferredTerminal: s.cfg.PreferredTerminal,
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
	if req.OnboardingComplete {
		s.cfg.OnboardingComplete = true
	}
	if req.PreferredTerminal != "" {
		s.cfg.PreferredTerminal = req.PreferredTerminal
	}

	if err := s.store.SaveConfig(r.Context(), s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save settings")
		return
	}

	// Re-initialize the NL client with new config
	s.reinitNLClient()

	s.getSettings(w, r)
}

func (s *Server) testSettings(w http.ResponseWriter, r *http.Request) {
	client := s.getNLClient()
	if client == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      false,
			"message": "No API key configured. Add your key and save first.",
		})
		return
	}

	provider := s.cfg.AIProvider
	if provider == "" {
		provider = "anthropic"
	}
	model := s.cfg.AnthropicModel
	if provider == "openai" {
		model = s.cfg.OpenAIModel
	}
	if model == "" {
		model = "default"
	}

	if err := client.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      false,
			"message": fmt.Sprintf("Connection failed: %v", err),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": fmt.Sprintf("Connected to %s (%s)", provider, model),
	})
}
