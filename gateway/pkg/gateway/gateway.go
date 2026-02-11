package gateway

type endpointToken struct {
	endpoint string
	model    string
}

type AITransaction struct {
	TokenCount map[endpointToken]bool
	Request    *Request  `json:"request,omitempty"`
	Response   *Response `json:"response,omitempty"`
}

type Response struct {
	Debug       bool     `json:"debug,omitempty"`
	BannedWords []string `json:"bannedWords,omitempty"`
}

type Request struct {
	MaxTokens         string         `json:"maxTokens,omitempty"`
	ModelReplace      []ModelReplace `json:"modelReplace,omitempty"`
	UserPromptReplace []PromtReplace `json:"userPromptReplace,omitempty"`
	DevPromptReplace  []PromtReplace `json:"devPromptReplace,omitempty"`
	Block             bool           `json:"block,omitempty"`
	Debug             bool           `json:"debug,omitempty"`
}

type PromtReplace struct {
	Orig          string `json:"orig,omitempty"`
	New           string `json:"new,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
}

type ModelReplace struct {
	Orig string `json:"orig,omitempty"`
	New  string `json:"new,omitempty"`
}

func (c *AITransaction) Reset() {
	c.Request = nil
	c.Response = nil
}

func (c *AITransaction) GetRequest() *Request {
	return c.Request
}

func (c *AITransaction) GetResponse() *Response {
	return c.Response
}
