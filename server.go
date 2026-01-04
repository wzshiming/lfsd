package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	contentMediaType = "application/vnd.git-lfs"
	metaMediaType    = "application/vnd.git-lfs+json"
)

// BatchRequest represents a batch API request.
type BatchRequest struct {
	Operation string           `json:"operation"`
	Transfers []string         `json:"transfers,omitempty"`
	Ref       *Ref             `json:"ref,omitempty"`
	Objects   []*ObjectRequest `json:"objects"`
	HashAlgo  string           `json:"hash_algo,omitempty"`
}

// Ref represents a git reference.
type Ref struct {
	Name string `json:"name"`
}

// ObjectRequest represents a single object in a batch request.
type ObjectRequest struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

// BatchResponse represents a batch API response.
type BatchResponse struct {
	Transfer string            `json:"transfer,omitempty"`
	Objects  []*ObjectResponse `json:"objects"`
	HashAlgo string            `json:"hash_algo,omitempty"`
}

// ObjectResponse represents a single object in a batch response.
type ObjectResponse struct {
	Oid           string           `json:"oid"`
	Size          int64            `json:"size"`
	Authenticated bool             `json:"authenticated,omitempty"`
	Actions       map[string]*Link `json:"actions,omitempty"`
	Error         *ObjectError     `json:"error,omitempty"`
}

// Link represents an action link.
type Link struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresIn int               `json:"expires_in,omitempty"`
}

// ObjectError represents an error for a single object.
type ObjectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Message          string `json:"message"`
	RequestID        string `json:"request_id,omitempty"`
	DocumentationURL string `json:"documentation_url,omitempty"`
}

// VerifyRequest represents a verify callback request.
type VerifyRequest struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

// App is the LFS server application.
type App struct {
	config       *Config
	contentStore *ContentStore
	metaStore    *MetaStore
}

// NewApp creates a new App.
func NewApp(config *Config, content *ContentStore, meta *MetaStore) *App {
	return &App{
		config:       config,
		contentStore: content,
		metaStore:    meta,
	}
}

// ServeHTTP implements http.Handler.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Route requests
	switch {
	case r.Method == "POST" && strings.HasSuffix(path, "/objects/batch"):
		a.batchHandler(w, r)
	case r.Method == "POST" && strings.Contains(path, "/verify/"):
		a.verifyHandler(w, r)
	case (r.Method == "GET" || r.Method == "HEAD") && strings.Contains(path, "/objects/"):
		a.downloadHandler(w, r)
	case r.Method == "PUT" && strings.Contains(path, "/objects/"):
		a.uploadHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

// batchHandler handles the batch API.
func (a *App) batchHandler(w http.ResponseWriter, r *http.Request) {
	// Validate content type
	if !isLFSMediaType(r.Header.Get("Accept")) && !isLFSMediaType(r.Header.Get("Content-Type")) {
		a.writeError(w, http.StatusNotAcceptable, "Invalid media type")
		return
	}

	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var objects []*ObjectResponse

	for _, obj := range req.Objects {
		resp := &ObjectResponse{
			Oid:  obj.Oid,
			Size: obj.Size,
		}

		switch req.Operation {
		case "download":
			// Check if object exists in meta store and content store
			_, err := a.metaStore.Get(obj.Oid)
			if err != nil || !a.contentStore.Exists(obj.Oid) {
				resp.Error = &ObjectError{
					Code:    404,
					Message: "Object not found",
				}
			} else {
				resp.Actions = map[string]*Link{
					"download": {
						Href:      a.objectURL(obj.Oid),
						ExpiresIn: 86400,
					},
				}
			}

		case "upload":
			// Check if object already exists
			_, err := a.metaStore.Get(obj.Oid)
			if err == nil && a.contentStore.Exists(obj.Oid) {
				// Object already exists, no action needed
				resp.Authenticated = true
			} else {
				// Store metadata and return upload URL
				a.metaStore.Put(obj.Oid, obj.Size)
				resp.Actions = map[string]*Link{
					"upload": {
						Href:      a.objectURL(obj.Oid),
						ExpiresIn: 86400,
					},
					"verify": {
						Href:      a.verifyURL(obj.Oid),
						ExpiresIn: 86400,
					},
				}
			}

		default:
			resp.Error = &ObjectError{
				Code:    400,
				Message: "Invalid operation",
			}
		}

		objects = append(objects, resp)
	}

	response := &BatchResponse{
		Transfer: "basic",
		Objects:  objects,
		HashAlgo: "sha256",
	}

	w.Header().Set("Content-Type", metaMediaType)
	json.NewEncoder(w).Encode(response)
}

// downloadHandler handles object downloads.
func (a *App) downloadHandler(w http.ResponseWriter, r *http.Request) {
	oid := extractOID(r.URL.Path)
	if oid == "" {
		a.writeError(w, http.StatusBadRequest, "Invalid OID")
		return
	}

	meta, err := a.metaStore.Get(oid)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "Object not found")
		return
	}

	// Support resume download using Range header
	var fromByte int64
	statusCode := http.StatusOK
	if rangeHdr := r.Header.Get("Range"); rangeHdr != "" {
		regex := regexp.MustCompile(`bytes=(\d+)\-.*`)
		match := regex.FindStringSubmatch(rangeHdr)
		if match != nil && len(match) > 1 {
			statusCode = http.StatusPartialContent
			fromByte, _ = strconv.ParseInt(match[1], 10, 64)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", fromByte, meta.Size-1, meta.Size-fromByte))
		}
	}

	content, err := a.contentStore.Get(oid, fromByte)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "Object not found")
		return
	}
	defer content.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	if r.Method == "HEAD" {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size-fromByte, 10))
		w.WriteHeader(statusCode)
		return
	}

	w.WriteHeader(statusCode)
	io.Copy(w, content)
}

// uploadHandler handles object uploads.
func (a *App) uploadHandler(w http.ResponseWriter, r *http.Request) {
	oid := extractOID(r.URL.Path)
	if oid == "" {
		a.writeError(w, http.StatusBadRequest, "Invalid OID")
		return
	}

	meta, err := a.metaStore.Get(oid)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "Object not found in metadata")
		return
	}

	if err := a.contentStore.Put(oid, meta.Size, r.Body); err != nil {
		a.metaStore.Delete(oid)
		a.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
}

// verifyHandler handles verify callbacks.
func (a *App) verifyHandler(w http.ResponseWriter, r *http.Request) {
	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Verify that the object exists and has correct size
	if !a.contentStore.Exists(req.Oid) {
		a.writeError(w, http.StatusNotFound, "Object not found")
		return
	}

	size, err := a.contentStore.Size(req.Oid)
	if err != nil || size != req.Size {
		a.writeError(w, http.StatusBadRequest, "Size mismatch")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// objectURL returns the URL for an object.
func (a *App) objectURL(oid string) string {
	return fmt.Sprintf("%s/objects/%s", a.config.ExternalURL(), oid)
}

// verifyURL returns the verify URL for an object.
func (a *App) verifyURL(oid string) string {
	return fmt.Sprintf("%s/verify/%s", a.config.ExternalURL(), oid)
}

// writeError writes an error response.
func (a *App) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", metaMediaType)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(&ErrorResponse{Message: message})
}

// isLFSMediaType checks if the media type is an LFS media type.
func isLFSMediaType(mediaType string) bool {
	parts := strings.Split(mediaType, ";")
	mt := strings.TrimSpace(parts[0])
	return mt == metaMediaType || mt == contentMediaType
}

// extractOID extracts the OID from a path like /objects/{oid} or /verify/{oid}.
func extractOID(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}
