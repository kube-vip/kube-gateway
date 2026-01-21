package gateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/gookit/slog"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

func (c *AIConfig) Http_gateway(ingress, egress net.Conn) error {
	// We need to create two loops for parsing what is being sent and what is being recieved
	go func() {
		for {
			reader := bufio.NewReader(ingress)
			req, err := http.ReadRequest(reader) // the request is where we aim to do our parsing!
			if err != nil {
				if err == io.EOF {
					return
				}
				slog.Error(err)
				return
			}
			// fmt.Println(req)
			body, err := io.ReadAll(req.Body)

			chat := openai.ChatCompletionNewParams{}
			err = json.Unmarshal(body, &chat)

			if c.Model != "" {
				slog.Infof("Changing Model %s -> %s", chat.Model, c.Model)
				chat.Model = c.Model
			}
			debugMessages(chat.Messages)
			//if len(c.MessageReplacer) != 0 {
			//	replacer := strings.NewReplacer(c.MessageReplacer...)
			// for x := range chat.Messages {
			// 	// y :=
			// 	// unknown := y.String()
			// 	if chat.Messages[x].OfUser != nil {
			// 		if chat.Messages[x].OfUser.Content. {

			// 		}
			// 	}
			// }
			//slog.Infof("Changing Model %s -> %s", chat.Mec.Model)
			//}
			newBody, _ := json.Marshal(chat)
			//Update header
			req.ContentLength = int64(len(newBody))
			req.Body = io.NopCloser(bytes.NewBuffer(newBody))

			if err != nil {
				slog.Errorf("Incoming data read: %v", err)
				if err == io.EOF {
					return
				}
				return
			}
			err = req.Write(egress)
			if err != nil {
				slog.Errorf("Writing to remote: %v", err)
				return
			}
		}
	}()

	for {

		reader := bufio.NewReader(egress)
		req, err := http.ReadResponse(reader, nil) // problem here
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
	for _, msg := range Messages {
		role := "unknown"
		content := ""

		switch {
		case msg.OfUser != nil:
			role = "user"
			if !param.IsOmitted(msg.OfUser.Content.OfString) {
				content = msg.OfUser.Content.OfString.Value
			}
		case msg.OfAssistant != nil:
			role = "assistant"
			if !param.IsOmitted(msg.OfAssistant.Content.OfString) {
				content = msg.OfAssistant.Content.OfString.Value
			}
			// Print tool calls if they exist
			if len(msg.OfAssistant.ToolCalls) > 0 {
				content += "\nTool Calls:"
				for _, toolCall := range msg.OfAssistant.ToolCalls {
					content += fmt.Sprintf("\n- Function: %s", toolCall.Function.Name)
					content += fmt.Sprintf("\n  Arguments: %s", toolCall.Function.Arguments)
				}
			}
		case msg.OfDeveloper != nil:
			role = "developer"
			if !param.IsOmitted(msg.OfDeveloper.Content.OfString) {
				content = msg.OfDeveloper.Content.OfString.Value
			}
		case msg.OfTool != nil:
			role = "tool"
			if !param.IsOmitted(msg.OfTool.Content.OfString) {
				content = msg.OfTool.Content.OfString.Value
			}
		}

		fmt.Printf("Role: %s\nContent: %s\n\n", role, content)
	}
}
