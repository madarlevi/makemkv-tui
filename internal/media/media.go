// Package media contains the frontend's engine-independent scan model.
package media

import "sort"

type Title struct {
	ID         int
	Attributes map[int]string
	Streams    map[int]map[int]string
	// Optional robot message codes preserve localized stream types. Existing
	// saved plans without these fields continue using the reported type text.
	AttributeCodes map[int]int         `json:",omitempty"`
	StreamCodes    map[int]map[int]int `json:",omitempty"`
}
type Disc struct {
	Count      int
	Attributes map[int]string
	Titles     map[int]*Title
}

func NewDisc() *Disc { return &Disc{Attributes: map[int]string{}, Titles: map[int]*Title{}} }
func (d *Disc) Sorted() []*Title {
	out := []*Title{}
	for _, t := range d.Titles {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
