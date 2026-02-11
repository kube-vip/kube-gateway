package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

func (c *AITransaction) openAIRequest(body []byte, req *http.Request) (block bool, res *http.Response, err error) {
	chat := openai.ChatCompletionNewParams{}

	err = json.Unmarshal(body, &chat)
	if err != nil {
		return false, nil, err
	}
	if c.Request.Debug {
		b, _ := httputil.DumpRequest(req, true)
		fmt.Println(string(b))
	}
	if c.Request.Block {
		// If it is blocked then we generate a pseudo response to the original requester
		resp := openai.ChatCompletion{
			ID:    "1234",
			Model: "kube-gateway",
			Usage: openai.CompletionUsage{CompletionTokens: 0, PromptTokens: 0},
		}
		resp.Choices = append(resp.Choices, openai.ChatCompletionChoice{Message: openai.ChatCompletionMessage{Content: "kube-gateway says no"}, FinishReason: "Stop"})
		newBody, err := json.Marshal(resp)
		if err != nil {
			// Very unlikely this should ever happen
			slog.Error("generating blocking request", "err", err)
		}
		r := http.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Request:    req,
			Header:     make(http.Header),
		}
		r.Header.Add("Content-Type", "application/json")
		r.Header.Add("User-Agent", "kube-gateway")
		r.ContentLength = int64(len(newBody))
		r.Body = io.NopCloser(bytes.NewBuffer(newBody))
		return true, &r, nil

	}
	if len(c.Request.ModelReplace) != 0 {
		for x := range c.Request.ModelReplace {
			if chat.Model == c.Request.ModelReplace[x].Orig {
				slog.Info("changing Model", "original", chat.Model, "replacement", c.Request.ModelReplace[x].New)
				chat.Model = c.Request.ModelReplace[x].New
			}
		}
	}

	for x := range chat.Messages {
		role := "unknown"
		content := ""

		switch {
		case chat.Messages[x].OfUser != nil:

			role = "user"
			if !param.IsOmitted(chat.Messages[x].OfUser.Content.OfString) {
				content = chat.Messages[x].OfUser.Content.OfString.Value
				if len(c.Request.UserPromptReplace) != 0 {
					for y := range c.Request.UserPromptReplace {
						content = strings.ReplaceAll(content, c.Request.UserPromptReplace[y].Orig, c.Request.UserPromptReplace[y].New)
						slog.Info("changing prompt word", "role", role, "original", c.Request.UserPromptReplace[y].Orig, "replacement", c.Request.UserPromptReplace[y].New)
					}
					chat.Messages[x].OfUser.Content.OfString.Value = content // swap the modified prompt
				}
			}
		case chat.Messages[x].OfAssistant != nil:
			role = "assistant"
			if !param.IsOmitted(chat.Messages[x].OfAssistant.Content.OfString) {
				content = chat.Messages[x].OfAssistant.Content.OfString.Value
			}
			// Print tool calls if they exist
			if len(chat.Messages[x].OfAssistant.ToolCalls) > 0 {
				content += "\nTool Calls:"
				for _, toolCall := range chat.Messages[x].OfAssistant.ToolCalls {
					content += fmt.Sprintf("\n- Function: %s", toolCall.Function.Name)
					content += fmt.Sprintf("\n  Arguments: %s", toolCall.Function.Arguments)
				}
			}
		case chat.Messages[x].OfDeveloper != nil:
			role = "developer"
			if !param.IsOmitted(chat.Messages[x].OfDeveloper.Content.OfString) {
				content = chat.Messages[x].OfDeveloper.Content.OfString.Value
			}
		case chat.Messages[x].OfTool != nil:
			role = "tool"
			if !param.IsOmitted(chat.Messages[x].OfTool.Content.OfString) {
				content = chat.Messages[x].OfTool.Content.OfString.Value
			}
		}

		//fmt.Printf("Role: %s\nContent: %s\n\n", role, content)
	}

	newBody, _ := json.Marshal(chat)
	req.ContentLength = int64(len(newBody))
	req.Body = io.NopCloser(bytes.NewBuffer(newBody))
	return false, nil, nil
}

func (c *AITransaction) openAIResponse(body []byte, res *http.Response) (block bool, err error) {
	var chat openai.ChatCompletion
	err = json.Unmarshal(body, &chat)
	if err != nil {
		return false, err
	}

	for x := range chat.Choices {
		for y := range c.Response.BannedWords {
			if strings.Contains(chat.Choices[x].Message.Content, c.Response.BannedWords[y]) {
				block = true
			}
		}
	}
	if block {
		chat.Choices = []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "kube-gateway says no"}, FinishReason: "Stop"}}
	}
	newBody, _ := json.Marshal(chat)
	res.ContentLength = int64(len(newBody))
	res.Body = io.NopCloser(bytes.NewBuffer(newBody))
	return true, nil
}
