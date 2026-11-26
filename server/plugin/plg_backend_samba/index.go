package plg_backend_samba

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hirochachacha/go-smb2"
	. "github.com/mickael-kerjean/filestash/server/common"
)

var SambaCache AppCache

func init() {
	Backend.Register("samba", Samba{})

	Hooks.Register.Onload(func() {
		Config.Get("samba.healthcheck").Schema(func(f *FormElement) *FormElement {
			if f == nil {
				f = &FormElement{}
			}
			f.Id = "samba_healthcheck"
			f.Name = "healthcheck"
			f.Type = "boolean"
			f.Default = true
			f.Description = "Probe idle pooled SMB sessions and reconnect stale ones"
			return f
		})
		Config.Get("samba.ls_cache_seconds").Schema(func(f *FormElement) *FormElement {
			if f == nil {
				f = &FormElement{}
			}
			f.Id = "samba_ls_cache_seconds"
			f.Name = "ls_cache_seconds"
			f.Type = "number"
			f.Default = defaultLsCacheSeconds
			f.Description = "Seconds to cache directory listings per session (0 disables)"
			return f
		})
	})

	SambaCache = NewAppCache()
	SambaCache.OnEvict(func(key string, value interface{}) {
		smb := value.(*Samba)
		if smb.inflight.Load() != 0 {
			SambaCache.SetKey(key, smb)
			return
		}
		for key, _ := range smb.share {
			if err := smb.share[key].Umount(); err != nil {
				Log.Warning("samba: error unmounting share: %v", err)
			}
		}
		if err := smb.session.Logoff(); err != nil {
			Log.Warning("samba: error logging out: %v", err)
		}
	})
}

type Samba struct {
	share    map[string]*smb2.Share
	inflight *atomic.Int32
	session  *smb2.Session
	lastUsed *atomic.Int64 // unix seconds of the last time this pooled session was handed out
	lsCache  *lsCache      // per-session directory-listing cache (SMB-2)
}

// sambaIdleProbeSeconds: a pooled session used more recently than this is
// assumed alive and handed back without a probe (keeps the hot path free).
const sambaIdleProbeSeconds int64 = 30

func shouldProbe(lastUsedUnix, nowUnix, idleThreshold int64) bool {
	return nowUnix-lastUsedUnix >= idleThreshold
}

func healthCheckEnabled() bool {
	if b, ok := Config.Get("samba.healthcheck").Interface().(bool); ok {
		return b
	}
	return true // default on
}

// isAlive decides whether a pooled session can be reused. Recently-used
// sessions skip the probe entirely (no added latency); only idle ones are
// checked, and only when the health check is enabled.
func (smb Samba) isAlive() bool {
	now := time.Now().Unix()
	last := int64(0)
	if smb.lastUsed != nil {
		last = smb.lastUsed.Load()
	}
	alive := true
	if healthCheckEnabled() && shouldProbe(last, now, sambaIdleProbeSeconds) {
		alive = smb.probe()
	}
	if alive && smb.lastUsed != nil {
		smb.lastUsed.Store(now)
	}
	return alive
}

// probe does one cheap round-trip to confirm the SMB connection is live.
func (smb Samba) probe() bool {
	for _, sh := range smb.share {
		_, err := sh.Stat(".")
		return err == nil
	}
	_, err := smb.session.ListSharenames()
	return err == nil
}

func (smb Samba) Init(params map[string]string, app *App) (IBackend, error) {
	if strings.HasPrefix(params["host"], "smb://") == false {
		params["host"] = "smb://" + params["host"]
	}
	if u, err := url.Parse(params["host"]); err == nil {
		params["host"] = u.Host
		if params["port"] == "" {
			params["port"] = u.Port()
		}
		if params["share"] == "" {
			params["share"] = strings.ReplaceAll(u.Path, "/", "")
		}
		if params["username"] == "" && u.User != nil {
			params["username"] = u.User.Username()
		}
		if params["password"] == "" && u.User != nil {
			params["password"], _ = u.User.Password()
		}
	}
	if params["port"] == "" {
		params["port"] = "445"
	}
	if c := SambaCache.Get(params); c != nil {
		cached := c.(*Samba)
		if cached.isAlive() {
			return cached, nil
		}
		// Stale pooled session (idle server timeout, blip, server restart):
		// evict it and fall through to establish a fresh connection.
		Log.Debug("plg_backend_samba::stale pooled session, reconnecting host[%s]", params["host"])
		SambaCache.Del(params)
	}

	host := fmt.Sprintf("%s:%s", params["host"], params["port"])
	conn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		Log.Debug("plg_backend_samba::netdial host[%s] err[%s]", host, err.Error())
		return nil, err
	}

	smb.share = make(map[string]*smb2.Share, 0)
	smb.inflight = &atomic.Int32{}
	smb.lastUsed = &atomic.Int64{}
	smb.lastUsed.Store(time.Now().Unix())
	smb.lsCache = newLsCache()
	smb.session, err = (&smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User: func() string {
				if params["username"] == "" {
					return "Guest"
				}
				return params["username"]
			}(),
			Password: params["password"],
			Domain:   params["domain"],
		},
	}).Dial(conn)
	if err != nil {
		Log.Debug("plg_backend_samba::smbdial host[%s] err[%s] username[%s] domain[%s]", host, err.Error(), params["username"], params["domain"])
		return nil, err
	}
	if params["share"] == "" {
		names, err := smb.session.ListSharenames()
		if err != nil {
			Log.Debug("plg_backend_samba::list host[%s] err[%s]", host, err.Error())
			return nil, err
		}
		for _, name := range names {
			if strings.HasSuffix(name, "$") {
				continue
			}
			if m, err := smb.session.Mount(name); err == nil {
				smb.share[name] = m
			}
		}
	} else {
		if m, err := smb.session.Mount(params["share"]); err == nil {
			smb.share[params["share"]] = m
		}
	}
	SambaCache.Set(params, &smb)
	return &smb, nil
}

func (smb Samba) LoginForm() Form {
	return Form{
		Elmnts: []FormElement{
			{
				Name:  "type",
				Type:  "hidden",
				Value: "samba",
			},
			{
				Name:        "host",
				Type:        "text",
				Placeholder: "Hostname",
			},
			{
				Name:        "username",
				Type:        "text",
				Placeholder: "Username",
			},
			{
				Name:        "password",
				Type:        "password",
				Placeholder: "Password",
			},
			{
				Name:        "advanced",
				Type:        "enable",
				Placeholder: "Advanced",
				Target:      []string{"samba_port", "samba_path", "samba_domain", "samba_share"},
			},
			{
				Id:          "samba_path",
				Name:        "path",
				Type:        "text",
				Placeholder: "Path",
			},
			{
				Id:          "samba_port",
				Name:        "port",
				Type:        "number",
				Placeholder: "Port - eg: 445",
			},
			{
				Id:          "samba_domain",
				Name:        "domain",
				Type:        "text",
				Placeholder: "Domain",
			},
			{
				Id:          "samba_share",
				Name:        "share",
				Type:        "text",
				Placeholder: "Share Name",
			},
		},
	}
}

func (smb Samba) Ls(path string) ([]os.FileInfo, error) {
	if path == "/" {
		f := make([]os.FileInfo, 0)
		for key, _ := range smb.share {
			f = append(f, File{
				FName: key,
				FType: "directory",
			})
		}
		return f, nil
	}
	orig := path
	ttl := lsCacheTTL()
	if ttl > 0 && smb.lsCache != nil {
		if data, ok := smb.lsCache.get(orig); ok {
			return data, nil
		}
	}

	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return nil, err
	}

	dir, err := share.Open(path)
	if err != nil {
		return nil, fromSambaErr(err)
	}
	defer dir.Close()

	fs, err := dir.Readdir(-1)
	if err != nil {
		return fs, fromSambaErr(err)
	}
	if ttl > 0 && smb.lsCache != nil {
		smb.lsCache.set(orig, fs, ttl)
	}
	return fs, nil
}

func (smb Samba) Stat(path string) (os.FileInfo, error) {
	if path == "/" {
		for key, _ := range smb.share {
			return File{
				FName: key,
				FType: "directory",
				FTime: -1,
				FSize: 0,
			}, nil
		}
		return nil, ErrNotFound
	}
	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return nil, err
	}
	f, err := share.Stat(path)
	return f, fromSambaErr(err)
}

func (smb Samba) Cat(path string) (io.ReadCloser, error) {
	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return nil, err
	}
	f, err := share.Open(path)
	if err != nil {
		return nil, fromSambaErr(err)
	}
	return NewReadahead(f, smb.inflight), nil
}

func (smb Samba) Mkdir(path string) error {
	orig := path
	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return err
	}
	err = fromSambaErr(share.Mkdir(path, os.ModeDir))
	if err == nil {
		smb.invalidateLs(parentDir(orig))
	}
	return err
}

func (smb Samba) Rm(path string) error {
	orig := path
	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return err
	}
	err = fromSambaErr(share.RemoveAll(path))
	if err == nil {
		smb.invalidateLs(parentDir(orig), orig)
	}
	return err
}

func (smb Samba) Mv(from, to string) error {
	fromShare, fromPath, err := smb.toSambaPath(from)
	if err != nil {
		return err
	}
	toShare, toPath, err := smb.toSambaPath(to)
	if err != nil {
		return err
	}
	if fromShare != toShare {
		return ErrNotImplemented
	}
	err = fromSambaErr(fromShare.Rename(fromPath, toPath))
	if err == nil {
		smb.invalidateLs(parentDir(from), parentDir(to), from)
	}
	return err
}

func (smb Samba) Save(path string, content io.Reader) error {
	smb.inflight.Add(1)
	defer smb.inflight.Add(-1)
	orig := path
	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return err
	}
	f, err := share.Create(path)
	if err != nil {
		return fromSambaErr(err)
	}
	if _, err = io.Copy(f, content); err != nil {
		f.Close()
		return fromSambaErr(err)
	}
	if err = f.Close(); err != nil {
		return err
	}
	smb.invalidateLs(parentDir(orig))
	return nil
}

func (smb Samba) Touch(path string) error {
	orig := path
	share, path, err := smb.toSambaPath(path)
	if err != nil {
		return err
	}
	f, err := share.Create(path)
	if err != nil {
		return fromSambaErr(err)
	}
	if err = fromSambaErr(f.Close()); err != nil {
		return err
	}
	smb.invalidateLs(parentDir(orig))
	return nil
}

// invalidateLs drops cached listings for the given directory keys (no-op when
// the per-session cache is disabled or unset).
func (smb Samba) invalidateLs(keys ...string) {
	if smb.lsCache != nil {
		smb.lsCache.invalidate(keys...)
	}
}

func (smb Samba) toSambaPath(path string) (*smb2.Share, string, error) {
	p := strings.Split(strings.Trim(path, "/"), "/")
	if len(p) == 0 {
		return nil, "", ErrNotAllowed
	}
	sharename := p[0]
	oPath := strings.TrimLeft(strings.Join(p[1:], "\\"), "\\")
	if smb.share[sharename] == nil {
		return nil, "", ErrNotFound
	}
	return smb.share[sharename], oPath, nil
}

func fromSambaErr(err error) error {
	switch {
	case os.IsPermission(err):
		return ErrPermissionDenied
	case os.IsNotExist(err):
		return ErrNotFound
	default:
		return err
	}
}
