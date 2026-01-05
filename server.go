package lfsd

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
)

const (
	contentMediaType = "application/vnd.git-lfs"
	metaMediaType    = contentMediaType + "+json"
)

var (
	ErrNotOwner = errors.New("Attempt to delete other user's lock")
)

// RequestVars contain variables from the HTTP request. Variables from routing, json body decoding, and
// some headers are stored.
type RequestVars struct {
	Origin string
	Oid    string
	Size   int64

	Repo          string
	Authorization string
}

func (v *RequestVars) objectsLink() string {
	return fmt.Sprintf("%s/objects/%s", v.Origin, v.Oid)
}

func (v *RequestVars) verifyLink() string {
	return fmt.Sprintf("%s/verify/%s", v.Origin, v.Oid)
}

// link provides a structure used to build a hypermedia representation of an HTTP link.
type link struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header,omitempty"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"`
}

// Server links a Router, ContentStore, and MetaStore to provide the LFS server.
type Server struct {
	router       *mux.Router
	contentStore Content
	locksStore   Locks
	authenticate Authenticate
}

type option func(*Server)

func WithContentStore(content Content) option {
	return func(s *Server) {
		s.contentStore = content
	}
}

func WithLocksStore(locks Locks) option {
	return func(s *Server) {
		s.locksStore = locks
	}
}

func WithAuthenticate(auth Authenticate) option {
	return func(s *Server) {
		s.authenticate = auth
	}
}

// NewServer creates a new App using the ContentStore and MetaStore provided
func NewServer(opts ...option) *Server {
	s := &Server{}

	for _, opt := range opts {
		opt(s)
	}

	s.route()
	return s
}

func (s *Server) route() {
	r := mux.NewRouter()

	r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/info/lfs/locks/verify", s.requireAuth(s.BatchHandler)).Methods("POST").MatcherFunc(metaMatcher)

	r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/objects/batch", s.requireAuth(s.BatchHandler)).Methods("POST").MatcherFunc(metaMatcher)

	route := "/{repo:[a-zA-Z0-9/._-]+}/objects/{oid}"
	r.HandleFunc(route, s.requireAuth(s.GetMetaHandler)).Methods("GET", "HEAD").MatcherFunc(metaMatcher)

	r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/objects", s.requireAuth(s.CreateMetaHandler)).Methods("POST").MatcherFunc(metaMatcher)

	r.HandleFunc("/objects/batch", s.requireAuth(s.BatchHandler)).Methods("POST").MatcherFunc(metaMatcher)

	route = "/objects/{oid}"
	r.HandleFunc(route, s.requireAuth(s.GetContentHandler)).Methods("GET", "HEAD").MatcherFunc(contentMatcher)
	r.HandleFunc(route, s.requireAuth(s.PutContentHandler)).Methods("PUT").MatcherFunc(contentMatcher)

	r.HandleFunc("/objects", s.requireAuth(s.CreateMetaHandler)).Methods("POST").MatcherFunc(metaMatcher)

	r.HandleFunc("/verify/{oid}", s.VerifyHandler).Methods("POST")

	if s.locksStore != nil {
		r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/locks", s.requireAuth(s.GetLocksHandler)).Methods("GET").MatcherFunc(metaMatcher)
		r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/locks/verify", s.requireAuth(s.LocksVerifyHandler)).Methods("POST").MatcherFunc(metaMatcher)
		r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/locks", s.requireAuth(s.CreateLockHandler)).Methods("POST").MatcherFunc(metaMatcher)
		r.HandleFunc("/{repo:[a-zA-Z0-9/._-]+}/locks/{id}/unlock", s.requireAuth(s.DeleteLockHandler)).Methods("POST").MatcherFunc(metaMatcher)

		r.HandleFunc("/locks/verify", s.requireAuth(s.LocksVerifyHandler)).Methods("POST").MatcherFunc(metaMatcher)
	}

	s.router = r
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) requireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authenticate == nil {
			h(w, r)
			return
		}

		user, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="git-lfs-server"`)
			writeStatus(w, r, 401)
			return
		}

		name, ret := s.authenticate.Authenticate(user, password)
		if !ret {
			w.Header().Set("WWW-Authenticate", `Basic realm="git-lfs-server"`)
			writeStatus(w, r, 401)
			return
		}

		context.Set(r, "USER", name)
		h(w, r)
	}
}

// contentMatcher provides a mux.MatcherFunc that only allows requests that contain
// an Accept header with the contentMediaType
func contentMatcher(r *http.Request, m *mux.RouteMatch) bool {
	mediaParts := strings.Split(r.Header.Get("Accept"), ";")
	mt := mediaParts[0]
	return mt == contentMediaType
}

// metaMatcher provides a mux.MatcherFunc that only allows requests that contain
// an Accept header with the metaMediaType
func metaMatcher(r *http.Request, m *mux.RouteMatch) bool {
	mediaParts := strings.Split(r.Header.Get("Accept"), ";")
	mt := mediaParts[0]
	return mt == metaMediaType
}

func randomLockId() string {
	var id [20]byte
	rand.Read(id[:])
	return fmt.Sprintf("%x", id[:])
}

func unpack(r *http.Request) *RequestVars {
	vars := mux.Vars(r)
	rv := &RequestVars{
		Repo:          vars["repo"],
		Oid:           vars["oid"],
		Authorization: r.Header.Get("Authorization"),
	}

	if r.Method == "POST" { // Maybe also check if +json
		var p RequestVars
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&p)
		if err != nil {
			return rv
		}

		rv.Oid = p.Oid
		rv.Size = p.Size
	}

	return rv
}

// TODO cheap hack, unify with unpack
func unpackBatch(r *http.Request) *BatchVars {
	vars := mux.Vars(r)

	var bv BatchVars

	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&bv)
	if err != nil {
		return &bv
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
		scheme = fwdProto
	}
	origin := fmt.Sprintf("%s://%s", scheme, r.Host)

	for i := 0; i < len(bv.Objects); i++ {
		bv.Objects[i].Repo = vars["repo"]
		bv.Objects[i].Authorization = r.Header.Get("Authorization")
		bv.Objects[i].Origin = origin
	}

	return &bv
}

func writeStatus(w http.ResponseWriter, r *http.Request, status int) {
	message := http.StatusText(status)

	mediaParts := strings.Split(r.Header.Get("Accept"), ";")
	mt := mediaParts[0]
	if strings.HasSuffix(mt, "+json") {
		message = `{"message":"` + message + `"}`
	}

	w.WriteHeader(status)
	fmt.Fprint(w, message)
}
