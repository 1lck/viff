package sshpath

import "testing"

func TestParseRoundTrip(t *testing.T) {
	cases := []string{
		"ssh://dev/srv/app",
		"ssh://me@dev/srv/app",
		"ssh://me@dev:2222/srv/app",
		"ssh://me@dev/srv/my project",
		"ssh://me@dev/srv/项目",
		"ssh://me@[::1]:2222/srv/app",
		"ssh://dev/",
	}
	for _, in := range cases {
		r, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := r.String(); got != in {
			t.Errorf("round trip %q -> %q", in, got)
		}
	}
}

func TestParseNormalizes(t *testing.T) {
	cases := map[string]string{
		"ssh://ME@DEV.example:22/srv/app/": "ssh://ME@dev.example/srv/app",
		"ssh://me@dev:22/srv//app":         "ssh://me@dev/srv/app",
		"ssh://me@[::1]:22/srv/app":        "ssh://me@[::1]/srv/app",
		"ssh://dev/srv/app//":              "ssh://dev/srv/app",
	}
	for in, want := range cases {
		r, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := r.String(); got != want {
			t.Errorf("Parse(%q).String() = %q, want %q", in, got, want)
		}
	}
}

func TestParseRejects(t *testing.T) {
	bad := []string{
		"", "/srv/app", "https://dev/srv/app", "ssh://", "ssh://dev",
		"ssh://-bad/srv", "ssh://me@/srv", "ssh://me@@dev/srv",
		"ssh://dev:0/srv", "ssh://dev:99999/srv", "ssh://dev:abc/srv",
		"ssh://::1/srv", "ssh://[::1/srv", "ssh://dev/srv\napp", "ssh://user name@dev/srv",
	}
	for _, in := range bad {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", in)
		}
	}
}

func TestHostInput(t *testing.T) {
	cases := []struct {
		in         string
		user, host string
		port       int
	}{
		{"dev", "", "dev", 0},
		{"me@dev", "me", "dev", 0},
		{"dev:2222", "", "dev", 2222},
		{"me@10.0.0.5:22", "me", "10.0.0.5", 22},
		{"me@[::1]:2200", "me", "::1", 2200},
		{"  me@dev  ", "me", "dev", 0},
	}
	for _, c := range cases {
		user, host, port, err := HostInput(c.in)
		if err != nil {
			t.Fatalf("HostInput(%q): %v", c.in, err)
		}
		if user != c.user || host != c.host || port != c.port {
			t.Errorf("HostInput(%q) = %q %q %d, want %q %q %d", c.in, user, host, port, c.user, c.host, c.port)
		}
	}
	for _, in := range []string{"", "-dev", "user name@dev", "dev:", "dev:70000", "dev:+22", "dev: 22", "[::1", "a@b@c"} {
		if _, _, _, err := HostInput(in); err == nil {
			t.Errorf("HostInput(%q) succeeded, want an error", in)
		}
	}
}

func TestURL(t *testing.T) {
	got, err := URL("me@dev:2222", "/srv/my app/")
	if err != nil {
		t.Fatal(err)
	}
	if want := "ssh://me@dev:2222/srv/my app"; got != want {
		t.Errorf("URL() = %q, want %q", got, want)
	}
	if _, err := URL("dev", "relative"); err == nil {
		t.Error("URL accepted a relative path")
	}
	for _, p := range []string{"/srv/../etc", "/srv/app\x01"} {
		if _, err := URL("dev", p); err == nil {
			t.Errorf("URL accepted %q", p)
		}
	}
	got, err = URL("dev", "/")
	if err != nil || got != "ssh://dev/" {
		t.Errorf("URL(dev, /) = %q, %v", got, err)
	}
}

func TestURLOfAndGitPath(t *testing.T) {
	base := "ssh://me@dev:2222/srv/app"
	got, err := URLOf(base, "/srv/app/.git")
	if err != nil {
		t.Fatal(err)
	}
	if want := "ssh://me@dev:2222/srv/app/.git"; got != want {
		t.Errorf("URLOf = %q, want %q", got, want)
	}
	got, err = URLOf(base, `C:\tmp\app`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ssh://me@dev:2222/C:/tmp/app"; got != want {
		t.Errorf("URLOf(Windows) = %q, want %q", got, want)
	}
	p, err := GitPath(base)
	if err != nil || p != "/srv/app" {
		t.Errorf("GitPath = %q, %v", p, err)
	}
	p, err = GitPathFrom(base, "ssh://me@dev:2222/srv/lib")
	if err != nil || p != "/srv/lib" {
		t.Errorf("GitPathFrom same host = %q, %v", p, err)
	}
	if _, err := GitPathFrom(base, "ssh://other/srv/lib"); err == nil {
		t.Error("GitPathFrom accepted another host")
	}
	if _, err := GitPathFrom(base, `C:\tmp\lib`); err == nil {
		t.Error("GitPathFrom accepted a local absolute path")
	}
	if p, err := GitPathFrom(base, "src/main.go"); err != nil || p != "src/main.go" {
		t.Errorf("GitPathFrom relative = %q, %v", p, err)
	}
}

func TestAbsPath(t *testing.T) {
	root := "ssh://me@dev/srv/app"
	got, err := AbsPath(root, "src/main.go")
	if err != nil || got != "/srv/app/src/main.go" {
		t.Fatalf("AbsPath = %q, %v", got, err)
	}
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../../x"} {
		if _, err := AbsPath(root, bad); err == nil {
			t.Errorf("AbsPath accepted %q", bad)
		}
	}
}

func TestDisplayAndName(t *testing.T) {
	r, _ := Parse("ssh://me@dev:2222/srv/my app")
	if got := r.Display(); got != "ssh://me@dev:2222/srv/my app" {
		t.Errorf("Display = %q", got)
	}
	if got := Name("ssh://me@dev/srv/my app"); got != "my app" {
		t.Errorf("Name = %q", got)
	}
	if got := Name("ssh://me@dev/"); got != "dev" {
		t.Errorf("Name(root) = %q", got)
	}
	if got := DisplayPath("ssh://me@dev/srv/app", "src/main.go"); got != "me@dev:/srv/app/src/main.go" {
		t.Errorf("DisplayPath = %q", got)
	}
}

func TestNormalizeIgnoresLocal(t *testing.T) {
	for _, in := range []string{"/srv/app", `C:\repo`, `\\wsl.localhost\Ubuntu\home\me\repo`, "ssh:broken"} {
		if got, ok := Normalize(in); ok {
			t.Errorf("Normalize(%q) = %q, true", in, got)
		}
	}
}
