package lfsd

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
)

func (s *Server) GetLocksHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repo := vars["repo"]

	enc := json.NewEncoder(w)
	ll := &LockList{}

	w.Header().Set("Content-Type", metaMediaType)

	locks, nextCursor, err := s.locksStore.Filtered(repo,
		r.FormValue("path"),
		r.FormValue("cursor"),
		r.FormValue("limit"))

	if err != nil {
		ll.Message = err.Error()
	} else {
		ll.Locks = locks
		ll.NextCursor = nextCursor
	}

	enc.Encode(ll)

}

func (s *Server) LocksVerifyHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repo := vars["repo"]
	user := getUserFromRequest(r)

	dec := json.NewDecoder(r.Body)
	enc := json.NewEncoder(w)

	w.Header().Set("Content-Type", metaMediaType)

	reqBody := &VerifiableLockRequest{}
	if err := dec.Decode(reqBody); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		enc.Encode(&VerifiableLockList{Message: err.Error()})
		return
	}

	// Limit is optional
	limit := reqBody.Limit
	if limit == 0 {
		limit = 100
	}

	ll := &VerifiableLockList{}
	locks, nextCursor, err := s.locksStore.Filtered(repo, "",
		reqBody.Cursor,
		strconv.Itoa(limit))
	if err != nil {
		ll.Message = err.Error()
	} else {
		ll.NextCursor = nextCursor

		for _, l := range locks {
			if l.Owner.Name == user {
				ll.Ours = append(ll.Ours, l)
			} else {
				ll.Theirs = append(ll.Theirs, l)
			}
		}
	}

	enc.Encode(ll)

}

func (s *Server) CreateLockHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repo := vars["repo"]
	user := getUserFromRequest(r)

	dec := json.NewDecoder(r.Body)
	enc := json.NewEncoder(w)

	w.Header().Set("Content-Type", metaMediaType)

	var lockRequest LockRequest
	if err := dec.Decode(&lockRequest); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		enc.Encode(&LockResponse{Message: err.Error()})
		return
	}

	locks, _, err := s.locksStore.Filtered(repo, lockRequest.Path, "", "1")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		enc.Encode(&LockResponse{Message: err.Error()})
		return
	}
	if len(locks) > 0 {
		w.WriteHeader(http.StatusConflict)
		enc.Encode(&LockResponse{Message: "lock already created"})
		return
	}

	lock := &Lock{
		Id:       randomLockId(),
		Path:     lockRequest.Path,
		Owner:    User{Name: user},
		LockedAt: time.Now(),
	}

	if err := s.locksStore.Add(repo, *lock); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		enc.Encode(&LockResponse{Message: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	enc.Encode(&LockResponse{
		Lock: lock,
	})

}

func (s *Server) DeleteLockHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repo := vars["repo"]
	lockId := vars["id"]
	user := getUserFromRequest(r)

	dec := json.NewDecoder(r.Body)
	enc := json.NewEncoder(w)

	w.Header().Set("Content-Type", metaMediaType)

	var unlockRequest UnlockRequest

	if len(lockId) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		enc.Encode(&UnlockResponse{Message: "invalid lock id"})
		return
	}

	if err := dec.Decode(&unlockRequest); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		enc.Encode(&UnlockResponse{Message: err.Error()})
		return
	}

	l, err := s.locksStore.Delete(repo, user, lockId, unlockRequest.Force)
	if err != nil {
		if err == ErrNotOwner {
			w.WriteHeader(http.StatusForbidden)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		enc.Encode(&UnlockResponse{Message: err.Error()})
		return
	}
	if l == nil {
		w.WriteHeader(http.StatusNotFound)
		enc.Encode(&UnlockResponse{Message: "unable to find lock"})
		return
	}

	enc.Encode(&UnlockResponse{Lock: l})

}

func getUserFromRequest(r *http.Request) string {
	user := context.Get(r, "USER")
	if user == nil {
		return ""
	}
	return user.(string)
}
