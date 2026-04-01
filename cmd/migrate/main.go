// cmd/migrate/main.go
//
// One-shot migration tool: copies jobs, runs, run-index, and chat history
// from the old agent-daemon.db into the current loom.db.
//
// Usage:
//
//	go run ./cmd/migrate
//	go run ./cmd/migrate --src ~/path/to/agent-daemon.db --dst ~/path/to/loom.db
//
// The tool is safe to run multiple times; existing keys in the destination are
// never overwritten (source wins only for keys absent in the destination).
// The config bucket is intentionally skipped so your current Loom config is
// preserved.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

func defaultPath(app, filename string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filename
	}
	return filepath.Join(home, "Library", "Application Support", app, filename)
}

func main() {
	src := flag.String("src", defaultPath("agent-daemon", "agent-daemon.db"), "source database (old agent-daemon)")
	dst := flag.String("dst", defaultPath("loom", "loom.db"), "destination database (current loom)")
	flag.Parse()

	// Buckets to migrate. "config" is intentionally excluded to preserve the
	// user's current Loom configuration.
	buckets := []string{
		"jobs",
		"runs",
		"runs_by_job",
		"chat",
		// mirror subsystem (present in newer agent-daemon versions)
		"mirror_connectors",
		"mirror_entities",
		"mirror_diffs",
	}

	log.Printf("source: %s", *src)
	log.Printf("dest:   %s", *dst)

	// Open source read-only.
	srcDB, err := bolt.Open(*src, 0600, &bolt.Options{
		Timeout:  3 * time.Second,
		ReadOnly: true,
	})
	if err != nil {
		log.Fatalf("open source db: %v", err)
	}
	defer srcDB.Close()

	// Open destination read-write.
	if err := os.MkdirAll(filepath.Dir(*dst), 0755); err != nil {
		log.Fatalf("create dest dir: %v", err)
	}
	dstDB, err := bolt.Open(*dst, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		log.Fatalf("open dest db: %v", err)
	}
	defer dstDB.Close()

	total := 0
	skipped := 0

	for _, bucket := range buckets {
		copied, skip, err := migrateBucket(srcDB, dstDB, []byte(bucket))
		if err != nil {
			log.Fatalf("migrate bucket %q: %v", bucket, err)
		}
		if copied > 0 || skip > 0 {
			fmt.Printf("  %-22s  %4d copied   %4d skipped (already existed)\n", bucket, copied, skip)
		} else {
			fmt.Printf("  %-22s  (empty or not present in source)\n", bucket)
		}
		total += copied
		skipped += skip
	}

	fmt.Printf("\nDone. %d records copied, %d skipped.\n", total, skipped)
}

// migrateBucket copies every key-value pair from the named bucket in srcDB
// into dstDB. Keys already present in the destination are left untouched.
// Returns (copied, skipped, error).
func migrateBucket(srcDB, dstDB *bolt.DB, bucket []byte) (int, int, error) {
	// Collect all key-value pairs from source.
	type kv struct{ k, v []byte }
	var pairs []kv

	err := srcDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		if b == nil {
			return nil // bucket doesn't exist in source — that's fine
		}
		return b.ForEach(func(k, v []byte) error {
			// Copy slices; bbolt re-uses memory after the transaction.
			kc := make([]byte, len(k))
			vc := make([]byte, len(v))
			copy(kc, k)
			copy(vc, v)
			pairs = append(pairs, kv{kc, vc})
			return nil
		})
	})
	if err != nil {
		return 0, 0, fmt.Errorf("read source: %w", err)
	}
	if len(pairs) == 0 {
		return 0, 0, nil
	}

	copied, skipped := 0, 0
	err = dstDB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		for _, p := range pairs {
			if existing := b.Get(p.k); existing != nil {
				skipped++
				continue
			}
			if err := b.Put(p.k, p.v); err != nil {
				return err
			}
			copied++
		}
		return nil
	})
	return copied, skipped, err
}
