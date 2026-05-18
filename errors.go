package seaweed

import "github.com/lingjhf/seaweed/internal/httpx"

// Error describes a non-success HTTP response returned by a SeaweedFS API.
type Error = httpx.Error

// APIError describes an API-level error returned in a successful JSON response.
type APIError = httpx.APIError
