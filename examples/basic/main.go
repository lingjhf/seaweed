package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lingjhf/seaweed"
	"github.com/lingjhf/seaweed/filer"
)

func main() {
	ctx := context.Background()
	client, err := seaweed.New(seaweed.Config{
		MasterURL: "http://127.0.0.1:9333",
		FilerURL:  "http://127.0.0.1:8888",
	})
	if err != nil {
		panic(err)
	}

	_, err = client.Filer().Put(ctx, "/sdk/hello.txt", strings.NewReader("hello seaweedfs"), filer.PutOptions{
		ContentType:   "text/plain",
		ContentLength: int64(len("hello seaweedfs")),
	})
	if err != nil {
		panic(err)
	}

	resp, err := client.Filer().Get(ctx, "/sdk/hello.txt", filer.GetOptions{})
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}
