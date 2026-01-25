package gateway

type AITransaction struct {
	Request Request  `json:"request,omitempty"`
	Respons Response `json:"response,omitempty"`
}

type Response struct {
}

type Request struct {
	MaxTokens    string `json:"maxTokens,omitempty"`
	ModelReplace []struct {
		Orig string `json:"orig,omitempty"`
		New  string `json:"new,omitempty"`
	} `json:"modelReplace,omitempty"`
	UserPromptReplace []PromtReplace `json:"userPromptReplace,omitempty"`
	DevPromptReplace  []PromtReplace `json:"devPromptReplace,omitempty"`
}

type PromtReplace struct {
	Orig          string `json:"orig,omitempty"`
	New           string `json:"new,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
}
