package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
)

func main() {
	model, found := os.LookupEnv("model")
	if !found {
		model = "llama3.2"
	}
	log.Println("Settings", "model", model)
	//c := http.Client{}
	ctx := context.Background()
	client := openai.NewClient(
		oaioption.WithBaseURL("http://ollama1.ollama:11434/v1"), //ollama is svc // ollama1 is pod
		oaioption.WithAPIKey("ollama"),
	//	oaioption.WithHTTPClient(&c),
	)

	r, err := client.Models.List(ctx, client.Options...)
	if err != nil {
		time.Sleep(time.Minute)
		log.Fatalf("Failed to retieve models: %v", err)
	}
	for x := range r.Data {
		fmt.Printf("Found Model: %s\n", r.Data[x].ID)
	}
	for {
		resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model: model,
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Tell me a joke about Go."),
			},
		})
		if err != nil {
			time.Sleep(time.Minute)
			log.Fatalf("Failed to generate: %v", err)
		}
		log.Println("Response:", resp.Choices[0].Message.Content)
		//c.CloseIdleConnections()
		time.Sleep(time.Second * 15)
	}
}
