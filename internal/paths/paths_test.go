package paths

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNormalize_AddsTrailingSeparator(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path semantics")
	}
	cases := map[string]string{
		`C:\temporary_test`:     `C:\temporary_test\`,
		`C:\temporary_test\`:    `C:\temporary_test\`,
		`C:\Users\demo\.ssh`:    `C:\Users\demo\.ssh\`,
		`C:\Users\demo\.ssh\`:   `C:\Users\demo\.ssh\`,
		`C:/Users/demo/.config`: `C:\Users\demo\.config\`,
		`C:\Users\foo\..\demo\`: `C:\Users\demo\`,
	}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil {
			t.Errorf("Normalize(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize_Empty(t *testing.T) {
	if _, err := Normalize(""); err == nil {
		t.Error("expected error for empty path")
	}
	if _, err := Normalize("   "); err == nil {
		t.Error("expected error for whitespace-only path")
	}
}

func TestCheckCollision(t *testing.T) {
	others := []string{
		`C:\Users\demo\.ssh\`,
		`D:\SynologyDrive\`,
	}
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"unrelated path", `C:\Users\demo\.config\`, false},
		{"exact duplicate", `C:\Users\demo\.ssh\`, true},
		{"case-insensitive duplicate (windows)", `C:\Users\demo\.ssh\`, true},
		{"nested inside other", `C:\Users\demo\.ssh\subfolder\`, true},
		{"contains other", `C:\Users\demo\`, true},
		{"different drive root", `D:\Other\`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS != "windows" && strings.Contains(tc.target, `:\`) {
				t.Skip("windows-only path")
			}
			err := CheckCollision(tc.target, others)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckCollision(%q): err=%v wantErr=%v", tc.target, err, tc.wantErr)
			}
		})
	}
}

func TestCheckCollision_IgnoresEmpty(t *testing.T) {
	if err := CheckCollision(`C:\foo\`, []string{"", `D:\bar\`}); err != nil {
		t.Errorf("expected no error with empty entries, got %v", err)
	}
}

func TestCheckCollision_POSIXStyle(t *testing.T) {
	others := []string{
		`/Users/demo/.ssh/`,
		`/Users/demo/SynologyDrive/`,
	}
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"unrelated path", `/Users/demo/.config/`, false},
		{"exact duplicate", `/Users/demo/.ssh/`, true},
		{"nested inside other", `/Users/demo/.ssh/configs/`, true},
		{"contains other", `/Users/demo/`, true},
		{"same prefix is not child", `/Users/demo/.ssh-backup/`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckCollision(tc.target, others)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckCollision(%q): err=%v wantErr=%v", tc.target, err, tc.wantErr)
			}
		})
	}
}

func TestNormalize_RelativePath(t *testing.T) {
	got, err := Normalize(".")
	if err != nil {
		t.Fatalf("Normalize(.) error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
	if !strings.HasSuffix(got, string(filepath.Separator)) {
		t.Errorf("expected trailing separator, got %q", got)
	}
}
