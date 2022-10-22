package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

// ManifestFileChunkPart defines a chunk part within a ManifestFileChunk
type ManifestFileChunkPart struct {
	GUID   string `json:"Guid"`
	Offset string `json:"Offset"`
	Size   string `json:"Size"`

	OffsetInt uint32 `json:"-"`
	SizeInt   uint32 `json:"-"`
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
	ManifestFileVersion  string            `json:"ManifestFileVersion"`
	BIsFileData          bool              `json:"bIsFileData"`
	AppID                string            `json:"AppID"`
	AppNameString        string            `json:"AppNameString"`
	BuildVersionString   string            `json:"BuildVersionString"`
	LaunchExeString      string            `json:"LaunchExeString"`
	LaunchCommand        string            `json:"LaunchCommand"`
	PreReqIds            []string          `json:"PrereqIds"`
	PreReqName           string            `json:"PrereqName"`
	PreReqPath           string            `json:"PrereqPath"`
	PreReqArgs           string            `json:"PrereqArgs"`
	FileManifestList     []ManifestFile    `json:"FileManifestList"`
	ChunkHashList        map[string]string `json:"ChunkHashList"`
	ChunkShaList         map[string]string `json:"ChunkShaList"`
	DataGroupList        map[string]string `json:"DataGroupList"`
	ChunkFilesizeList    map[string]string `json:"ChunkFilesizeList"`
	ChunkFilesizeListInt map[string]uint64 `json:"-"`
	CustomFields         struct{}          `json:"CustomFields"`
}

// Load manifest from a file on disk
func readManifestFile(filename string) (*Manifest, error) {
	// Open file
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileData, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return parseManifest(fileData)
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

	// Parse manifest
	manifest, err = parseManifest(body)
	return
}

func parseManifest(data []byte) (manifest *Manifest, err error) {
	// Parse as json
	if data[0] == '{' {
		err = json.Unmarshal(data, manifest)
		return
	}

	buffer := make([]byte, 4)
	reader := bytes.NewReader(data)

	reader.Read(buffer)
	magic := binary.LittleEndian.Uint32(buffer)
	if magic != 0x44BEC00C {
		err = fmt.Errorf("read invalid magic %d", magic)
		return
	}

	reader.Read(buffer)
	headerSize := binary.LittleEndian.Uint32(buffer)

	reader.Read(buffer)
	uncompressedSize := binary.LittleEndian.Uint32(buffer)

	reader.Read(buffer)
	compressedSize := binary.LittleEndian.Uint32(buffer)

	checksum := make([]byte, 20)
	reader.Read(checksum)

	format, _ := reader.ReadByte()

	reader.Read(buffer)
	//version := binary.LittleEndian.Uint32(buffer)

	if reader.Size()-int64(reader.Len()) != int64(headerSize) {
		err = errors.New("invalid header")
		return
	}

	if reader.Len() != int(compressedSize) {
		err = errors.New("invalid header")
		return
	}

	var decompressed []byte

	if format == 0 {
		decompressed = make([]byte, uncompressedSize)
		reader.Read(decompressed)
	} else if format == 1 {
		decompressor, _ := zlib.NewReader(reader)
		decompressed, _ = ioutil.ReadAll(decompressor)
	} else {
		err = errors.New("invalid format")
		return
	}

	if len(decompressed) != int(uncompressedSize) {
		err = errors.New("invalid data")
		return
	}

	hasher := sha1.New()
	hasher.Write(decompressed)
	if !bytes.Equal(hasher.Sum(nil), checksum) {
		err = errors.New("checksum mismatch")
		return
	}

	reader = bytes.NewReader(decompressed)

	reader.Seek(14, io.SeekCurrent)

	manifest = new(Manifest)
	manifest.ChunkHashList = make(map[string]string)
	manifest.ChunkShaList = make(map[string]string)
	manifest.DataGroupList = make(map[string]string)
	manifest.ChunkFilesizeListInt = make(map[string]uint64)

	manifest.AppNameString = readString(reader)
	manifest.BuildVersionString = readString(reader)
	manifest.LaunchExeString = readString(reader)
	manifest.LaunchCommand = readString(reader)

	reader.Read(buffer)
	if binary.LittleEndian.Uint32(buffer) != 0x00 {
		err = errors.New("fixme: read arrays") // likely [u32 size][element 0][...]
		return
	}

	manifest.PreReqName = readString(reader)
	manifest.PreReqPath = readString(reader)
	manifest.PreReqArgs = readString(reader)

	// chunks
	reader.Seek(5, io.SeekCurrent)

	reader.Read(buffer)
	chunkSize := binary.LittleEndian.Uint32(buffer)

	guids := make(map[int]string)

	guidBuffer := make([]byte, 16)
	for i := 0; i < int(chunkSize); i++ {
		reader.Read(guidBuffer)
		guids[i] = strings.ToUpper(hex.EncodeToString(guidBuffer))
	}

	hashBuffer := make([]byte, 8)
	for i := 0; i < int(chunkSize); i++ {
		reader.Read(hashBuffer)
		manifest.ChunkHashList[guids[i]] = strings.ToUpper(hex.EncodeToString(hashBuffer))
	}

	shaBuffer := make([]byte, 20)
	for i := 0; i < int(chunkSize); i++ {
		reader.Read(shaBuffer)
		manifest.ChunkShaList[guids[i]] = hex.EncodeToString(shaBuffer)
	}

	for i := 0; i < int(chunkSize); i++ {
		n, _ := reader.ReadByte()
		manifest.DataGroupList[guids[i]] = strconv.Itoa(int(n))
	}

	reader.Seek(int64(4*chunkSize), io.SeekCurrent)

	fileSizeBuffer := make([]byte, 8)
	for i := 0; i < int(chunkSize); i++ {
		reader.Read(fileSizeBuffer)
		manifest.ChunkFilesizeListInt[guids[i]] = binary.LittleEndian.Uint64(fileSizeBuffer)
	}

	// files
	reader.Seek(5, io.SeekCurrent)

	reader.Read(buffer)
	fileSize := binary.LittleEndian.Uint32(buffer)

	manifest.FileManifestList = make([]ManifestFile, fileSize)

	for i := 0; i < int(fileSize); i++ {
		manifest.FileManifestList[i].FileName = readString(reader)
	}

	for i := 0; i < int(fileSize); i++ {
		readString(reader)
	}

	for i := 0; i < int(fileSize); i++ {
		reader.Read(shaBuffer)
		manifest.FileManifestList[i].FileHash = hex.EncodeToString(shaBuffer)
	}

	reader.Seek(int64(fileSize), io.SeekCurrent)

	for i := 0; i < int(fileSize); i++ {
		reader.Read(buffer)
		size := binary.LittleEndian.Uint32(buffer)

		manifest.FileManifestList[i].InstallTags = make([]string, size)

		for j := 0; j < int(size); j++ {
			manifest.FileManifestList[i].InstallTags[j] = readString(reader)
		}
	}

	for i := 0; i < int(fileSize); i++ {
		reader.Read(buffer)
		size := binary.LittleEndian.Uint32(buffer)

		manifest.FileManifestList[i].FileChunkParts = make([]ManifestFileChunkPart, size)

		guidBuffer := make([]byte, 16)
		for j := 0; j < int(size); j++ {
			reader.Seek(4, io.SeekCurrent)
			reader.Read(guidBuffer)
			manifest.FileManifestList[i].FileChunkParts[j].GUID = strings.ToUpper(hex.EncodeToString(guidBuffer))

			reader.Read(buffer)
			manifest.FileManifestList[i].FileChunkParts[j].OffsetInt = binary.LittleEndian.Uint32(buffer)
			manifest.FileManifestList[i].FileChunkParts[j].Offset = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(buffer)), 10)

			reader.Read(buffer)
			manifest.FileManifestList[i].FileChunkParts[j].SizeInt = binary.LittleEndian.Uint32(buffer)
			manifest.FileManifestList[i].FileChunkParts[j].Size = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(buffer)), 10)
		}
	}

	return
}

func readString(reader *bytes.Reader) string {
	stringSize := make([]byte, 4)
	reader.Read(stringSize)
	size := binary.LittleEndian.Uint32(stringSize)

	if size == 0 {
		return ""
	}

	stringBuffer := make([]byte, size)
	reader.Read(stringBuffer)

	return string(stringBuffer[:size-1])
}
