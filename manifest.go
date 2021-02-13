package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// ManifestFileChunkPart defines a chunk part within a ManifestFileChunk
type ManifestFileChunkPart struct {
	GUID   string `json:"Guid"`
	Offset string `json:"Offset"`
	Size   string `json:"Size"`
}

// ManifestFile defines a file within a FileManifestList
type ManifestFile struct {
	FileName       string                  `json:"Filename"`
	FileHash       string                  `json:"FileHash"`
	FileChunkParts []ManifestFileChunkPart `json:"FileChunkParts"`
	InstallTags    []string                `json:"InstallTags"`
}

// Manifest defines a manifest
type Manifest struct {
	ManifestFileVersion string            `json:"ManifestFileVersion"`
	BIsFileData         bool              `json:"bIsFileData"`
	AppID               string            `json:"AppID"`
	AppNameString       string            `json:"AppNameString"`
	BuildVersionString  string            `json:"BuildVersionString"`
	LaunchExeString     string            `json:"LaunchExeString"`
	LaunchCommand       string            `json:"LaunchCommand"`
	PreReqIds           []string          `json:"PrereqIds"`
	PreReqName          string            `json:"PrereqName"`
	PreReqPath          string            `json:"PrereqPath"`
	PreReqArgs          string            `json:"PrereqArgs"`
	FileManifestList    []ManifestFile    `json:"FileManifestList"`
	ChunkHashList       map[string]string `json:"ChunkHashList"`
	ChunkShaList        map[string]string `json:"ChunkShaList"`
	DataGroupList       map[string]string `json:"DataGroupList"`
	ChunkFilesizeList   map[string]string `json:"ChunkFilesizeList"`
	CustomFields        struct{}          `json:"CustomFields"`
}

// Load manifest from a file on disk
func readManifestFile(filename string) (*Manifest, error) {
	// Open file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create new manifest instance
	manifest := new(Manifest)

	// Parse content
	if err := json.NewDecoder(file).Decode(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// Fetch manifest from a url
func fetchManifest(url string) (manifest *Manifest, body []byte, err error) {
	// Get manifest
	resp, err := httpClient.Get(url)
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
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	// Create new manifest instance
	manifest = new(Manifest)

	// Parse response body
	err = json.Unmarshal(body, manifest)
	return
}
