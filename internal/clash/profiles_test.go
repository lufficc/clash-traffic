package clash

import (
	"os"
	"testing"
)

func TestProfilesLive(t *testing.T) {
	if os.Getenv("CLASH_LIVE") == "" {
		t.Skip("set CLASH_LIVE=1 to read this computer's Clash client profiles")
	}
	ps, err := Profiles(Endpoint{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		t.Logf("%s current=%v type=%s used=%d total=%d expire=%d", p.Name, p.Current, p.Type, p.Upload+p.Download, p.Total, p.Expire)
	}
}
