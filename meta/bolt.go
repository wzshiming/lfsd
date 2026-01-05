package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
	"github.com/wzshiming/lfsd"
)

// boltMeta implements a metadata storage. It stores user credentials and Meta information
// for objects. The storage is handled by boltdb.
type boltMeta struct {
	db *bolt.DB
}

var (
	errNoBucket = errors.New("Bucket not found")
)

var (
	locksBucket = []byte("locks")
)

// NewBolt creates a new MetaStore using the boltdb database at dbFile.
func NewBolt(dbFile string) (lfsd.Locks, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(locksBucket); err != nil {
			return err
		}

		return nil
	})

	return &boltMeta{db: db}, nil
}

// Add write locks to the store for the repo.
func (s *boltMeta) Add(repo string, l ...lfsd.Lock) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(locksBucket)
		if bucket == nil {
			return errNoBucket
		}

		var locks []lfsd.Lock
		data := bucket.Get([]byte(repo))
		if data != nil {
			if err := json.Unmarshal(data, &locks); err != nil {
				return err
			}
		}
		locks = append(locks, l...)
		sort.Sort(LocksByCreatedAt(locks))
		data, err := json.Marshal(&locks)
		if err != nil {
			return err
		}

		return bucket.Put([]byte(repo), data)
	})
	return err
}

// List retrieves locks for the repo from the store
func (s *boltMeta) List(repo string) ([]lfsd.Lock, error) {
	var locks []lfsd.Lock
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(locksBucket)
		if bucket == nil {
			return errNoBucket
		}

		data := bucket.Get([]byte(repo))
		if data != nil {
			if err := json.Unmarshal(data, &locks); err != nil {
				return err
			}
		}
		return nil
	})
	return locks, err
}

// Filtered return filtered locks for the repo
func (s *boltMeta) Filtered(repo, path, cursor, limit string) (locks []lfsd.Lock, next string, err error) {
	locks, err = s.List(repo)
	if err != nil {
		return
	}

	if cursor != "" {
		lastSeen := -1
		for i, l := range locks {
			if l.Id == cursor {
				lastSeen = i
				break
			}
		}

		if lastSeen > -1 {
			locks = locks[lastSeen:]
		} else {
			err = fmt.Errorf("cursor (%s) not found", cursor)
			return
		}
	}

	if path != "" {
		var filtered []lfsd.Lock
		for _, l := range locks {
			if l.Path == path {
				filtered = append(filtered, l)
			}
		}

		locks = filtered
	}

	if limit != "" {
		var size int
		size, err = strconv.Atoi(limit)
		if err != nil || size < 0 {
			locks = make([]lfsd.Lock, 0)
			err = fmt.Errorf("Invalid limit amount: %s", limit)
			return
		}

		size = int(math.Min(float64(size), float64(len(locks))))
		if size+1 < len(locks) {
			next = locks[size].Id
		}
		locks = locks[:size]
	}

	return locks, next, nil
}

// Delete removes lock for the repo by id from the store
func (s *boltMeta) Delete(repo, user, id string, force bool) (*lfsd.Lock, error) {
	var deleted *lfsd.Lock
	err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(locksBucket)
		if bucket == nil {
			return errNoBucket
		}

		var locks []lfsd.Lock
		data := bucket.Get([]byte(repo))
		if data != nil {
			if err := json.Unmarshal(data, &locks); err != nil {
				return err
			}
		}
		newLocks := make([]lfsd.Lock, 0, len(locks))

		var lock lfsd.Lock
		for _, l := range locks {
			if l.Id == id {
				if l.Owner.Name != user && !force {
					return lfsd.ErrNotOwner
				}
				lock = l
			} else if len(l.Id) > 0 {
				newLocks = append(newLocks, l)
			}
		}
		if lock.Id == "" {
			return nil
		}
		deleted = &lock

		if len(newLocks) == 0 {
			return bucket.Delete([]byte(repo))
		}

		data, err := json.Marshal(&newLocks)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repo), data)
	})
	return deleted, err
}

type LocksByCreatedAt []lfsd.Lock

func (c LocksByCreatedAt) Len() int           { return len(c) }
func (c LocksByCreatedAt) Less(i, j int) bool { return c[i].LockedAt.Before(c[j].LockedAt) }
func (c LocksByCreatedAt) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }

// Close closes the underlying boltdb.
func (s *boltMeta) Close() {
	s.db.Close()
}
