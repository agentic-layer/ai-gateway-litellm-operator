/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GetWireMockRequestBody fetches WireMock's request journal from the given service
// target and returns the body of the most recent request whose URL contains
// urlFragment.
func GetWireMockRequestBody(target ServiceTarget, urlFragment string) (string, error) {
	body, statusCode, err := MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
		b, _, status, err := GetRequest(baseURL + "/__admin/requests")
		return b, status, err
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch WireMock journal: %w", err)
	}
	if statusCode != 200 {
		return "", fmt.Errorf("WireMock journal returned status %d", statusCode)
	}

	var journal map[string]interface{}
	if err := json.Unmarshal(body, &journal); err != nil {
		return "", fmt.Errorf("failed to parse WireMock journal: %w", err)
	}

	requests, ok := journal["requests"].([]interface{})
	if !ok || len(requests) == 0 {
		return "", fmt.Errorf("WireMock journal contains no requests")
	}

	// WireMock 3.x nests fields under a "request" key.
	for _, entry := range requests {
		e := entry.(map[string]interface{})
		r, ok := e["request"].(map[string]interface{})
		if !ok {
			r = e
		}
		if url, ok := r["url"].(string); ok && strings.Contains(url, urlFragment) {
			if reqBody, ok := r["body"].(string); ok {
				return reqBody, nil
			}
		}
	}
	return "", fmt.Errorf("no request matching %q found in WireMock journal", urlFragment)
}
