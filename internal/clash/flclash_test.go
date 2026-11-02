package clash

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// flClashSchema is the profiles table of FlClash 0.9.2.
const flClashSchema = `CREATE TABLE profiles (
	id INTEGER NOT NULL, type TEXT NOT NULL, label TEXT NOT NULL,
	current_group_name TEXT NULL, url TEXT NOT NULL, age_secret_key TEXT NULL,
	last_update_date INTEGER NULL, extend_type TEXT NOT NULL, script_id INTEGER NULL,
	match_target TEXT NULL, auto_update_duration_millis INTEGER NOT NULL,
	subscription_info TEXT NULL, auto_update INTEGER NOT NULL,
	selected_map TEXT NOT NULL, unfold_set TEXT NOT NULL, "order" INTEGER NULL,
	overrides TEXT NULL, PRIMARY KEY (id))`

func flClashDB(t *testing.T, rows ...[]any) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Application Support 100%", "com.follow.clash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(flClashSchema); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO profiles (id, type, label, url, last_update_date, subscription_info, "order",
			extend_type, auto_update_duration_millis, auto_update, selected_map, unfold_set)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'standard', 0, 1, '{}', '[]')`, r...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func current(id int64) flClashConfig {
	return flClashConfig{CurrentProfileID: &id}
}

func TestFlClashProfiles(t *testing.T) {
	dir := flClashDB(t,
		[]any{101, "file", "local.yaml", "", nil, nil, 2},
		[]any{102, "url", "机场", "https://example.com/sub", 1760000000, `{"upload":10,"download":20,"total":100,"expire":1770000000}`, 1},
		[]any{103, "url", "", "https://sub.example.org/x?token=1", nil, `not json`, nil},
		[]any{104, "custom", "", "", nil, nil, nil},
	)
	ps, err := flClashProfiles(dir, current(102))
	if err != nil {
		t.Fatal(err)
	}
	want := []Profile{
		{UID: "102", Name: "机场", Type: "remote", Current: true, Upload: 10, Download: 20, Total: 100, Expire: 1770000000, Updated: 1760000000},
		{UID: "101", Name: "local.yaml", Type: "local"},
		{UID: "103", Name: "sub.example.org", Type: "remote"},
		{UID: "104", Name: "104", Type: "local"},
	}
	if len(ps) != len(want) {
		t.Fatalf("got %+v", ps)
	}
	for i := range want {
		if ps[i] != want[i] {
			t.Errorf("profile %d: got %+v, want %+v", i, ps[i], want[i])
		}
	}
}

func TestFlClashProfilesCurrent(t *testing.T) {
	one := flClashDB(t, []any{1, "url", "A", "", nil, nil, nil})
	ps, err := flClashProfiles(one, flClashConfig{})
	if err != nil || len(ps) != 1 || !ps[0].Current {
		t.Fatalf("the only profile is the one in use: %+v, %v", ps, err)
	}
	two := flClashDB(t, []any{1, "url", "A", "", nil, nil, nil}, []any{2, "url", "B", "", nil, nil, nil})
	ps, err = flClashProfiles(two, current(9))
	if err != nil || len(ps) != 2 || ps[0].Current || ps[1].Current {
		t.Fatalf("no profile should be current: %+v, %v", ps, err)
	}
}

func TestFlClashProfilesNoDatabase(t *testing.T) {
	ps, err := flClashProfiles(t.TempDir(), flClashConfig{})
	if ps != nil || err != nil {
		t.Fatalf("got %+v, %v", ps, err)
	}
}

func TestFlClashDatabaseIsReadOnly(t *testing.T) {
	dir := flClashDB(t, []any{1, "url", "A", "", nil, nil, nil})
	db, err := sql.Open("sqlite", flClashDSN(filepath.Join(dir, "database.sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM profiles`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("read: %d, %v", n, err)
	}
	if _, err := db.Exec(`DELETE FROM profiles`); err == nil {
		t.Fatal("the database should be read-only")
	}
}

func TestParseFlClashPrefs(t *testing.T) {
	data := []byte(`{"flutter.version":3,"flutter.config":"{\"currentProfileId\":42,\"patchClashConfig\":{\"external-controller\":\"127.0.0.1:9090\",\"secret\":\"s3\"}}"}`)
	cfg, err := parseFlClashPrefs(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentProfileID == nil || *cfg.CurrentProfileID != 42 || cfg.Patch.Controller != "127.0.0.1:9090" || cfg.Patch.Secret != "s3" {
		t.Fatalf("got %+v", cfg)
	}
	if cfg, err := parseFlClashPrefs([]byte(`{}`)); err != nil || cfg.CurrentProfileID != nil {
		t.Fatalf("no config: %+v, %v", cfg, err)
	}
	if _, err := parseFlClashPrefs([]byte(`{`)); err == nil {
		t.Fatal("a broken file should be an error")
	}
}

func TestParseFlClashPlist(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>flutter.other</key>
	<string>{"currentProfileId":1}</string>
	<key>flutter.config</key>
	<string>{"currentProfileId":42,"patchClashConfig":{"external-controller":"127.0.0.1:9090","secret":"a&amp;b"}}</string>
</dict>
</plist>`)
	cfg, err := parseFlClashPlist(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentProfileID == nil || *cfg.CurrentProfileID != 42 || cfg.Patch.Secret != "a&b" {
		t.Fatalf("got %+v", cfg)
	}
	if cfg, err := parseFlClashPlist([]byte(`<plist><dict/></plist>`)); err != nil || cfg.CurrentProfileID != nil {
		t.Fatalf("no config: %+v, %v", cfg, err)
	}
}

func TestOwnedByFlClash(t *testing.T) {
	var cfg flClashConfig
	cfg.Patch.Secret = "s3"
	if !ownedByFlClash(cfg, Endpoint{Secret: "s3"}) {
		t.Error("the same secret is FlClash's")
	}
	if ownedByFlClash(cfg, Endpoint{Secret: "other"}) {
		t.Error("another secret is not")
	}
	if ownedByFlClash(flClashConfig{}, Endpoint{}) {
		t.Error("no secret proves nothing")
	}
}

// TestProfilesPicksClient reads profiles from a home of both clients.
func TestProfilesPicksClient(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	var dir string
	switch runtime.GOOS {
	case "windows":
		dir = filepath.Join(base, "com.follow", "clash")
	case "darwin":
		dir = filepath.Join(base, "com.follow.clash")
	case "linux":
		dir = filepath.Join(home, ".local", "share", "com.follow.clash")
	default:
		t.Skip("FlClash does not run here")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(flClashDB(t, []any{7, "url", "FlClash 订阅", "", nil, nil, nil}), dir); err != nil {
		t.Fatal(err)
	}
	prefs := `{"flutter.config":"{\"currentProfileId\":7,\"patchClashConfig\":{\"external-controller\":\"127.0.0.1:9090\",\"secret\":\"s3\"}}"}`
	if err := os.WriteFile(filepath.Join(dir, "shared_preferences.json"), []byte(prefs), 0o644); err != nil {
		t.Fatal(err)
	}
	first := func(ep Endpoint) string {
		t.Helper()
		ps, err := Profiles(ep)
		if err != nil || len(ps) == 0 || !ps[0].Current {
			t.Fatalf("got %+v, %v", ps, err)
		}
		return ps[0].Name
	}

	if got := first(Endpoint{}); got != "FlClash 订阅" {
		t.Errorf("FlClash alone: %q", got)
	}

	verge := filepath.Join(base, "io.github.clash-verge-rev.clash-verge-rev")
	if err := os.MkdirAll(verge, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "current: a\nitems:\n  - {uid: a, type: remote, name: Verge 订阅}\n"
	if err := os.WriteFile(filepath.Join(verge, "profiles.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := first(Endpoint{Secret: "other"}); got != "Verge 订阅" {
		t.Errorf("another client's core: %q", got)
	}
	// The macOS defaults are outside the test's home.
	if runtime.GOOS != "darwin" {
		if got := first(Endpoint{Secret: "s3"}); got != "FlClash 订阅" {
			t.Errorf("FlClash's core: %q", got)
		}
		eps := fromFlClash()
		if len(eps) != 1 || eps[0].Address != "http://127.0.0.1:9090" || eps[0].Secret != "s3" {
			t.Errorf("controller: %+v", eps)
		}
	}
}
