package backend

import "testing"

func TestHumanLogKeepsFormattedSpecialPaths(t *testing.T) {
	for line, want := range map[string]string{
		`INFO · MSG:1005,0,1,"Opened /media/Movie%1.iso","Opened %1","/media/Movie%1.iso"`: `INFO: Opened /media/Movie%1.iso`,
		`MSG:1005,0,1,"Opening %1","Opening %1","Évadsorozat, Disc"`:                       `Opening Évadsorozat, Disc`,
		`SINFO:6,1,39,0,"Default"`:   "",
		`FUTURE:6,1,"Unknown"`:       "",
		`ERROR: Could not read disc`: `ERROR: Could not read disc`,
	} {
		if got := DisplayLog(line); got != want {
			t.Errorf("%s => %q want %q", line, got, want)
		}
	}
}
