package gateway

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"

	"github.com/openai/openai-go"
)

func Http_gateway(ingress, egress net.Conn, c *AITransaction) error {
	// gatewayFunc(input from the application, A destination, the configuration)

	go func() {
		for {
			reader := bufio.NewReader(ingress)
			req, err := http.ReadRequest(reader) // the request is where we aim to do our parsing!
			if err != nil {
				if err == io.EOF {
					return
				}
				slog.Error("reading request", "err", err)
				continue
				//return
			}
			//  fmt.Println(req)

			request := c.GetRequest()
			if request != nil {

				block, resp, err := c.openAIRequest(req)
				if err != nil {
					slog.Error("parse openAI request", "err", err)
					continue
				}
				if block {
					err = resp.Write(ingress)
					if err != nil {
						slog.Error("writing blocking request", "err", err)
					}
					if c.Request.Debug {
						b, _ := httputil.DumpResponse(resp, true)
						fmt.Println(string(b))
					}
					slog.Info("block request", "dest", ingress.RemoteAddr().String())
					continue
				}
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
		res, err := http.ReadResponse(reader, nil) // problem here
		response := c.GetResponse()
		if response != nil {
			if response.Debug {
				b, _ := httputil.DumpResponse(res, true)
				fmt.Println(string(b))
			}
			body, err := io.ReadAll(res.Body)
			if err != nil {
				slog.Error("data read", "err", err)
				if err == io.EOF {
					return err
				}
				return err
			}
			block, err := c.openAIResponse(body, res)
			if block {
				slog.Info("block response", "dest", ingress.RemoteAddr().String())
			}
		}
		if err != nil {
			return fmt.Errorf("Failed reading from remote: %v", err)
		}
		err = res.Write(ingress)
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
