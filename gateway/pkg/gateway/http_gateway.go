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

	var recieverErr error
	recieverErr = nil //can't remember if this is needed or not.. TODO: find someone cleverer than me
	go func() {
		for {
			defer fmt.Println("errr")

			reader := bufio.NewReader(ingress)
			req, err := http.ReadRequest(reader) // the request is where we aim to do our parsing!
			if err != nil {
				slog.Error(err)
				continue
			}
			// fmt.Println(req)
			body, err := io.ReadAll(req.Body)

			// // // Gross but will do for now
			// bodyCopy, _ := req.GetBody()
			// body := io.NopCloser(req.Body)
			// if err != nil {
			// 	slog.Error(err)
			// 	continue
			// }
			chat := openai.ChatCompletionNewParams{}
			// json.NewDecoder(body).Decode(chat)
			err = json.Unmarshal(body, &chat)
			// b, _ := json.MarshalIndent(chat, "", "   ")
			// fmt.Println(string(b))
			// Overwrite model
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
		if recieverErr != nil {
			return recieverErr
		}
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
