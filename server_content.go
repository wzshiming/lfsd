package lfsd

import (
	"fmt"
	"net/http"
	"os"
)

// PutContentHandler receives data from the client and puts it into the content store
func (s *Server) PutContentHandler(w http.ResponseWriter, r *http.Request) {
	rv := unpack(r)
	if err := s.contentStore.Put(rv.Oid, r.Body, r.ContentLength); err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, `{"message":"%s"}`, err)
		return
	}
}

// GetContentHandler gets the content from the content store
func (s *Server) GetContentHandler(w http.ResponseWriter, r *http.Request) {
	rv := unpack(r)
	content, stat, err := s.contentStore.Get(rv.Oid)
	if err != nil {
		writeStatus(w, r, 404)
		return
	}
	defer content.Close()

	http.ServeContent(w, r, "", stat.ModTime(), content)
}

func (s *Server) VerifyHandler(w http.ResponseWriter, r *http.Request) {
	rv := unpack(r)
	info, err := s.contentStore.Info(rv.Oid)
	if err != nil {
		if !os.IsNotExist(err) {
			w.WriteHeader(500)
			fmt.Fprintf(w, `{"message":"%s"}`, err)
			return
		}
		w.WriteHeader(404)
		return
	}

	if info.Size() != rv.Size {
		w.WriteHeader(409)
		fmt.Fprintf(w, `{"message":"size mismatch: expected %d, got %d"}`, rv.Size, info.Size())
		return
	}

	w.WriteHeader(200)
}
