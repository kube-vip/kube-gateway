package gateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

func Http_gateway(ingress, egress net.Conn, c *AITransaction) error {

	go func() {
		for {
			reader := bufio.NewReader(ingress)
			req, err := http.ReadRequest(reader) // the request is where we aim to do our parsing!
			if err != nil {
				if err == io.EOF {
					return
				}
				slog.Error("reading request", "err", err)
				return
			}
			//  fmt.Println(req)
			body, err := io.ReadAll(req.Body)

			chat := openai.ChatCompletionNewParams{}

			err = json.Unmarshal(body, &chat)
			request := c.GetRequest()
			if request != nil {
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
						slog.Error("generating locking request", "err", err)
					}
					r := http.Response{
						StatusCode: 200,
						Header:     make(http.Header),
					}
					r.Header.Add("Content-Type", "application/json")
					r.Header.Add("User-Agent", "kube-gateway")
					r.ContentLength = int64(len(newBody))
					r.Body = io.NopCloser(bytes.NewBuffer(newBody))

					err = r.Write(ingress)
					if err != nil {
						slog.Error("writing blocking request", "err", err)
					}

					continue

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
			}
			newBody, _ := json.Marshal(chat)
			//Update header
			req.ContentLength = int64(len(newBody))
			req.Body = io.NopCloser(bytes.NewBuffer(newBody))

			if err != nil {
				slog.Error("data read", "err", err)
				if err == io.EOF {
					return
				}
				return
			}
			err = req.Write(egress)
			if err != nil {
				slog.Error("data write", "err", err)
				return
			}
		}
	}()

	for {

		reader := bufio.NewReader(egress)
		req, err := http.ReadResponse(reader, nil) // problem here
		response := c.GetResponse()
		if response != nil {
			if response.Debug {
				b, _ := httputil.DumpResponse(req, true)
				fmt.Println(string(b))
			}
		}
		if err != nil {
			return fmt.Errorf("Failed reading from remote: %v", err)
		}
		err = req.Write(ingress)
		if err != nil {
			return fmt.Errorf("Writing to local: %v", err)
		}
	}
}

// https://community.openai.com/t/what-exactly-does-a-system-msg-do/459409/2

/*
the user messages are messages that the user wrote
the assistant messages those the bot wrote
the system message is a message that the developer wrote to tell the bot how to interpret the conversation.
They’re supposed to give instructions that can override the rest of the convo, but they’re not always super
reliable depending on what model you’re using.
*/

func debugMessages(Messages []openai.ChatCompletionMessageParamUnion) {

}
