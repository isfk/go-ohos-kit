package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionLineIgnoresTheComments(t *testing.T) {
	// What the file actually looks like: a paragraph, then the release.
	file := `# The Go release these files were taken from. ` + "`scripts/check.sh`" + ` warns when
# the installed Go has moved on: re-read the three files, re-apply the patch
# (each says what it does at the top), and bump this.
go1.27.3
`
	if got := versionLine(file); got != "go1.27.3" {
		t.Errorf("versionLine = %q, want go1.27.3", got)
	}
	if got := versionLine("go1.28\n"); got != "go1.28" {
		t.Errorf("versionLine = %q, want go1.28", got)
	}
	if got := versionLine("# nothing here\n"); got != "" {
		t.Errorf("versionLine = %q, want empty", got)
	}
}

func TestGoDirectiveReadsPastTheComments(t *testing.T) {
	// What go/go.mod actually looks like: the reason, then the floor. The comment
	// is prose, and a `toolchain` line is advice rather than a floor — neither is
	// the answer to "which Go do the patches need?".
	mod := `module gokit

// The 1.27.3 is not decoration: platform/ohos/runtime/ holds sources taken from
// exactly that release, and the patches are anchored to lines inside them.
go 1.27.3

require modernc.org/sqlite v1.61.1
`
	if got := goDirective(mod); got != "1.27.3" {
		t.Errorf("goDirective = %q, want 1.27.3", got)
	}
	if got := goDirective("go 1.27\ntoolchain go1.28.0\n"); got != "1.27" {
		t.Errorf("goDirective = %q, want 1.27", got)
	}
	if got := goDirective("module x\n"); got != "" {
		t.Errorf("goDirective = %q, want empty", got)
	}
}

func TestOverlayMapsEveryPatchToTheRuntime(t *testing.T) {
	b, err := overlayJSON("/goroot", "/repo")
	if err != nil {
		t.Fatalf("overlayJSON: %v", err)
	}
	var got struct {
		Replace map[string]string `json:"Replace"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("overlay isn't valid JSON: %v", err)
	}
	if len(got.Replace) != len(patches) {
		t.Fatalf("overlay has %d entries, want one per patch (%d)", len(got.Replace), len(patches))
	}
	for _, p := range patches {
		// Go replaces the path it would read; a file we add is named by us.
		want := filepath.Join("/goroot", "src/runtime", p.file)
		if p.replaces != "" {
			want = filepath.Join("/goroot", "src/runtime", p.replaces)
		}
		from := filepath.Join("/repo", "platform/ohos/runtime", p.file)
		if got.Replace[want] != from {
			t.Errorf("overlay[%s] = %q, want %q", want, got.Replace[want], from)
		}
	}
}

func TestCountInitialExecTLS(t *testing.T) {
	// Types are truncated in real output, so the check must match a prefix.
	clean := `Relocation section '.rela.dyn' at offset 0x1 contains 3 entries:
   000000000001 000000000000 R_AARCH64_RELATIVE                    1234
   000000000002 000000000000 R_AARCH64_GLOB_DAT       x + 0
`
	if n := countInitialExecTLS(clean); n != 0 {
		t.Errorf("clean = %d, want 0", n)
	}
	ie := `   000000000001 000000000000 R_AARCH64_TLS_TPREL64     x + 0
   000000000002 000000000000 R_AARCH64_TLS_TPREL64     y + 0
   000000000003 000000000000 R_AARCH64_TLS_TPREL16_LO12 z + 0
`
	if n := countInitialExecTLS(ie); n != 3 {
		t.Errorf("initial-exec = %d, want 3 (the whole family, not just 64)", n)
	}
}

func TestParseNeeded(t *testing.T) {
	dyn := `Dynamic section at offset 0x2 contains 20 entries:
 0x0000000000000001 (NEEDED)             Shared library: [libace_napi.z.so]
 0x0000000000000001 (NEEDED)             Shared library: [libhilog_ndk.z.so]
 0x0000000000000001 (NEEDED)             Shared library: [libc.so]
 0x000000000000000e (SONAME)             Library soname: [libgocore.so]
`
	got := parseNeeded(dyn)
	want := []string{"libace_napi.z.so", "libhilog_ndk.z.so", "libc.so"}
	if len(got) != len(want) {
		t.Fatalf("parseNeeded = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseNeeded[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLocateRepoWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "platform/ohos/runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "tools/patch")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(deep)
	got, err := locateRepo("")
	if err != nil {
		t.Fatalf("locateRepo: %v", err)
	}
	// macOS resolves /var to /private/var, so compare what the filesystem says.
	if real, _ := filepath.EvalSymlinks(got); real != root {
		if realRoot, _ := filepath.EvalSymlinks(root); real != realRoot {
			t.Errorf("locateRepo = %q, want %q", got, root)
		}
	}
}
