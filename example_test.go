package seaweed_test

import (
	"fmt"

	"github.com/lingjhf/seaweed"
)

func ExampleNew() {
	client, err := seaweed.New(seaweed.Config{
		MasterURL:   "http://127.0.0.1:9333",
		FilerURL:    "http://127.0.0.1:8888",
		TUSBasePath: "/.tus",
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(client.Config().TUSBasePath)
	// Output: /.tus
}
