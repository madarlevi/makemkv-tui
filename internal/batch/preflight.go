package batch

import (
	"fmt"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
)

func (c *Control) preflightISO(i int) error {
	c.mu.Lock()
	c.s.Operation = "Rechecking selected title identities"
	c.mu.Unlock()
	d, e := c.Scan(i)
	if e == nil && d == nil {
		e = fmt.Errorf("engine returned no title information")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	j := &c.s.ISOs[i]
	if e == nil {
		for _, t := range j.Titles {
			if !t.Selected || t.Status == batchstate.Completed {
				continue
			}
			now := d.Titles[t.ID]
			if now == nil {
				e = fmt.Errorf("planned title ID %d disappeared from %s; restore the planned engine/settings", t.ID, j.RelativePath)
				break
			}
			for _, key := range []int{9, 11, 16, 25, 26} {
				if t.Metadata.Attributes[key] != now.Attributes[key] {
					e = fmt.Errorf("title ID %d metadata changed in %s (attribute %d); restore the planned engine/settings rather than extracting a different title", t.ID, j.RelativePath, key)
					break
				}
			}
			if e != nil {
				break
			}
		}
	}
	if e == nil {
		return nil
	}
	if c.ctx.Err() != nil {
		return stopped(c.pauseLocked())
	}
	j.Status = batchstate.Failed
	c.s.Status = batchstate.Failed
	for k := range j.Titles {
		t := &j.Titles[k]
		if t.Selected && t.Status != batchstate.Completed {
			t.Status = batchstate.Failed
			addError(c.s, j, t, e)
		}
	}
	if saveErr := c.save(); saveErr != nil {
		return saveErr
	}
	return e
}
