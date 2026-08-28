// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

// Petstore OAuth demo: password login via Petstore GET /user/login, then
// GET /user/{username} for userStatus, then issue HS256 JWTs.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mevansam/context-mesh-engine/examples/petstore/auth-server/jwtx"
)

const (
	defaultAddr         = "localhost:8092"
	petstoreLocalBase   = "http://localhost:8090/api/v3"
	petstoreHostedBase  = "https://petstore3.swagger.io/api/v3"
	defaultClientID     = "petstore-mcp"
	defaultClientSecret = "mcp-secret"
	defaultJWTSecret    = "petstore-demo-hs256"
	tokenTTL            = time.Hour
)

func main() {
	addr := flag.String("addr", defaultAddr, "listen address")
	petstore := flag.String("petstore", "local", "Petstore 3 target: local or hosted")
	petstoreURL := flag.String("petstore-url", "", "override Petstore 3 OpenAPI origin")
	jwtSecret := flag.String("jwt-secret", defaultJWTSecret, "HS256 secret shared with mcp-server")
	clientID := flag.String("client-id", defaultClientID, "OAuth client_id for client_credentials")
	clientSecret := flag.String("client-secret", defaultClientSecret, "OAuth client_secret")
	flag.Parse()

	base, err := resolvePetstoreBase(*petstore, *petstoreURL)
	if err != nil {
		log.Fatal(err)
	}

	s := &authServer{
		petstore:     strings.TrimRight(base, "/"),
		jwtSecret:    []byte(*jwtSecret),
		clientID:     *clientID,
		clientSecret: *clientSecret,
		http:         &http.Client{Timeout: 15 * time.Second},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("POST /oauth/token", s.handleToken)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("auth-server http://%s  petstore %s", *addr, s.petstore)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	_ = srv.Shutdown(context.Background())
}

type authServer struct {
	petstore     string
	jwtSecret    []byte
	clientID     string
	clientSecret string
	http         *http.Client
}

func (s *authServer) handleToken(w http.ResponseWriter, r *http.Request) {
	grant, params, err := parseTokenRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	switch grant {
	case "client_credentials":
		id := params["client_id"]
		sec := params["client_secret"]
		if id != s.clientID || sec != s.clientSecret {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_client"})
			return
		}
		tok, err := jwtx.SignClient(s.jwtSecret, id, tokenTTL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
			return
		}
		log.Printf("issued client JWT grant=client_credentials client_id=%s iss=%s aud=%s ttl=%s",
			id, jwtx.IssuerAuth, jwtx.AudienceMCP, tokenTTL)
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": tok,
			"token_type":   "Bearer",
			"expires_in":   int(tokenTTL.Seconds()),
		})
	case "password":
		user := params["username"]
		pass := params["password"]
		if user == "" || pass == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request", "error_description": "username and password required"})
			return
		}
		if err := s.loginUser(r.Context(), user, pass); err != nil {
			log.Printf("loginUser %s: %v", user, err)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_grant"})
			return
		}
		status, err := s.userStatus(r.Context(), user)
		if err != nil {
			log.Printf("getUserByName %s: %v", user, err)
			status = 1
		}
		tok, err := jwtx.SignUser(s.jwtSecret, user, status, tokenTTL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error"})
			return
		}
		log.Printf("issued user JWT grant=password username=%s userStatus=%d iss=%s aud=%s ttl=%s",
			user, status, jwtx.IssuerAuth, jwtx.AudienceMCP, tokenTTL)
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": tok,
			"token_type":   "Bearer",
			"expires_in":   int(tokenTTL.Seconds()),
			"username":     user,
			"userStatus":   status,
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported_grant_type"})
	}
}

func (s *authServer) loginUser(ctx context.Context, username, password string) error {
	u := s.petstore + "/user/login?" + url.Values{
		"username": {username},
		"password": {password},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login status %d", resp.StatusCode)
	}
	log.Printf("petstore loginUser username=%s status=%d", username, resp.StatusCode)
	return nil
}

func (s *authServer) userStatus(ctx context.Context, username string) (int, error) {
	u := s.petstore + "/user/" + url.PathEscape(username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("getUser status %d", resp.StatusCode)
	}
	var user struct {
		UserStatus int `json:"userStatus"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return 0, err
	}
	if user.UserStatus == 0 {
		user.UserStatus = 1
	}
	log.Printf("petstore getUserByName username=%s userStatus=%d", username, user.UserStatus)
	return user.UserStatus, nil
}

func parseTokenRequest(r *http.Request) (grant string, params map[string]string, err error) {
	params = map[string]string{}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var body map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil && err != io.EOF {
			return "", nil, err
		}
		for k, v := range body {
			params[k] = fmt.Sprint(v)
		}
		return params["grant_type"], params, nil
	}
	if err := r.ParseForm(); err != nil {
		return "", nil, err
	}
	for k := range r.PostForm {
		params[k] = r.PostForm.Get(k)
	}
	return params["grant_type"], params, nil
}

func resolvePetstoreBase(kind, urlOverride string) (string, error) {
	if u := strings.TrimSpace(urlOverride); u != "" {
		return strings.TrimRight(u, "/"), nil
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "local":
		return petstoreLocalBase, nil
	case "hosted":
		return petstoreHostedBase, nil
	default:
		return "", fmt.Errorf("-petstore must be local or hosted, got %q", kind)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
