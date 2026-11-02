package clash

import (
	"log"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Profile is a configuration of the Clash client: a subscription when it is
// remote, with the usage its provider reports.
type Profile struct {
	UID      string
	Name     string
	Type     string // "remote" or "local"
	Current  bool
	Upload   int64
	Download int64
	Total    int64
	Expire   int64 // unix seconds
	Updated  int64 // unix seconds
}

type profilesFile struct {
	Current string `yaml:"current"`
	Items   []struct {
		UID     string `yaml:"uid"`
		Type    string `yaml:"type"`
		Name    string `yaml:"name"`
		Updated int64  `yaml:"updated"`
		Extra   *struct {
			Upload   int64 `yaml:"upload"`
			Download int64 `yaml:"download"`
			Total    int64 `yaml:"total"`
			Expire   int64 `yaml:"expire"`
		} `yaml:"extra"`
	} `yaml:"items"`
}

// Profiles reads the configurations of the client whose core is at ep, the
// one in use first: FlClash when ep is its controller, else Clash Verge, else
// any FlClash. It returns none when neither is installed.
func Profiles(ep Endpoint) ([]Profile, error) {
	flclash := FlClashDirs()
	configs := make(map[string]flClashConfig, len(flclash))
	for _, dir := range flclash {
		cfg, err := readFlClashConfig(dir)
		if err != nil {
			log.Println("FlClash config:", err)
		}
		configs[dir] = cfg
		if ownedByFlClash(cfg, ep) {
			return flClashProfiles(dir, cfg)
		}
	}
	out, err := vergeProfiles()
	if err != nil || len(out) > 0 {
		return out, err
	}
	for _, dir := range flclash {
		out, err := flClashProfiles(dir, configs[dir])
		if err != nil || len(out) > 0 {
			return out, err
		}
	}
	return nil, nil
}

// vergeProfiles reads the configurations of Clash Verge from profiles.yaml.
func vergeProfiles() ([]Profile, error) {
	for _, dir := range VergeDirs() {
		data, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
		if err != nil {
			continue
		}
		var f profilesFile
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, err
		}
		var out []Profile
		for _, it := range f.Items {
			// Merges, scripts and rule sets enhance profiles; they are none.
			if (it.Type != "remote" && it.Type != "local") || it.Name == "" {
				continue
			}
			p := Profile{UID: it.UID, Name: it.Name, Type: it.Type, Current: it.UID == f.Current, Updated: it.Updated}
			if it.Extra != nil {
				p.Upload, p.Download, p.Total, p.Expire = it.Extra.Upload, it.Extra.Download, it.Extra.Total, it.Extra.Expire
			}
			out = append(out, p)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Current && !out[j].Current })
		return out, nil
	}
	return nil, nil
}
