package metrics

import "testing"

// The command lines here are trimmed copies of real ones read from /proc on the
// target machine, so the rules are exercised against the shapes they must
// actually survive.
func TestProcessDetail(t *testing.T) {
	tests := []struct {
		name    string
		proc    string
		cmdline []string
		cwd     string
		want    string
	}{
		{
			name:    "chromium renderer",
			proc:    "brave",
			cmdline: []string{"/opt/brave-bin/brave", "--type=renderer", "--renderer-client-id=7", "--lang=en-US"},
			want:    "renderer",
		},
		{
			name:    "chromium extension renderer",
			proc:    "brave",
			cmdline: []string{"/opt/brave-bin/brave", "--type=renderer", "--extension-process", "--lang=en-US"},
			want:    "extension",
		},
		{
			name:    "chromium gpu",
			proc:    "brave",
			cmdline: []string{"/opt/brave-bin/brave", "--type=gpu-process", "--ozone-platform=wayland"},
			want:    "GPU",
		},
		{
			name:    "chromium named utility service",
			proc:    "brave",
			cmdline: []string{"/opt/brave-bin/brave", "--type=utility", "--utility-sub-type=network.mojom.NetworkService"},
			want:    "Network Service",
		},
		{
			name:    "chromium zygote keeps its raw type",
			proc:    "brave",
			cmdline: []string{"/opt/brave-bin/brave", "--type=zygote"},
			want:    "zygote",
		},
		{
			name:    "postgres backend describes itself",
			proc:    "postgres",
			cmdline: []string{"postgres:", "appuser", "appdb", "10.0.0.5(54321)", "idle"},
			want:    "appuser appdb 10.0.0.5(54321) idle",
		},
		{
			name:    "postgres auxiliary process",
			proc:    "postgres",
			cmdline: []string{"postgres:", "autovacuum", "launcher", "", ""},
			want:    "autovacuum launcher",
		},
		{
			name:    "postgres primary has no title to report",
			proc:    "postgres",
			cmdline: []string{"postgres"},
			want:    "",
		},
		{
			name:    "node names its script",
			proc:    "node",
			cmdline: []string{"node", "dist/src/services/worker/nuq-worker.js"},
			want:    "nuq-worker.js",
		},
		{
			name:    "node skips flags before the script",
			proc:    "node",
			cmdline: []string{"node", "--enable-source-maps", "dist/src/harness.js", "--start-docker"},
			want:    "harness.js",
		},
		{
			name:    "cwd identifies an otherwise identical process",
			proc:    "claude",
			cmdline: []string{"claude", "--dangerously-skip-permissions"},
			cwd:     "/home/tester/repositories/hardware-usage",
			want:    "hardware-usage",
		},
		{
			name:    "unknown program falls back to its arguments",
			proc:    "somed",
			cmdline: []string{"/usr/bin/somed", "--config", "/etc/somed.conf"},
			want:    "--config /etc/somed.conf",
		},
		{
			name:    "no cmdline and no cwd yields no label",
			proc:    "kthreadd",
			cmdline: nil,
			want:    "",
		},
		{
			name:    "bare argv0 yields no label",
			proc:    "init",
			cmdline: []string{"/sbin/init"},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProcessDetail(tt.proc, tt.cmdline, tt.cwd); got != tt.want {
				t.Errorf("ProcessDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A browser's startup flags are identical on every instance, so a fallback that
// long identifies nothing and is dropped rather than truncated into noise.
func TestProcessDetailDropsUninformativeLongFallback(t *testing.T) {
	long := make([]string, 0, 40)
	long = append(long, "/usr/bin/weird")
	for i := 0; i < 40; i++ {
		long = append(long, "--a-fairly-long-flag-name")
	}

	if got := ProcessDetail("weird", long, ""); got != "" {
		t.Errorf("ProcessDetail() = %q, want an empty label", got)
	}
}

// The real case this rule exists for: the main Chromium process carries no
// --type=, sits in the user's home, and would otherwise dump ~20 startup flags.
func TestProcessDetailDropsChromiumMainProcessFlags(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	raw := "/opt/brave-bin/brave --ozone-platform=wayland --ozone-platform-hint=wayland " +
		"--enable-features=TouchpadOverscrollHistoryNavigation --disable-features=EyeDropper"

	if got := ProcessDetail("brave", []string{raw}, "/home/tester"); got != "" {
		t.Errorf("ProcessDetail() = %q, want an empty label", got)
	}
}

// A short argument list still identifies an instance and must survive.
func TestProcessDetailKeepsShortFallback(t *testing.T) {
	got := ProcessDetail("myworker", []string{"myworker", "--queue=emails"}, "")
	if got != "--queue=emails" {
		t.Errorf("ProcessDetail() = %q, want %q", got, "--queue=emails")
	}
}

// Chromium's rules must win over the cwd fallback: a browser child's working
// directory is the same for every one of them and would erase the role.
func TestProcessDetailPrefersRoleOverCwd(t *testing.T) {
	got := ProcessDetail("brave",
		[]string{"/opt/brave-bin/brave", "--type=renderer"},
		"/home/tester")
	if got != "renderer" {
		t.Errorf("ProcessDetail() = %q, want %q", got, "renderer")
	}
}

// Chromium writes its command line as ONE space-separated string with no NUL
// separators, so gopsutil returns every argument inside a single slice element.
// Reading it as pre-split arguments silently mislabelled every browser child
// with its sandbox working directory; these cases pin the real shape.
func TestProcessDetailHandlesUnsplitCommandLine(t *testing.T) {
	tests := []struct {
		name string
		proc string
		raw  string
		cwd  string
		want string
	}{
		{
			name: "renderer in a single element",
			proc: "brave",
			raw:  "/opt/brave-bin/brave --type=renderer --renderer-client-id=7 --lang=en-US",
			cwd:  "/proc/17331/fdinfo",
			want: "renderer",
		},
		{
			name: "extension renderer in a single element",
			proc: "brave",
			raw:  "/opt/brave-bin/brave --type=renderer --extension-process --ozone-platform=wayland",
			cwd:  "/proc/17331/fdinfo",
			want: "extension",
		},
		{
			name: "named utility service in a single element",
			proc: "brave",
			raw:  "/opt/brave-bin/brave --type=utility --utility-sub-type=network.mojom.NetworkService",
			cwd:  "/proc/17331/fdinfo",
			want: "Network Service",
		},
		{
			name: "interpreter script in a single element",
			proc: "node",
			raw:  "node dist/src/services/worker/nuq-worker.js",
			want: "nuq-worker.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProcessDetail(tt.proc, []string{tt.raw}, tt.cwd); got != tt.want {
				t.Errorf("ProcessDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A working directory only earns a label when it names something. Sandboxed
// Chromium children sit in /proc/<pid>/fdinfo and would all read "fdinfo".
func TestProcessDetailIgnoresUninformativeCwd(t *testing.T) {
	t.Setenv("HOME", "/home/tester")

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{name: "sandbox fdinfo", cwd: "/proc/17331/fdinfo", want: ""},
		{name: "filesystem root", cwd: "/", want: ""},
		{name: "the user's home itself", cwd: "/home/tester", want: ""},
		{name: "a real project directory", cwd: "/home/tester/repositories/kernl", want: "kernl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A bare argv[0] leaves cwd as the only possible source of a label.
			if got := ProcessDetail("claude", []string{"claude"}, tt.cwd); got != tt.want {
				t.Errorf("ProcessDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}
