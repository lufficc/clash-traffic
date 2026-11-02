package clash

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // FlClash keeps its profiles in SQLite
)

// flClashPrefKey is the shared_preferences entry that holds FlClash's
// configuration, as a string of JSON.
const flClashPrefKey = "flutter.config"

// flClashConfig is the part of FlClash's configuration read here.
type flClashConfig struct {
	CurrentProfileID *int64 `json:"currentProfileId"`
	Patch            struct {
		Controller string `json:"external-controller"`
		Secret     string `json:"secret"`
	} `json:"patchClashConfig"`
}

// FlClashDirs returns the data directories of FlClash that exist on this
// machine.
func FlClashDirs() []string {
	cfg, _ := os.UserConfigDir()
	var dirs []string
	switch runtime.GOOS {
	case "windows":
		if cfg != "" {
			dirs = append(dirs, filepath.Join(cfg, "com.follow", "clash"))
		}
	case "darwin":
		if cfg != "" {
			dirs = append(dirs, filepath.Join(cfg, "com.follow.clash"))
		}
	case "linux":
		if data := linuxDataHome(); data != "" {
			dirs = append(dirs, filepath.Join(data, "com.follow.clash"), filepath.Join(data, "FlClash"))
		}
	}
	var found []string
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			found = append(found, d)
		}
	}
	return found
}

// readFlClashConfig reads FlClash's configuration: from a file in its data
// directory, or from the user defaults on macOS.
func readFlClashConfig(dir string) (flClashConfig, error) {
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "export", "com.follow.clash", "-").Output()
		if err != nil {
			return flClashConfig{}, err
		}
		return parseFlClashPlist(out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "shared_preferences.json"))
	if err != nil {
		return flClashConfig{}, err
	}
	return parseFlClashPrefs(data)
}

// parseFlClashPrefs reads the configuration out of shared_preferences.json.
func parseFlClashPrefs(data []byte) (flClashConfig, error) {
	var prefs struct {
		Config *string `json:"flutter.config"`
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return flClashConfig{}, err
	}
	if prefs.Config == nil {
		return flClashConfig{}, nil
	}
	var cfg flClashConfig
	err := json.Unmarshal([]byte(*prefs.Config), &cfg)
	return cfg, err
}

// parseFlClashPlist reads the configuration out of an XML property list.
func parseFlClashPlist(data []byte) (flClashConfig, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	var element, key string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return flClashConfig{}, nil
		}
		if err != nil {
			return flClashConfig{}, err
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			element = tok.Name.Local
		case xml.CharData:
			switch {
			case element == "key":
				key = string(tok)
			case element == "string" && key == flClashPrefKey:
				var cfg flClashConfig
				err := json.Unmarshal(tok, &cfg)
				return cfg, err
			}
		case xml.EndElement:
			element = ""
		}
	}
}

// fromFlClash returns the controller set in FlClash's configuration.
// FlClash leaves its controller closed while it has no secret.
func fromFlClash() []Endpoint {
	var eps []Endpoint
	for _, dir := range FlClashDirs() {
		cfg, err := readFlClashConfig(dir)
		if err != nil || cfg.Patch.Controller == "" || cfg.Patch.Secret == "" {
			continue
		}
		if ep, err := ParseEndpoint(cfg.Patch.Controller, cfg.Patch.Secret); err == nil {
			ep.Source = "FlClash"
			eps = append(eps, ep)
		}
	}
	return eps
}

// ownedByFlClash tells whether ep is the controller of the FlClash with this
// configuration. FlClash generates its secret, so the secret tells it from
// another client on the same port, whichever way the address was written.
func ownedByFlClash(cfg flClashConfig, ep Endpoint) bool {
	return cfg.Patch.Secret != "" && cfg.Patch.Secret == ep.Secret
}

// flClashDSN opens FlClash's database read-only, as FlClash has it open.
func flClashDSN(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/… on Windows
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro&_pragma=busy_timeout(500)"}
	return u.String()
}

// flClashProfiles reads the profiles of the FlClash in dir, the one in use
// first. It returns none when dir has no database.
func flClashProfiles(dir string, cfg flClashConfig) ([]Profile, error) {
	path := filepath.Join(dir, "database.sqlite")
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	db, err := sql.Open("sqlite", flClashDSN(path))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT id, type, label, url, last_update_date, subscription_info
		FROM profiles ORDER BY "order" IS NULL, "order", id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Profile
	current := false
	for rows.Next() {
		var (
			id         int64
			kind       string
			label, src sql.NullString
			updated    any
			info       sql.NullString
		)
		if err := rows.Scan(&id, &kind, &label, &src, &updated, &info); err != nil {
			return nil, err
		}
		p := Profile{UID: strconv.FormatInt(id, 10), Name: flClashLabel(label.String, src.String, id), Type: "local"}
		if kind == "url" {
			p.Type = "remote"
		}
		if cfg.CurrentProfileID != nil && *cfg.CurrentProfileID == id {
			p.Current, current = true, true
		}
		if secs, ok := updated.(int64); ok {
			p.Updated = secs
		}
		if info.Valid {
			var usage struct {
				Upload   int64 `json:"upload"`
				Download int64 `json:"download"`
				Total    int64 `json:"total"`
				Expire   int64 `json:"expire"`
			}
			// Usage that cannot be read leaves the subscription, without it.
			if json.Unmarshal([]byte(info.String), &usage) == nil {
				p.Upload, p.Download, p.Total, p.Expire = usage.Upload, usage.Download, usage.Total, usage.Expire
			}
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The core cannot be running another profile when there is only one.
	if !current && len(out) == 1 {
		out[0].Current = true
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Current && !out[j].Current })
	return out, nil
}

// flClashLabel names a profile as FlClash does: by its label, or else the
// host of its URL, or else its id.
func flClashLabel(label, src string, id int64) string {
	if label != "" {
		return label
	}
	if u, err := url.Parse(src); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return strconv.FormatInt(id, 10)
}
