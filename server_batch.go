package lfsd

import (
	"encoding/json"
	"net/http"
	"os"
)

// BatchHandler provides the batch api
func (s *Server) BatchHandler(w http.ResponseWriter, r *http.Request) {
	bv := unpackBatch(r)

	var responseObjects []*Representation

	// Create a response object
	for _, object := range bv.Objects {
		exists := s.contentStore.Exists(object.Oid)
		if exists { // Object is found and exists
			responseObjects = append(responseObjects, s.Represent(object, true, false))
			continue
		}

		// Object is not found
		if bv.Operation == "upload" {
			responseObjects = append(responseObjects, s.Represent(object, false, true))
		} else {
			rep := &Representation{
				Oid:  object.Oid,
				Size: object.Size,
				Error: &ObjectError{
					Code:    404,
					Message: "Not found",
				},
			}
			responseObjects = append(responseObjects, rep)
		}
	}

	w.Header().Set("Content-Type", metaMediaType)

	respobj := &BatchResponse{Objects: responseObjects}

	enc := json.NewEncoder(w)
	enc.Encode(respobj)

}

// GetMetaHandler retrieves metadata about the object
func (s *Server) GetMetaHandler(w http.ResponseWriter, r *http.Request) {
	rv := unpack(r)
	exists := s.contentStore.Exists(rv.Oid)
	if !exists {
		writeStatus(w, r, 404)
		return
	}

	w.Header().Set("Content-Type", metaMediaType)

	if r.Method == "GET" {
		enc := json.NewEncoder(w)
		enc.Encode(s.Represent(rv, true, false))
	}
}

// CreateMetaHandler instructs the client how to upload data
func (s *Server) CreateMetaHandler(w http.ResponseWriter, r *http.Request) {
	rv := unpack(r)
	info, err := s.contentStore.Info(rv.Oid)
	if err != nil {
		if !os.IsNotExist(err) {
			writeStatus(w, r, 500)
			return
		}
	}

	w.Header().Set("Content-Type", metaMediaType)

	sentStatus := 202
	if info != nil {
		sentStatus = 200
	}
	w.WriteHeader(sentStatus)

	enc := json.NewEncoder(w)
	enc.Encode(s.Represent(rv, info != nil, true))
	// logRequest(r, sentStatus)
}

// Represent takes a RequestVars and Meta and turns it into a Representation suitable
// for json encoding
func (s *Server) Represent(rv *RequestVars, download, upload bool) *Representation {
	rep := &Representation{
		Oid:     rv.Oid,
		Size:    rv.Size,
		Actions: make(map[string]*link),
	}

	header := make(map[string]string)
	verifyHeader := make(map[string]string)

	header["Accept"] = contentMediaType

	if len(rv.Authorization) > 0 {
		header["Authorization"] = rv.Authorization
		verifyHeader["Authorization"] = rv.Authorization
	}

	if download {
		rep.Actions["download"] = &link{Href: rv.objectsLink(), Header: header}
	}

	if upload {
		rep.Actions["upload"] = &link{Href: rv.objectsLink(), Header: header}
		rep.Actions["verify"] = &link{Href: rv.verifyLink(), Header: verifyHeader}
	}
	return rep
}
