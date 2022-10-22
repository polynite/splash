package main

import (
	"encoding/json"
	"net/url"
)

// Catalog defines a catalog
type Catalog struct {
	Elements []struct {
		AppName      string `json:"appName"`
		LabelName    string `json:"labelName"`
		BuildVersion string `json:"buildVersion"`
		Hash         string `json:"hash"`
		UseSignedUrl bool   `json:"useSignedUrl"`
		Manifests    []struct {
			URI         string `json:"uri"`
			QueryParams []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"queryParams,omitempty"`
		} `json:"manifests"`
	} `json:"elements"`
}

// GetManifestURL returns a manifest url
func (c *Catalog) GetManifestURL() string {
	for _, m := range c.Elements[0].Manifests {
		if len(m.QueryParams) == 0 {
			return m.URI
		}

		// Ignore options with multiple query params
		if len(m.QueryParams) > 1 {
			continue
		}

		// Build url
		u, err := url.Parse(m.URI)
		if err == nil {
			// Build query string
			query := u.Query()

			// Add all params
			for _, q := range m.QueryParams {
				query.Set(q.Name, q.Value)
			}

			// Set query
			u.RawQuery, err = url.QueryUnescape(query.Encode())

			if err == nil {
				return u.String()
			}
		}
	}

	return ""
}

// Parse a catalog from bytes
func parseCatalog(data []byte) (catalog *Catalog, err error) {
	catalog = new(Catalog)

	err = json.Unmarshal(data, catalog)
	return
}
