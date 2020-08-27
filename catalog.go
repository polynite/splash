package main

import (
	"encoding/json"
	"os"
)

// Catalog defines a catalog
type Catalog struct {
	Elements []struct {
		AppName      string `json:"appName"`
		LabelName    string `json:"labelName"`
		BuildVersion string `json:"buildVersion"`
		Hash         string `json:"hash"`
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
	}

	return ""
}

// Load catalog from a file on disk
func readCatalogFile(filename string) (catalog *Catalog, err error) {
	// Open file
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	// Create new catalog instance
	catalog = new(Catalog)

	// Parse content
	err = json.NewDecoder(file).Decode(catalog)
	return
}

// Parse a catalog from bytes
func parseCatalog(data []byte) (catalog *Catalog, err error) {
	catalog = new(Catalog)

	err = json.Unmarshal(data, catalog)
	return
}
