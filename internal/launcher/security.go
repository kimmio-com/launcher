package launcher

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const csrfCookieName = "kimmio_csrf"

func withMutationGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requiresMutationGuard(r.Method) {
			if reason := validateMutationRequest(r); reason != "" {
				logWarn("request_blocked", map[string]any{"reason": reason, "path": r.URL.Path, "method": r.Method})
				http.Error(w, reason, http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func requiresMutationGuard(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookieName); err == nil && strings.TrimSpace(c.Value) != "" {
		return c.Value
	}
	token := randomToken(48)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
	return token
}

func validateMutationRequest(r *http.Request) string {
	if !isLoopbackRequest(r) {
		return "forbidden: local requests only"
	}
	if !hasValidOriginOrReferer(r) {
		return "forbidden: invalid request origin"
	}
	expected, err := r.Cookie(csrfCookieName)
	if err != nil || strings.TrimSpace(expected.Value) == "" {
		return "forbidden: missing csrf cookie"
	}
	provided := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if provided == "" {
		if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/x-www-form-urlencoded") ||
			strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
			_ = r.ParseForm()
			provided = strings.TrimSpace(r.FormValue("csrf_token"))
		}
	}
	if provided == "" {
		return "forbidden: missing csrf token"
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected.Value)) != 1 {
		return "forbidden: invalid csrf token"
	}
	return ""
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	requestHost := strings.ToLower(r.Host)
	requestHost = strings.TrimPrefix(requestHost, "[")
	requestHost = strings.TrimSuffix(requestHost, "]")
	if strings.HasPrefix(requestHost, "localhost") || strings.HasPrefix(requestHost, "127.0.0.1") || strings.HasPrefix(requestHost, "::1") {
		return true
	}
	return false
}

func hasValidOriginOrReferer(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		if !isAllowedRequestURL(origin, r.Host) {
			return false
		}
	}
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		if !isAllowedRequestURL(referer, r.Host) {
			return false
		}
	}
	return true
}

func isAllowedRequestURL(raw, expectedHost string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	exp := strings.ToLower(expectedHost)
	if host != exp {
		return false
	}
	name := strings.ToLower(u.Hostname())
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}
