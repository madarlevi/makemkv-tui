package batch

import (
	"context"
	"errors"
	"github.com/madarlevi/makemkv-tui/internal/batchstate"
	"strings"
	"testing"
)

type failSave struct {
	Saver
	completion bool
}

type unavailableState struct{ Saver }

func (unavailableState) Save(*batchstate.State) error {
	return errors.New("state filesystem unavailable")
}

func TestUnpersistedPlanNeverStartsExtraction(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2, 7)
	f := &fakeEngine{}
	if e := NewControl(context.Background(), s, unavailableState{store}, f).Run(); e == nil || len(f.calls) != 0 {
		t.Fatal("extraction started without persisted state", e, f.calls)
	}
}

func (f failSave) Save(s *batchstate.State) error {
	for _, j := range s.ISOs {
		for _, t := range j.Titles {
			if t.Verification != nil && ((f.completion && t.Status == batchstate.Completed) || (!f.completion && t.Status == batchstate.Running)) {
				return errors.New("simulated state disk failure")
			}
		}
	}
	return f.Saver.Save(s)
}
func TestStateWriteFailureStopsQueueAndRecovers(t *testing.T) {
	for _, completion := range []bool{false, true} {
		name := "checkpoint"
		if completion {
			name = "completion"
		}
		t.Run(name, func(t *testing.T) {
			s, store := makePlan(t, []string{"disc.iso"}, 2, 7)
			f := &fakeEngine{}
			c := NewControl(context.Background(), s, failSave{store, completion}, f)
			if e := c.Run(); e == nil || len(f.calls) != 1 {
				t.Fatal("queue continued after state write failed", e, f.calls)
			}
			q := load(t, store)
			if e := Recover(context.Background(), q, store); e != nil {
				t.Fatal(e)
			}
			next := &fakeEngine{}
			if e := NewControl(context.Background(), q, store, next).Run(); e != nil {
				t.Fatal(e)
			}
			// Engine success now survives a failed hash checkpoint as well.
			want := 1
			if len(next.calls) != want {
				t.Fatal("wrong recovery work", next.calls)
			}
		})
	}
}
func TestChangedTitleMappingNeverExtracts(t *testing.T) {
	s, store := makePlan(t, []string{"disc.iso"}, 2)
	s.ISOs[0].Titles[0].Metadata.Attributes[9] = "changed duration"
	store.Save(s)
	f := &fakeEngine{}
	if e := NewControl(context.Background(), s, store, f).Run(); e == nil || !strings.Contains(e.Error(), "metadata changed") || len(f.calls) != 0 {
		t.Fatal(e, f.calls)
	}
	if load(t, store).Status != batchstate.Failed {
		t.Fatal("identity failure not persisted")
	}
}
