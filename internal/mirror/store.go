package mirror

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Bucket names for the mirror system.
var (
	bucketConnectors = []byte("mirror_connectors")
	bucketEntities   = []byte("mirror_entities")
	bucketMeta       = []byte("mirror_meta")
	bucketChanges    = []byte("mirror_changes")
	bucketConnIdx    = []byte("mirror_conn_idx") // key: connectorID/entityAddress → ""
)

// MirrorStore provides persistence for the mirror system using the same bbolt
// database as the rest of loom. It manages connectors, entities, entity
// metadata, change records, and a connector→entity index.
type MirrorStore struct {
	db *bolt.DB
}

// NewMirrorStore wraps an existing bbolt.DB and ensures the mirror buckets exist.
func NewMirrorStore(db *bolt.DB) (*MirrorStore, error) {
	s := &MirrorStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *MirrorStore) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{
			bucketConnectors, bucketEntities, bucketMeta, bucketChanges, bucketConnIdx,
		} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
}

// ── Connectors ───────────────────────────────────────────────────────────────

// SaveConnector persists a connector. Maintains the connector→entity index.
func (s *MirrorStore) SaveConnector(_ context.Context, conn *Connector) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(conn)
		if err != nil {
			return err
		}
		if err := tx.Bucket(bucketConnectors).Put([]byte(conn.ID), data); err != nil {
			return err
		}
		// Maintain index: connectorID/entityAddress → ""
		if conn.EntityAddress != "" {
			idxKey := []byte(conn.ID + "/" + conn.EntityAddress)
			if err := tx.Bucket(bucketConnIdx).Put(idxKey, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetConnector retrieves a connector by ID.
func (s *MirrorStore) GetConnector(_ context.Context, id string) (*Connector, error) {
	var conn Connector
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketConnectors).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("connector %s not found", id)
		}
		return json.Unmarshal(data, &conn)
	})
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// ListConnectors returns all connectors.
func (s *MirrorStore) ListConnectors(_ context.Context) ([]*Connector, error) {
	var conns []*Connector
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketConnectors).ForEach(func(_, v []byte) error {
			var c Connector
			if err := json.Unmarshal(v, &c); err != nil {
				return err
			}
			conns = append(conns, &c)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(conns, func(i, j int) bool {
		return conns[i].CreatedAt.Before(conns[j].CreatedAt)
	})
	return conns, nil
}

// DeleteConnector removes a connector and its index entries.
func (s *MirrorStore) DeleteConnector(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketConnectors).Delete([]byte(id)); err != nil {
			return err
		}
		// Clean up index entries for this connector
		idx := tx.Bucket(bucketConnIdx)
		prefix := []byte(id + "/")
		c := idx.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			if err := idx.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListConnectorsForEntity returns all connector IDs that write to the given entity address.
func (s *MirrorStore) ListConnectorsForEntity(_ context.Context, address string) ([]string, error) {
	var ids []string
	err := s.db.View(func(tx *bolt.Tx) error {
		suffix := []byte("/" + address)
		return tx.Bucket(bucketConnIdx).ForEach(func(k, _ []byte) error {
			if bytes.HasSuffix(k, suffix) {
				parts := bytes.SplitN(k, []byte("/"), 2)
				if len(parts) >= 1 {
					ids = append(ids, string(parts[0]))
				}
			}
			return nil
		})
	})
	return ids, err
}

// ── Entities ─────────────────────────────────────────────────────────────────

// SaveEntity persists an entity snapshot (the raw data).
func (s *MirrorStore) SaveEntity(_ context.Context, entity *Entity) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(entity)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketEntities).Put([]byte(entity.Address), data)
	})
}

// GetEntity retrieves an entity by address.
func (s *MirrorStore) GetEntity(_ context.Context, address string) (*Entity, error) {
	var entity Entity
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketEntities).Get([]byte(address))
		if data == nil {
			return fmt.Errorf("entity %s not found", address)
		}
		return json.Unmarshal(data, &entity)
	})
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// ListEntities returns all entities, optionally filtered by kind prefix.
// Pass "" to list all. Pass "github.pr/" to list all GitHub PRs.
func (s *MirrorStore) ListEntities(_ context.Context, kindPrefix string) ([]*Entity, error) {
	var entities []*Entity
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketEntities)
		if kindPrefix == "" {
			return b.ForEach(func(_, v []byte) error {
				var e Entity
				if err := json.Unmarshal(v, &e); err != nil {
					return err
				}
				entities = append(entities, &e)
				return nil
			})
		}
		// Prefix scan
		prefix := []byte(kindPrefix)
		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var e Entity
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			entities = append(entities, &e)
		}
		return nil
	})
	return entities, err
}

// DeleteEntity removes an entity and its metadata.
func (s *MirrorStore) DeleteEntity(_ context.Context, address string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		key := []byte(address)
		if err := tx.Bucket(bucketEntities).Delete(key); err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Delete(key)
	})
}

// ── Entity Meta ──────────────────────────────────────────────────────────────

// SaveEntityMeta persists entity metadata.
func (s *MirrorStore) SaveEntityMeta(_ context.Context, meta *EntityMeta) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Put([]byte(meta.Address), data)
	})
}

// GetEntityMeta retrieves entity metadata.
func (s *MirrorStore) GetEntityMeta(_ context.Context, address string) (*EntityMeta, error) {
	var meta EntityMeta
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketMeta).Get([]byte(address))
		if data == nil {
			return fmt.Errorf("entity meta %s not found", address)
		}
		return json.Unmarshal(data, &meta)
	})
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// ── Change Records ───────────────────────────────────────────────────────────

// AppendChange adds an immutable change record to the log.
func (s *MirrorStore) AppendChange(_ context.Context, rec *ChangeRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		// Key: timestamp/address — chronological ordering with entity grouping
		key := []byte(rec.Timestamp.UTC().Format(time.RFC3339Nano) + "/" + rec.Address)
		return tx.Bucket(bucketChanges).Put(key, data)
	})
}

// ListChanges returns recent change records, optionally filtered by entity address prefix.
// Pass "" for all changes. Results are in reverse-chronological order.
func (s *MirrorStore) ListChanges(_ context.Context, addressPrefix string, limit int) ([]*ChangeRecord, error) {
	var records []*ChangeRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketChanges)
		c := b.Cursor()

		// Iterate in reverse (most recent first)
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			if addressPrefix != "" {
				// Key format: "timestamp/address" — extract address part
				parts := bytes.SplitN(k, []byte("/"), 2)
				if len(parts) < 2 || !bytes.HasPrefix(parts[1], []byte(addressPrefix)) {
					continue
				}
			}
			var rec ChangeRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			records = append(records, &rec)
			if limit > 0 && len(records) >= limit {
				break
			}
		}
		return nil
	})
	return records, err
}

// PruneChanges removes change records older than the given duration.
func (s *MirrorStore) PruneChanges(_ context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format(time.RFC3339Nano)
	deleted := 0
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketChanges)
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			// Keys are timestamp-prefixed; stop once we pass the cutoff
			if string(k) >= cutoff {
				break
			}
			if err := b.Delete(k); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	return deleted, err
}