package clash

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"clashtraffic/internal/i18n"
)

// Detect finds the controller of the running Clash Verge core: it tries
// the controllers named on the command line of running cores, then those
// of Clash Verge's configuration files, active Windows pipes, and the
// usual defaults.
func Detect(ctx context.Context) (Endpoint, string, error) {
	cands := Candidates()
	var errs []string
	for _, ep := range cands {
		c := NewClient(ep)
		tctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		v, err := c.Version(tctx)
		cancel()
		c.Close()
		if err == nil {
			return ep, v, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", ep, err))
		if ctx.Err() != nil {
			return Endpoint{}, "", ctx.Err()
		}
	}
	if len(cands) == 0 {
		return Endpoint{}, "", errors.New(i18n.T("Clash Verge's controller was not found: make sure Clash Verge is running"))
	}
	return Endpoint{}, "", i18n.Errorf("cannot connect to the Clash controller (tried %d addresses): %s", len(cands), strings.Join(errs, "; "))
}

// Candidates lists the controllers to try, most likely first.
func Candidates() []Endpoint {
	var out []Endpoint
	seen := map[string]bool{}
	add := func(ep Endpoint) {
		if ep.Address == "" || seen[ep.String()] {
			return
		}
		if ep.Kind == KindUnix {
			if runtime.GOOS == "windows" {
				return
			}
			if _, err := os.Stat(ep.Address); err != nil {
				return
			}
		}
		if ep.Kind == KindPipe && runtime.GOOS != "windows" {
			return
		}
		seen[ep.String()] = true
		out = append(out, ep)
	}

	for _, ep := range fromProcesses() {
		add(ep)
	}
	var fileSecret string
	for _, ep := range fromConfigFiles() {
		if ep.Kind == KindHTTP && fileSecret == "" {
			fileSecret = ep.Secret
		}
		add(ep)
	}
	for _, ep := range fromFlClash() {
		if ep.Kind == KindHTTP && fileSecret == "" {
			fileSecret = ep.Secret
		}
		add(ep)
	}
	for _, ep := range fromRunningPipes() {
		add(ep)
	}
	switch runtime.GOOS {
	case "windows":
		add(Endpoint{Kind: KindPipe, Address: `\\.\pipe\verge-mihomo`, Source: "default"})
	default:
		add(Endpoint{Kind: KindUnix, Address: fmt.Sprintf("/var/run/clash-verge-service/users/%d/verge-mihomo.sock", os.Getuid()), Source: "default"})
		add(Endpoint{Kind: KindUnix, Address: "/tmp/verge/verge-mihomo.sock", Source: "default"})
		add(Endpoint{Kind: KindUnix, Address: filepath.Join(os.TempDir(), "verge-mihomo.sock"), Source: "default"})
		add(Endpoint{Kind: KindUnix, Address: "/tmp/verge-mihomo.sock", Source: "default"})
	}
	add(Endpoint{Kind: KindHTTP, Address: "http://127.0.0.1:9097", Secret: fileSecret, Source: "default"})
	add(Endpoint{Kind: KindHTTP, Address: "http://127.0.0.1:9090", Secret: fileSecret, Source: "default"})
	return out
}

// fromProcesses reads the controllers of running mihomo / clash cores off
// their command lines, as Clash Verge starts them with -ext-ctl-unix.
func fromProcesses() []Endpoint {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "axww", "-o", "command=").Output()
	if err != nil {
		return nil
	}
	var eps []Endpoint
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		low := strings.ToLower(line)
		if !strings.Contains(low, "mihomo") && !strings.Contains(low, "clash") {
			continue
		}
		secret := flagValue(line, "-secret")
		if v := flagValue(line, "-ext-ctl-unix"); v != "" {
			eps = append(eps, Endpoint{Kind: KindUnix, Address: v, Source: "the running core"})
		}
		if v := flagValue(line, "-ext-ctl"); v != "" {
			if ep, err := ParseEndpoint(v, secret); err == nil {
				ep.Source = "the running core"
				eps = append(eps, ep)
			}
		}
	}
	return eps
}

// flagValue returns the value after " name " or " name=" in a command
// line, up to the next flag.
func flagValue(line, name string) string {
	i := strings.Index(line, " "+name+" ")
	if i < 0 {
		i = strings.Index(line, " "+name+"=")
	}
	if i < 0 {
		return ""
	}
	v := line[i+len(name)+2:]
	if j := strings.Index(v, " -"); j >= 0 {
		v = v[:j]
	}
	return strings.Trim(strings.TrimSpace(v), `"'`)
}

// linuxDataHome returns the directory of applications' data on Linux.
func linuxDataHome() string {
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		return data
	}
	if home, _ := os.UserHomeDir(); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	return ""
}

// VergeDirs returns the data directories of Clash Verge Rev and Clash
// Verge that exist on this machine.
func VergeDirs() []string {
	home, _ := os.UserHomeDir()
	cfg, _ := os.UserConfigDir()
	ids := []string{"io.github.clash-verge-rev.clash-verge-rev", "io.github.clash-verge-rev.clash-verge-rev.dev"}
	var bases []string
	if cfg != "" {
		bases = append(bases, cfg)
	}
	if runtime.GOOS == "linux" {
		if data := linuxDataHome(); data != "" {
			bases = append(bases, data)
		}
	}
	var dirs []string
	for _, b := range bases {
		for _, id := range ids {
			dirs = append(dirs, filepath.Join(b, id))
		}
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".config", "clash-verge")) // the original Clash Verge
	}
	var found []string
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			found = append(found, d)
		}
	}
	return found
}

// fromConfigFiles reads the controllers of Clash Verge's configuration:
// clash-verge.yaml, the runtime configuration it gives the core, then
// config.yaml, its base.
func fromConfigFiles() []Endpoint {
	var eps []Endpoint
	for _, dir := range VergeDirs() {
		for _, name := range []string{"clash-verge.yaml", "config.yaml"} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			src := filepath.Base(dir) + "/" + name
			secret := yamlScalar(data, "secret")
			if v := yamlScalar(data, "external-controller-unix"); v != "" {
				eps = append(eps, Endpoint{Kind: KindUnix, Address: v, Source: src})
			}
			if v := yamlScalar(data, "external-controller-pipe"); v != "" {
				eps = append(eps, Endpoint{Kind: KindPipe, Address: v, Source: src})
			}
			if v := yamlScalar(data, "external-controller"); v != "" {
				if strings.HasPrefix(v, "0.0.0.0:") || strings.HasPrefix(v, ":") {
					v = "127.0.0.1:" + v[strings.LastIndex(v, ":")+1:]
				}
				if ep, err := ParseEndpoint(v, secret); err == nil {
					ep.Source = src
					eps = append(eps, ep)
				}
			}
		}
	}
	return eps
}

// yamlScalar returns the value of a top-level scalar key of a YAML
// document, which is all the controller settings are.
func yamlScalar(data []byte, key string) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	prefix := key + ":"
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		v := strings.TrimSpace(line[len(prefix):])
		switch {
		case strings.HasPrefix(v, `"`):
			if s, err := strconv.Unquote(v[:strings.LastIndex(v, `"`)+1]); err == nil {
				return s
			}
			return strings.Trim(v, `"`)
		case strings.HasPrefix(v, "'"):
			v = v[1:]
			if j := strings.LastIndex(v, "'"); j >= 0 {
				v = v[:j]
			}
			return strings.ReplaceAll(v, "''", "'")
		}
		if j := strings.Index(v, " #"); j >= 0 {
			v = strings.TrimSpace(v[:j])
		}
		if v == "~" || v == "null" {
			return ""
		}
		return v
	}
	return ""
}
