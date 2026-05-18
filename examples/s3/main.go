package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/lingjhf/seaweed"
)

func main() {
	ctx := context.Background()
	client, err := seaweed.New(seaweed.Config{
		MasterURL:       "http://127.0.0.1:9333",
		S3URL:           "http://127.0.0.1:8333",
		AccessKeyID:     "seaweed_admin",
		SecretAccessKey: "seaweed_secret",
		Region:          "us-east-1",
	})
	if err != nil {
		panic(err)
	}

	s3Client, err := client.S3(ctx)
	if err != nil {
		panic(err)
	}

	bucket := "sdk-example"
	key := "hello.txt"
	_, _ = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader("hello seaweedfs s3"),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		panic(err)
	}

	out, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		panic(err)
	}
	defer out.Body.Close()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}
