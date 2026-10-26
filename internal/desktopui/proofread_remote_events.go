package desktopui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"glance/internal/agentsettings"
	"glance/internal/fileutil"
	"glance/internal/proofhistory"
	"glance/internal/workspace"
)

type proofreadingAuthSettings struct{ Username, TokenRef, SSHKey, KnownHosts string }

func proofreadingAuthRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	root = filepath.Join(root, fileutil.AppDir, "proofreading", "authentication")
	return root, os.MkdirAll(root, 0700)
}
func loadProofreadingAuth(store *workspace.Store) (proofreadingAuthSettings, error) {
	var auth proofreadingAuthSettings
	data, found, err := store.Preference("proofreading.authentication")
	if err == nil && found {
		err = json.Unmarshal([]byte(data), &auth)
	}
	return auth, err
}
func saveProofreadingAuth(store *workspace.Store, auth proofreadingAuthSettings) error {
	data, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	return store.SetPreference("proofreading.authentication", string(data))
}
func (a *App) saveProofreadingToken() {
	w := a.ensureProofreadingWorkbench()
	user, token := strings.TrimSpace(w.authUser.Text()), w.token.Text()
	if user == "" || token == "" || strings.ContainsAny(user+token, "\r\n\x00") {
		w.message = a.textID("proofread.remote.invalid_auth")
		return
	}
	store := a.proofreadingStore()
	if store == nil {
		return
	}
	a.proofreadingWorkbenchJob(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		auth, err := loadProofreadingAuth(store)
		var oldRef string
		if err == nil {
			oldRef = auth.TokenRef
			key := make([]byte, 16)
			_, err = rand.Read(key)
			if err == nil {
				auth.Username = user
				auth.TokenRef = "proofreading-" + hex.EncodeToString(key)
				err = (agentsettings.SystemCredentialStore{}).Put(ctx, auth.TokenRef, token)
			}
			if err == nil {
				err = saveProofreadingAuth(store, auth)
				if err != nil {
					_ = (agentsettings.SystemCredentialStore{}).Delete(ctx, auth.TokenRef)
				} else if oldRef != "" {
					_ = (agentsettings.SystemCredentialStore{}).Delete(ctx, oldRef)
				}
			}
		}
		return func() {
			w.token.SetText("")
			if err != nil {
				w.message = a.textID("proofread.remote.auth_failed")
			} else {
				w.message = a.textID("proofread.remote.auth_saved")
			}
		}
	})
}
func (a *App) saveProofreadingSSH() {
	w := a.ensureProofreadingWorkbench()
	path := strings.TrimSpace(w.ssh.Text())
	store := a.proofreadingStore()
	if store == nil {
		return
	}
	a.proofreadingWorkbenchJob(func() func() {
		info, err := os.Stat(path)
		if err == nil && (!info.Mode().IsRegular() || !filepath.IsAbs(path)) {
			err = errors.New("invalid private key path")
		}
		var auth proofreadingAuthSettings
		if err == nil {
			auth, err = loadProofreadingAuth(store)
		}
		if err == nil {
			auth.SSHKey = path
			root, e := proofreadingAuthRoot()
			err = e
			auth.KnownHosts = filepath.Join(root, "known_hosts")
			if err == nil {
				err = saveProofreadingAuth(store, auth)
			}
		}
		return func() {
			if err != nil {
				w.message = a.textID("proofread.remote.invalid_key")
			} else {
				w.message = a.textID("proofread.remote.auth_saved")
			}
		}
	})
}
func (a *App) generateProofreadingSSH() {
	w := a.ensureProofreadingWorkbench()
	store := a.proofreadingStore()
	if store == nil {
		return
	}
	a.proofreadingWorkbenchJob(func() func() {
		root, err := proofreadingAuthRoot()
		var key, public string
		if err == nil {
			var dir string
			dir, err = os.MkdirTemp(root, "key-")
			if err == nil {
				key = filepath.Join(dir, "id_ed25519")
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				err = exec.CommandContext(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "Glance proofreading", "-f", key).Run()
				cancel()
				if err != nil {
					os.RemoveAll(dir)
				} else {
					data, e := os.ReadFile(key + ".pub")
					err = e
					public = string(data)
				}
			}
		}
		if err == nil {
			auth, e := loadProofreadingAuth(store)
			err = e
			if err == nil {
				auth.SSHKey = key
				auth.KnownHosts = filepath.Join(root, "known_hosts")
				err = saveProofreadingAuth(store, auth)
			}
		}
		return func() {
			if err != nil {
				w.message = a.textID("proofread.remote.auth_failed")
			} else {
				w.ssh.SetText(key)
				w.message = a.textID("proofread.remote.public_key") + "\n" + public
			}
		}
	})
}
func (a *App) saveProofreadingRemote() {
	w := a.ensureProofreadingWorkbench()
	url := strings.TrimSpace(w.remote.Text())
	if err := proofhistory.ValidateRemoteURL(url); err != nil {
		w.message = err.Error()
		return
	}
	store := a.proofreadingStore()
	id := w.historyProject.ID
	a.proofreadingWorkbenchJob(func() func() {
		p, found, err := store.LoadProofreadingProjectMetadata(id)
		if err == nil && !found {
			err = errors.New("project not found")
		}
		if err == nil {
			p.RemoteURL = url
			err = store.SetProofreadingRemote(p.ID, url)
		}
		return func() {
			if err != nil {
				w.message = err.Error()
			} else {
				w.historyProject = p
				w.message = a.textID("proofread.remote.saved")
			}
		}
	})
}
func (a *App) pushProofreadingRemote() {
	w := a.ensureProofreadingWorkbench()
	store := a.proofreadingStore()
	id := w.historyProject.ID
	if w.busy {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	w.cancel = cancel
	a.proofreadingWorkbenchJob(func() func() {
		defer cancel()
		p, found, err := store.LoadProofreadingProjectMetadata(id)
		if err == nil && !found {
			err = errors.New("project not found")
		}
		var settings proofreadingAuthSettings
		var auth proofhistory.TransportAuth
		if err == nil {
			settings, err = loadProofreadingAuth(store)
		}
		if err == nil {
			if strings.HasPrefix(p.RemoteURL, "https:") {
				auth.Username = settings.Username
				auth.Token, err = (agentsettings.SystemCredentialStore{}).Get(ctx, settings.TokenRef)
			} else {
				auth.SSHKey = settings.SSHKey
				auth.KnownHosts = settings.KnownHosts
			}
		}
		var hash string
		if err == nil {
			hash, err = (proofhistory.Repository{Directory: p.Directory}).Push(ctx, proofhistory.RemoteConfig{URL: p.RemoteURL}, auth)
		}
		if err == nil {
			err = store.SetPreference("proofreading.pushed."+p.ID, hash)
		}
		return func() {
			w.cancel = nil
			if err != nil {
				w.message = a.textID("proofread.remote.push_failed")
			} else {
				w.message = a.textID("proofread.remote.pushed") + " · " + hash[:min(10, len(hash))]
				if a.proofreading != nil && a.proofreading.projectDocumentID == p.ID {
					a.proofreading.history.pushed = hash
					a.queueProofreadingDraftPersistence()
				}
			}
		}
	})
}
func (a *App) scanProofreadingHost() {
	w := a.ensureProofreadingWorkbench()
	url := strings.TrimSpace(w.remote.Text())
	a.proofreadingWorkbenchJob(func() func() {
		root, err := proofreadingAuthRoot()
		var keys, fp string
		if err == nil {
			keys, fp, err = proofhistory.ScanSSHHost(context.Background(), url, root)
		}
		return func() {
			if err != nil {
				w.message = a.textID("proofread.remote.host_failed")
			} else {
				w.hostKeys = keys
				w.hostURL = url
				w.message = a.textID("proofread.remote.host_confirm") + "\n" + fp
			}
		}
	})
}
func (a *App) trustProofreadingHost() {
	w := a.ensureProofreadingWorkbench()
	if w.hostKeys == "" || w.hostURL != strings.TrimSpace(w.remote.Text()) {
		return
	}
	keys := w.hostKeys
	store := a.proofreadingStore()
	a.proofreadingWorkbenchJob(func() func() {
		root, err := proofreadingAuthRoot()
		if err == nil {
			var f *os.File
			f, err = os.OpenFile(filepath.Join(root, "known_hosts"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err == nil {
				_, err = f.WriteString(keys)
				f.Close()
			}
		}
		if err == nil {
			auth, e := loadProofreadingAuth(store)
			err = e
			if err == nil {
				auth.KnownHosts = filepath.Join(root, "known_hosts")
				err = saveProofreadingAuth(store, auth)
			}
		}
		return func() {
			w.hostKeys = ""
			if err != nil {
				w.message = a.textID("proofread.remote.host_failed")
			} else {
				w.message = a.textID("proofread.remote.host_trusted")
			}
		}
	})
}
