package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/openai/openai-go"
	oaioption "github.com/openai/openai-go/option"
)

var client openai.Client

//go:embed public
var embeddedFiles embed.FS

func main() {

	model, found := os.LookupEnv("model")
	if !found {
		model = "llama3.2"
	}
	url, found := os.LookupEnv("url")
	if !found {
		url = "http://ollama1.ollama:11434/v1"
	}

	log.Println("Settings", "model", model, "endpoint", url)
	//c := http.Client{}
	ctx := context.Background()
	client = openai.NewClient(
		oaioption.WithBaseURL(url), //ollama is svc // ollama1 is pod
		oaioption.WithAPIKey("ollama"),
	//	oaioption.WithHTTPClient(&c),
	)

	r, err := client.Models.List(ctx, client.Options...)
	if err != nil {
		time.Sleep(time.Minute)
		log.Fatalf("Failed to retieve models: %v", err)
	}
	if len(r.Data) == 0 {
		log.Print("[ERROR] No models currently present")
	}
	for x := range r.Data {
		fmt.Printf("Found Model: %s\n", r.Data[x].ID)
	}

	publicFS, err := fs.Sub(embeddedFiles, "public")
	if err != nil {
		log.Fatal(err)
	}

	// Create an HTTP file server from the embedded file system
	http.Handle("/", http.FileServer(http.FS(publicFS)))
	http.HandleFunc("/form", FormHandler)

	go func() {
		for {
			time.Sleep(time.Second * 15)
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
		}
	}()

	// Start the HTTP server
	port := ":8000"
	fmt.Printf("Server starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))

	// http.Handle("/",http.FileServer())

}

func FormHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		fmt.Fprintf(w, "ParseForm() err: %v", err)
		return
	}
	message := r.FormValue("message")

	fmt.Fprintf(w, "Your query: %s\n", message)
	resp, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Model: "llama3.2",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(message),
		},
	})
	if err != nil {
		time.Sleep(time.Minute)
		log.Fatalf("Failed to generate: %v", err)
	}
	//log.Println("Response:", resp.Choices[0].Message.Content)

	fmt.Fprintf(w, "\n---------------------- AI Details ----------------------\n")
	fmt.Fprintf(w, "The model: %s\n", resp.Model)
	fmt.Fprintf(w, "Tokens: prompt->%d  completion->%d\n", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	fmt.Fprintf(w, "--------------------------------------------------------\n\n")
	fmt.Fprintf(w, "The generated response: \n%s\n", resp.Choices[0].Message.Content)

}
