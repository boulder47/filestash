// Package plg_authorisation_rules adds per-directory, per-user access rules
// (SEC-4) on top of Filestash's IAuthorisation hook.
//
// Safety: it is a GUARANTEED no-op unless authorisation.enable is set (default
// false), and it fails OPEN — any problem reading/parsing the rules disables
// enforcement and logs, so a misconfiguration can never lock users out.
//
// Semantics match how the core probes authorisation: Ls (read) gates whether a
// directory is listable; the write actions gate capability flags (upload,
// delete, rename, …), so a read-only rule yields a read-only view.
package plg_authorisation_rules

import (
	"sync"

	. "github.com/mickael-kerjean/filestash/server/common"
)

const (
	actionRead  = "read"
	actionWrite = "write"
)

var state struct {
	mu      sync.RWMutex
	enabled bool
	def     string
	rules   []Rule
}

func init() {
	Hooks.Register.AuthorisationMiddleware(RulesAuth{})
	Hooks.Register.Onload(func() {
		declareConfig()
		reload()
	})
	Hooks.Register.OnConfig(reload)
}

func declareConfig() {
	Config.Get("authorisation.enable").Schema(func(f *FormElement) *FormElement {
		if f == nil {
			f = &FormElement{}
		}
		f.Id, f.Name, f.Type, f.Default = "authorisation_enable", "enable", "boolean", false
		f.Description = "Enable per-directory authorisation rules (off = no enforcement)"
		return f
	})
	Config.Get("authorisation.default").Schema(func(f *FormElement) *FormElement {
		if f == nil {
			f = &FormElement{}
		}
		f.Id, f.Name, f.Type, f.Default = "authorisation_default", "default", "select", "allow"
		f.Opts = []string{"allow", "deny"}
		f.Description = "Decision when no rule matches"
		return f
	})
	Config.Get("authorisation.rules").Schema(func(f *FormElement) *FormElement {
		if f == nil {
			f = &FormElement{}
		}
		f.Id, f.Name, f.Type, f.Default = "authorisation_rules", "rules", "text", ""
		f.Description = `JSON, e.g. [{"users":["alice"],"paths":["/finance"],"actions":["*"],"effect":"deny"}]`
		return f
	})
}

func reload() {
	enabled := Config.Get("authorisation.enable").Bool()
	def := Config.Get("authorisation.default").String()
	if def == "" {
		def = "allow"
	}
	rules, err := parseRules(Config.Get("authorisation.rules").String())
	if err != nil {
		Log.Warning("plg_authorisation_rules - invalid rules JSON; failing open (no enforcement): %s", err.Error())
		enabled, rules = false, nil
	}
	state.mu.Lock()
	state.enabled, state.def, state.rules = enabled, def, rules
	state.mu.Unlock()
}

type RulesAuth struct{}

func (RulesAuth) decide(ctx *App, action, path string) error {
	state.mu.RLock()
	enabled, def, rules := state.enabled, state.def, state.rules
	state.mu.RUnlock()
	if !enabled {
		return nil
	}
	user := ""
	if ctx != nil {
		user = ctx.Session["user"]
	}
	if evaluate(rules, def, user, action, path) {
		return nil
	}
	return ErrPermissionDenied
}

func (a RulesAuth) Ls(ctx *App, path string) error    { return a.decide(ctx, actionRead, path) }
func (a RulesAuth) Cat(ctx *App, path string) error   { return a.decide(ctx, actionRead, path) }
func (a RulesAuth) Stat(ctx *App, path string) error  { return a.decide(ctx, actionRead, path) }
func (a RulesAuth) Mkdir(ctx *App, path string) error { return a.decide(ctx, actionWrite, path) }
func (a RulesAuth) Rm(ctx *App, path string) error    { return a.decide(ctx, actionWrite, path) }
func (a RulesAuth) Save(ctx *App, path string) error  { return a.decide(ctx, actionWrite, path) }
func (a RulesAuth) Touch(ctx *App, path string) error { return a.decide(ctx, actionWrite, path) }

func (a RulesAuth) Mv(ctx *App, from, to string) error {
	if err := a.decide(ctx, actionWrite, from); err != nil {
		return err
	}
	return a.decide(ctx, actionWrite, to)
}
