// Command gokit-patch keeps the Go runtime patches maintainable.
//
// Go is replaced, not forked: platform/ohos/build.sh hands `go build -overlay`
// a handful of runtime files that live in this repo. That works, but it has two
// failure modes nobody notices until a phone shows a blank screen:
//
//   - a patch quietly stops applying, because upstream renamed the function it
//     hooks (sysargs, load_g, osinit) — the build still succeeds;
//   - the produced .so uses initial-exec TLS, which musl refuses at load time.
//
// This tool answers both without a device:
//
//	go run ./tools/patch check                    # do the patches still fit this Go?
//	go run ./tools/patch overlay -o overlay.json  # the overlay, for any project
//	go run ./tools/patch verify libgocore.so      # is the .so loadable on musl?
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "check":
		err = cmdCheck(os.Args[2:])
	case "overlay":
		err = cmdOverlay(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "gokit-patch: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "gokit-patch: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `gokit-patch: the Go runtime patches this kit carries.

  check [-root DIR]                    do the patches still fit the installed Go?
  overlay [-root DIR] [-o PATH]        write the -overlay JSON (stdout by default)
  verify [-expect a.so,b.so] [-readelf BIN] LIB.SO
                                       dynamic deps, and "no initial-exec TLS"

check and overlay find the repo by walking up to platform/ohos/runtime.
`)
}

// --- check ------------------------------------------------------------------

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	root := fs.String("root", "", "repo root (default: found by walking up)")
	// Check against a Go other than the one on PATH: a toolchain you're about
	// to switch to, say. Default is `go env GOROOT`.
	goroot := fs.String("goroot", "", "Go tree to check against (default: go env GOROOT)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := locateRepo(*root)
	if err != nil {
		return err
	}
	g, err := goEnv("GOROOT", "GOVERSION")
	if err != nil {
		return err
	}
	if *goroot != "" {
		if abs, err := filepath.Abs(*goroot); err == nil {
			g["GOROOT"] = abs
		}
	}

	var problems []string
	warn := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	// 1. Which Go are the patches for?
	want, err := os.ReadFile(filepath.Join(r, "platform/ohos/runtime/VERSION"))
	if err != nil {
		warn("no VERSION next to the patches: %v", err)
	} else if v := versionLine(string(want)); v != g["GOVERSION"] {
		// A warning, not a failure: the patches are small and usually survive.
		fmt.Printf("note: patches were taken from %q, this Go is %s\n", v, g["GOVERSION"])
	}

	// 2. The same fact, in the other file that states it. `go/go.mod` names the
	//    release the patches were taken from as the module's floor, so a toolchain
	//    older than the sources cannot quietly overlay the patches onto something
	//    else. Two files, so they get compared: this pair drifted once (VERSION
	//    said 1.27.3, go.mod said 1.26.0) and nothing said a word.
	if err == nil {
		wanted := strings.TrimPrefix(versionLine(string(want)), "go")
		mod, readErr := os.ReadFile(filepath.Join(r, "go/go.mod"))
		if readErr != nil {
			warn("cannot read go/go.mod: %v", readErr)
		} else if got := goDirective(string(mod)); got != wanted {
			warn("go/go.mod's go directive is %q, but the patches were taken from go%s: "+
				"the floor and the patched sources are the same fact", got, wanted)
		}
	}

	// 3. Every patch still carries its markers, and still has something to hook.
	for _, p := range patches {
		ours := filepath.Join(r, "platform/ohos/runtime", p.file)
		body, err := os.ReadFile(ours)
		if err != nil {
			warn("%s is missing: %v", p.file, err)
			continue
		}
		for _, m := range p.markers {
			if !strings.Contains(string(body), m) {
				warn("%s lost its patch: %s (want %s)", p.file, p.what, m)
			}
		}
		if p.replaces == "" {
			continue // a file of ours, not a replacement: no anchor to check
		}
		upstream, err := os.ReadFile(filepath.Join(g["GOROOT"], "src/runtime", p.replaces))
		if err != nil {
			warn("%s is gone from this Go: it changed how it starts (%v)", p.replaces, err)
			continue
		}
		for _, a := range p.anchors {
			if !strings.Contains(string(upstream), a) {
				warn("%s no longer contains %q: the patch needs re-applying by hand",
					p.replaces, a)
			}
		}
	}

	// 4. The build script must take its overlay from here, or what it builds
	//    isn't what this checked: the patches are only real if they're used.
	build, err := os.ReadFile(filepath.Join(r, "platform/ohos/build.sh"))
	if err != nil {
		warn("cannot read build.sh: %v", err)
	} else if !strings.Contains(string(build), "tools/patch") {
		warn("build.sh doesn't build its overlay with tools/patch: what it compiles isn't what this checked")
	}

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "check: %s\n", p)
		}
		return errors.New("the patches do not fit this Go")
	}
	fmt.Printf("check: %d patches are in place and still fit %s\n", len(patches), g["GOVERSION"])
	return nil
}

// --- overlay ----------------------------------------------------------------

func cmdOverlay(args []string) error {
	fs := flag.NewFlagSet("overlay", flag.ContinueOnError)
	root := fs.String("root", "", "repo root (default: found by walking up)")
	out := fs.String("o", "", "write here (default: stdout)")
	goroot := fs.String("goroot", "", "Go tree to overlay (default: go env GOROOT)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := locateRepo(*root)
	if err != nil {
		return err
	}
	g, err := goEnv("GOROOT")
	if err != nil {
		return err
	}
	if *goroot != "" {
		if abs, err := filepath.Abs(*goroot); err == nil {
			g["GOROOT"] = abs
		}
	}
	b, err := overlayJSON(g["GOROOT"], r)
	if err != nil {
		return err
	}
	if *out == "" {
		_, err = os.Stdout.Write(b)
		return err
	}
	return os.WriteFile(*out, b, 0o644)
}

// overlayJSON is what `go build -overlay` wants: a map from the file Go would
// read to the file it should read instead.
func overlayJSON(goroot, root string) ([]byte, error) {
	replace := map[string]string{}
	for _, p := range patches {
		from := filepath.Join(goroot, "src/runtime", p.replaces)
		if p.replaces == "" {
			// A file Go doesn't have: adding it rather than replacing it. Go's
			// overlay supports that by pointing at a path inside the package.
			from = filepath.Join(goroot, "src/runtime", p.file)
		}
		replace[from] = filepath.Join(root, "platform/ohos/runtime", p.file)
	}
	b, err := json.MarshalIndent(struct {
		Replace map[string]string `json:"Replace"`
	}{replace}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// --- verify -----------------------------------------------------------------

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	readelf := fs.String("readelf", "", "readelf binary (default: the NDK's, else $PATH)")
	expect := fs.String("expect", "libace_napi.z.so,libhilog_ndk.z.so,libc.so",
		"libraries the .so must link, comma separated")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("verify needs exactly one .so")
	}
	lib := fs.Arg(0)
	bin, err := findReadelf(*readelf)
	if err != nil {
		return err
	}
	dyn, err := exec.Command(bin, "-dW", lib).Output()
	if err != nil {
		return fmt.Errorf("readelf -dW %s: %w", lib, err)
	}
	rel, err := exec.Command(bin, "-rW", lib).Output()
	if err != nil {
		return fmt.Errorf("readelf -rW %s: %w", lib, err)
	}

	var problems []string
	if n := countInitialExecTLS(string(rel)); n != 0 {
		problems = append(problems, fmt.Sprintf(
			"%d initial-exec TLS relocation(s): musl refuses to load a dlopen'd library like this", n))
	}
	needed := parseNeeded(string(dyn))
	for _, want := range splitList(*expect) {
		if !slicesContains(needed, want) {
			problems = append(problems, fmt.Sprintf("doesn't link %s (has %s)", want, strings.Join(needed, ", ")))
		}
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "verify: %s\n", p)
		}
		return errors.New(lib + " would not load")
	}
	size, _ := os.Stat(lib)
	fmt.Printf("verify: %s is clean (%.1f MB, links %s, no initial-exec TLS)\n",
		filepath.Base(lib), float64(size.Size())/(1<<20), strings.Join(needed, ", "))
	return nil
}

func findReadelf(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	// The NDK's readelf knows HarmonyOS's ELF details; a plain binutils one
	// reads them fine too, so it's only a fallback.
	if sdk := firstEnv("DEVECO_SDK_HOME"); sdk != "" {
		ndk := filepath.Join(sdk, "default/openharmony/native/llvm/bin/llvm-readelf")
		if _, err := os.Stat(ndk); err == nil {
			return ndk, nil
		}
	}
	for _, name := range []string{"llvm-readelf", "readelf", "eu-readelf"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no readelf found: set -readelf or DEVECO_SDK_HOME")
}

// countInitialExecTLS counts the relocations that make musl refuse a library:
// the whole IE family, not just the 64-bit flavour.
func countInitialExecTLS(readelf string) int {
	n := 0
	for _, line := range strings.Split(readelf, "\n") {
		if strings.Contains(line, "TLS_TPREL") || strings.Contains(line, "TLS_IE") {
			n++
		}
	}
	return n
}

// parseNeeded pulls the DT_NEEDED names out of `readelf -d` output.
func parseNeeded(readelf string) []string {
	var libs []string
	for _, line := range strings.Split(readelf, "\n") {
		if !strings.Contains(line, "NEEDED") {
			continue
		}
		if i := strings.Index(line, "["); i >= 0 {
			if j := strings.Index(line[i:], "]"); j > 0 {
				libs = append(libs, line[i+1:i+j])
			}
		}
	}
	return libs
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func slicesContains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// --- plumbing ---------------------------------------------------------------

// versionLine reads the release out of VERSION, which is a file for humans: the
// bare `go1.x.y` line, surrounded by whatever explanation it deserves.
func versionLine(file string) string {
	for _, line := range strings.Split(file, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 2 && strings.HasPrefix(line, "go") && !strings.ContainsRune(line, ' ') &&
			!strings.Contains(line, "#") {
			return line
		}
	}
	return ""
}

// goDirective is the `go` line of a go.mod file: the minimum toolchain that
// module allows. Comments above it are prose, and a `toolchain` line is advice
// rather than a floor, so neither counts.
func goDirective(mod string) string {
	for _, line := range strings.Split(mod, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "go "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// locateRepo finds the directory holding platform/ohos/runtime, so the tool
// works from anywhere in the tree.
func locateRepo(explicit string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "platform/ohos/runtime")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no platform/ohos/runtime above here: pass -root")
		}
		dir = parent
	}
}

// goEnv reads `go env` values: the tool must talk about the Go that will build
// the library, which isn't necessarily the Go it was compiled with.
func goEnv(names ...string) (map[string]string, error) {
	out, err := exec.Command("go", append([]string{"env"}, names...)...).Output()
	if err != nil {
		return nil, fmt.Errorf("go env: %w (is Go on PATH?)", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(names) {
		return nil, fmt.Errorf("go env: expected %d lines, got %d", len(names), len(lines))
	}
	vals := map[string]string{}
	for i, n := range names {
		vals[n] = strings.TrimSpace(lines[i])
	}
	return vals, nil
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}
