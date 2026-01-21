package gateway

type AIConfig struct {
	Model         string `json:"model,omitempty"`
	MaxTokens     string `json:"maxTokens,omitempty"`
	PromptReplace []struct {
		Orig          string `json:"orig,omitempty"`
		New           string `json:"new,omitempty"`
		CaseSensitive bool   `json:"caseSensitive,omitempty"`
		PromptType    string `json:"promptType,omitempty"`
	} `json:"promptReplace,omitempty"`
}
