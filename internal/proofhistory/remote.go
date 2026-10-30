package proofhistory

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type RemoteConfig struct{ URL, Branch, CredentialRef string }
type TransportAuth struct{ Username, Token, SSHKey, KnownHosts string }

var scpRemote = regexp.MustCompile(`^[A-Za-z0-9_.-]+@([A-Za-z0-9.-]+):[^\s]+$`)

func ValidateRemoteURL(raw string) error {
	if strings.ContainsAny(raw, "\r\n\x00") || strings.HasPrefix(raw, "-") {
		return errors.New("invalid remote URL")
	}
	if scpRemote.MatchString(raw) {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "ssh") {
		return errors.New("use an HTTPS or SSH repository URL")
	}
	if u.User != nil {
		if _, ok := u.User.Password(); ok {
			return errors.New("do not put credentials in repository URL")
		}
		if u.Scheme == "https" {
			return errors.New("HTTPS username belongs in authentication settings")
		}
	}
	return nil
}
func (r Repository) Push(ctx context.Context, c RemoteConfig, auth TransportAuth) (string, error) {
	if err := ValidateRemoteURL(c.URL); err != nil {
		return "", err
	}
	if c.Branch != "" && c.Branch != "main" {
		return "", errors.New("unsupported branch")
	}
	if strings.HasPrefix(c.URL, "https:") {
		if auth.Token == "" || auth.Username == "" {
			return "", errors.New("HTTPS authentication is not configured")
		}
	} else if auth.SSHKey == "" || auth.KnownHosts == "" {
		return "", errors.New("SSH authentication is not configured")
	}
	return r.push(ctx, c.URL, auth)
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func (r Repository) push(ctx context.Context, remote string, auth TransportAuth) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	hash, err := r.command(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	args := []string{"-c", "core.hooksPath=" + os.DevNull, "-c", "credential.helper=", "-C", r.Directory, "push", "--", remote, hash + ":refs/heads/main"}
	cmd := newProofhistoryCommand(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if auth.Token != "" {
		dir, err := os.MkdirTemp(r.Directory, ".auth-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)
		helper := filepath.Join(dir, "askpass")
		script := "#!/bin/sh\ncase \"$1\" in *[Uu]sername*) printf '%s\\n' \"$GLANCE_GIT_USERNAME\" ;; *) printf '%s\\n' \"$GLANCE_GIT_TOKEN\" ;; esac\n"
		if runtime.GOOS == "windows" {
			return "", errors.New("HTTPS askpass is not available on this platform")
		}
		if err = os.WriteFile(helper, []byte(script), 0700); err != nil {
			return "", err
		}
		cmd.Env = append(cmd.Env, "GIT_ASKPASS="+helper, "GLANCE_GIT_USERNAME="+auth.Username, "GLANCE_GIT_TOKEN="+auth.Token)
	}
	if auth.SSHKey != "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o UserKnownHostsFile="+shellQuote(auth.KnownHosts)+" -i "+shellQuote(auth.SSHKey))
	}
	// Never expose stderr: servers and credential helpers can echo supplied secrets.
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("push failed (authentication, host trust, or remote divergence): %w", err)
	}
	return hash, nil
}
func SSHHost(raw string) (string, string, error) {
	if match := scpRemote.FindStringSubmatch(raw); match != nil {
		return match[1], "22", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "ssh" || u.Hostname() == "" {
		return "", "", errors.New("invalid SSH URL")
	}
	port := u.Port()
	if port == "" {
		port = "22"
	}
	if _, err = net.LookupPort("tcp", port); err != nil {
		return "", "", err
	}
	return u.Hostname(), port, nil
}

// ScanSSHHost returns untrusted keys and their fingerprint; the caller must obtain explicit trust.
func ScanSSHHost(ctx context.Context, raw, tempRoot string) (string, string, error) {
	host, port, err := SSHHost(raw)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := newProofhistoryCommand(ctx, "ssh-keyscan", "-T", "5", "-p", port, "--", host)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return "", "", errors.New("could not obtain SSH host key")
	}
	f, err := os.CreateTemp(tempRoot, ".host-key-*")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(out); err != nil {
		f.Close()
		return "", "", err
	}
	f.Close()
	fp, err := newProofhistoryCommand(ctx, "ssh-keygen", "-lf", f.Name()).Output()
	if err != nil {
		return "", "", err
	}
	return string(out), string(fp), nil
}
