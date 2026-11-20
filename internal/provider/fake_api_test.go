package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI mirrors the Shieldome routes the provider uses, with the same
// status codes and field shapes as app.py (2026-11-20). It is not a mock of
// the provider's calls: tests assert on the end state through Terraform.
type fakeAPI struct {
	mu        sync.Mutex
	key       string
	n         int
	sites     map[string]map[string]any
	scheds    map[string]map[string]any
	profiles  map[string]map[string]any
	scans     []map[string]any
	published map[string]string // DNS TXT records "published" by the test
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	f := &fakeAPI{key: "shd_test", sites: map[string]map[string]any{}, scheds: map[string]map[string]any{},
		profiles: map[string]map[string]any{}, published: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeAPI) id(prefix string) string { f.n++; return fmt.Sprintf("%s-%d", prefix, f.n) }

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func str(v any) string { s, _ := v.(string); return s }

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Shieldome-Key") != f.key {
		// the real app redirects to the login page (HTML)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html>login</html>"))
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	// ── sites ──
	case r.URL.Path == "/api/me/allowed-sites" && r.Method == "POST":
		d := strings.ToLower(str(body["domain"]))
		for _, s := range f.sites {
			if s["domain"] == d {
				reply(w, 409, map[string]string{"error": "Site already authorized"})
				return
			}
		}
		s := map[string]any{"id": f.id("site"), "domain": d, "is_verified": false, "verification_active": false,
			"verify_method": nil, "verified_at": nil, "verify_token": nil}
		f.sites[s["id"].(string)] = s
		reply(w, 201, s)
	case len(p) == 4 && p[2] == "allowed-sites" && r.Method == "GET":
		s, ok := f.sites[p[3]]
		if !ok {
			reply(w, 404, map[string]string{"error": "Not found"})
			return
		}
		out := map[string]any{}
		for k, v := range s {
			out[k] = v
		}
		out["verify_record"] = f.record(s)
		reply(w, 200, out)
	case len(p) == 4 && p[2] == "allowed-sites" && r.Method == "DELETE":
		reply(w, 403, map[string]string{"error": "Authorized sites cannot be removed self-service.", "code": "ticket_required"})
	case len(p) == 6 && p[4] == "verify" && p[5] == "init":
		s, ok := f.sites[p[3]]
		if !ok {
			reply(w, 404, map[string]string{"error": "Site not found"})
			return
		}
		if s["verification_active"] == true {
			reply(w, 200, map[string]any{"already_verified": true})
			return
		}
		if s["verify_token"] == nil {
			s["verify_token"] = "tok" + strings.TrimPrefix(p[3], "site-")
		}
		s["verify_method"] = "dns_txt"
		reply(w, 200, map[string]any{"token": s["verify_token"], "verify_record": f.record(s)})
	case len(p) == 6 && p[4] == "verify" && p[5] == "check":
		s, ok := f.sites[p[3]]
		if !ok {
			reply(w, 404, map[string]string{"error": "Site not found"})
			return
		}
		if s["verify_token"] == nil {
			reply(w, 400, map[string]string{"error": "Run /verify/init first"})
			return
		}
		rec := f.record(s)
		if f.published[rec["name"]] == rec["value"] {
			s["is_verified"], s["verification_active"], s["verified_at"] = true, true, "2026-11-20T10:00:00"
			reply(w, 200, map[string]any{"verified": true, "verified_at": "2026-11-20T10:00:00"})
			return
		}
		reply(w, 200, map[string]any{"verified": false, "message": "Token not found."})
	// ── scheduled scans ──
	case r.URL.Path == "/api/scheduled-scans" && r.Method == "POST":
		s := map[string]any{"id": f.id("sched"), "target_url": body["target_url"], "scan_type": body["scan_type"],
			"interval_hours": body["interval_hours"], "enabled": true, "notify_email": body["notify_email"],
			"webhook_url": body["webhook_url"], "excluded_paths": body["excluded_paths"],
			"next_run": "2026-11-21T10:00:00", "last_run": nil}
		f.scheds[s["id"].(string)] = s
		reply(w, 201, s)
	case len(p) == 3 && p[1] == "scheduled-scans":
		s, ok := f.scheds[p[2]]
		if !ok {
			reply(w, 404, map[string]string{"error": "Not found"})
			return
		}
		switch r.Method {
		case "GET":
			reply(w, 200, s)
		case "PATCH":
			for _, k := range []string{"scan_type", "enabled", "notify_email", "webhook_url", "excluded_paths"} {
				if v, ok := body[k]; ok {
					s[k] = v
				}
			}
			if v, ok := body["interval_hours"]; ok {
				s["interval_hours"], s["next_run"] = v, "2026-12-01T00:00:00"
			}
			reply(w, 200, s)
		case "DELETE":
			delete(f.scheds, p[2])
			reply(w, 200, map[string]bool{"deleted": true})
		}
	// ── profiles ──
	case r.URL.Path == "/api/profiles" && r.Method == "POST":
		s := map[string]any{"id": f.id("prof")}
		for k, v := range body {
			s[k] = v
		}
		if str(s["platform_profile"]) == "" {
			s["platform_profile"] = nil
		}
		f.profiles[s["id"].(string)] = s
		reply(w, 201, s)
	case len(p) == 3 && p[1] == "profiles":
		s, ok := f.profiles[p[2]]
		if !ok {
			reply(w, 404, map[string]string{"error": "Not found"})
			return
		}
		switch r.Method {
		case "GET":
			reply(w, 200, s)
		case "PATCH":
			for k, v := range body {
				s[k] = v
			}
			if str(s["platform_profile"]) == "" {
				s["platform_profile"] = nil
			}
			reply(w, 200, s)
		case "DELETE":
			delete(f.profiles, p[2])
			reply(w, 200, map[string]string{"deleted": p[2]})
		}
	// ── scans ──
	case r.URL.Path == "/api/scans" && r.Method == "GET":
		items := []map[string]any{}
		for _, s := range f.scans {
			if s["target_url"] == r.URL.Query().Get("target_url") && s["status"] == "completed" {
				items = append(items, s)
				break
			}
		}
		reply(w, 200, map[string]any{"items": items, "total": len(items)})
	default:
		reply(w, 404, map[string]string{"error": "no route " + r.Method + " " + r.URL.Path})
	}
}

func (f *fakeAPI) record(s map[string]any) map[string]string {
	if s["verify_token"] == nil {
		return nil
	}
	d := strings.TrimPrefix(s["domain"].(string), "www.")
	return map[string]string{"type": "TXT", "name": "_shieldome-verify." + d,
		"value": "shieldome-verify=" + s["verify_token"].(string)}
}
