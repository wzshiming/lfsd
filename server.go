package lfsd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	contentMediaType = "application/vnd.git-lfs"
	metaMediaType    = contentMediaType + "+json"
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

// Server is the LFS server application.
type Server struct {
	host         string
	contentStore *ContentStore
}

// NewServer creates a new App.
func NewServer(host string, content *ContentStore) *Server {
	return &Server{
		host:         host,
		contentStore: content,
	}
}

// ServeHTTP implements http.Handler.
func (a *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
func (a *Server) batchHandler(w http.ResponseWriter, r *http.Request) {
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
			if !a.contentStore.Exists(obj.Oid) {
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
			if a.contentStore.Exists(obj.Oid) {
				// Object already exists, no action needed
				resp.Authenticated = true
			} else {
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
func (a *Server) downloadHandler(w http.ResponseWriter, r *http.Request) {
	oid := extractOID(r.URL.Path)
	if oid == "" {
		a.writeError(w, http.StatusBadRequest, "Invalid OID")
		return
	}

	a.contentStore.Download(w, r, oid)
}

// uploadHandler handles object uploads.
func (a *Server) uploadHandler(w http.ResponseWriter, r *http.Request) {
	oid := extractOID(r.URL.Path)
	if oid == "" {
		a.writeError(w, http.StatusBadRequest, "Invalid OID")
		return
	}

	a.contentStore.Upload(w, r, oid)
}

// verifyHandler handles verify callbacks.
func (a *Server) verifyHandler(w http.ResponseWriter, r *http.Request) {
	oid := extractOID(r.URL.Path)
	if oid == "" {
		a.writeError(w, http.StatusBadRequest, "Invalid OID")
		return
	}

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	a.contentStore.Verify(w, r, oid, req.Size)
}

// objectURL returns the URL for an object.
func (a *Server) objectURL(oid string) string {
	return fmt.Sprintf("%s/objects/%s", a.host, oid)
}

// verifyURL returns the verify URL for an object.
func (a *Server) verifyURL(oid string) string {
	return fmt.Sprintf("%s/verify/%s", a.host, oid)
}

// writeError writes an error response.
func (a *Server) writeError(w http.ResponseWriter, status int, message string) {
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
