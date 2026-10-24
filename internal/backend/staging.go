package backend

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Receipt struct {
	Version         int
	Source          string
	TitleID         int
	File            string
	EngineSucceeded bool
}

func stagedName(name string) bool {
	return name != "" && filepath.Base(name) == name && filepath.IsLocal(name) && strings.EqualFold(filepath.Ext(name), ".mkv")
}

// WriteReceipt is called only after a successful process exit and MSG:5005.
// The batch attempt directory is private and already persisted in the plan.
func WriteReceipt(dir, source string, id int, name string) error {
	if !stagedName(name) {
		return fmt.Errorf("unsafe staged filename %q", name)
	}
	r, e := os.OpenRoot(dir)
	if e != nil {
		return e
	}
	defer r.Close()
	if st, e := r.Lstat(name); e != nil {
		return e
	} else if !st.Mode().IsRegular() || st.Size() == 0 {
		return fmt.Errorf("invalid staged file %s", name)
	}
	b, e := json.Marshal(Receipt{1, source, id, name, true})
	if e != nil {
		return e
	}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	temp := ".engine-success-" + hex.EncodeToString(nonce) + ".tmp"
	f, e := r.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	n, e := f.Write(b)
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	} // Retain receipt fragments for diagnosis.
	// A unique receipt filename avoids needing a replace/rename operation on
	// FUSE. A complete JSON receipt is recognized; truncated ones never prove success.
	return batchstate.SyncDir(dir)
}

// ReadReceipt tolerates incomplete receipt fragments but never silently treats
// an existing, complete receipt for a different source/title as success.
func ReadReceipt(dir, source string, id int) (*Receipt, error) {
	r, e := os.OpenRoot(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer r.Close()
	dirFile, e := r.Open(".")
	if e != nil {
		return nil, e
	}
	list, e := dirFile.ReadDir(-1)
	e = errors.Join(e, dirFile.Close())
	if e != nil {
		return nil, e
	}
	fragments := false
	for _, entry := range list {
		if !strings.HasPrefix(entry.Name(), ".engine-success-") || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		fragments = true
		st, e := r.Lstat(entry.Name())
		if e != nil {
			return nil, e
		}
		if !st.Mode().IsRegular() {
			return nil, fmt.Errorf("invalid receipt file %s", entry.Name())
		}
		f, e := r.Open(entry.Name())
		if e != nil {
			return nil, e
		}
		var receipt Receipt
		d := json.NewDecoder(io.LimitReader(f, 64*1024))
		d.DisallowUnknownFields()
		e = d.Decode(&receipt)
		trailing := d.Decode(new(any))
		ce := f.Close()
		if ce != nil {
			return nil, ce
		}
		if e != nil || trailing != io.EOF {
			continue
		}
		if receipt.Version != 1 || receipt.Source != source || receipt.TitleID != id || !receipt.EngineSucceeded || !stagedName(receipt.File) {
			return nil, fmt.Errorf("engine-success receipt does not match planned source/title: %s", filepath.Join(dir, entry.Name()))
		}
		return &receipt, nil
	}
	if fragments {
		return nil, fmt.Errorf("incomplete engine-success receipt in %s; MKV data retained; inspect it before an explicit failed-title retry", dir)
	}
	return nil, nil
}
