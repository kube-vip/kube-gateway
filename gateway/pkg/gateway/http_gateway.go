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
)

func (c *Config) Http_gateway(ingress, egress net.Conn) error {
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
