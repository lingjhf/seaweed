package seaweed_test

import (
	"fmt"

	"github.com/lingjhf/seaweed"
)

func ExampleNew() {
	client, err := seaweed.New(seaweed.Config{
		MasterURLs:  []string{"http://127.0.0.1:9333"},
		FilerURLs:   []string{"http://127.0.0.1:8888"},
		TUSBasePath: "/.tus",
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(client.Config().TUSBasePath)
	// Output: /.tus
}

func ExampleNewHTTPClient() {
	httpClient := seaweed.NewHTTPClient(seaweed.HTTPClientConfig{
		MaxIdleConnsPerHost: 64,
	})
	client, err := seaweed.New(seaweed.Config{
		MasterURLs: []string{"http://127.0.0.1:9333"},
	}, seaweed.WithHTTPClient(httpClient))
	if err != nil {
		panic(err)
	}
	defer client.Close()

	fmt.Println(client.Config().MasterURLs[0])
	// Output: http://127.0.0.1:9333
}

func ExampleConfig_endpointPolicy() {
	config := seaweed.Config{
		MasterURLs: []string{
			"http://master-1:9333",
			"http://master-2:9333",
		},
		EndpointPolicy: seaweed.EndpointPolicy{
			Mode: seaweed.EndpointPolicyRoundRobin,
			CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
				Enabled:          true,
				FailureThreshold: 2,
			},
		},
	}

	fmt.Println(config.EndpointPolicy.Mode)
	fmt.Println(config.EndpointPolicy.CircuitBreaker.FailureThreshold)
	// Output:
	// round-robin
	// 2
}
