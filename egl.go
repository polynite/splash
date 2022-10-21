package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const (
	accountServiceURL  = "https://account-public-service-prod03.ol.epicgames.com"
	launcherServiceURL = "https://launcher-public-service-prod06.ol.epicgames.com"

	eglUserAgent   = "UELauncher/14.2.4-22208432+++Portal+Release-Live Windows/10.0.22000.1.256.64bit"
	eglCredentials = "MzRhMDJjZjhmNDQxNGUyOWIxNTkyMTg3NmRhMzZmOWE6ZGFhZmJjY2M3Mzc3NDUwMzlkZmZlNTNkOTRmYzc2Y2Y="
)

var bearerToken = ""

// Perform OAuth authentication
func authenticate() (token string, err error) {
	// Build form body
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("token_type", "eg1")

	// Create http request
	req, err := http.NewRequest("POST", accountServiceURL+"/account/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return
	}

	// Set headers
	req.Header.Set("User-Agent", eglUserAgent)
	req.Header.Set("Authorization", "basic "+eglCredentials)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Check response code
	if resp.StatusCode != 200 {
		err = fmt.Errorf("invalid status code %d", resp.StatusCode)
		return
	}

	// Parse response
	var respBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		return
	}

	// Set token from response
	token = respBody["access_token"].(string)
	bearerToken = token

	return
}

// Fetch a catalog
func fetchCatalog(platform string, namespace string, item string, app string, label string) (data []byte, err error) {
	// Make sure we are authenticated
	if bearerToken == "" {
		// Attempt to authenticate
		_, err = authenticate()
		if err != nil {
			return
		}
	}

	// Build url
	url := fmt.Sprintf("%s/launcher/api/public/assets/v2/platform/%s/namespace/%s/catalogItem/%s/app/%s/label/%s", launcherServiceURL, platform, namespace, item, app, label)

	// Create http request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	// Set headers
	req.Header.Set("User-Agent", eglUserAgent)
	req.Header.Set("Authorization", "bearer "+bearerToken)

	// Make request
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Check response code
	if resp.StatusCode != 200 {
		err = fmt.Errorf("invalid status code %d", resp.StatusCode)
		return
	}

	// Read body
	data, err = ioutil.ReadAll(resp.Body)

	return
}
