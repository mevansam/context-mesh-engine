// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Local Petstore 3 does not allocate store order ids; omitting id stores 0.
// Hosted petstore3 usually returns its own id, which we prefer when non-zero.
var orderIDs atomic.Int64

type confirmation struct {
	OrderID int64
	PetID   int64
	Status  string
	Raw     map[string]any
}

type orderServer struct {
	mu     sync.Mutex
	byCorr map[string]confirmation
	client *http.Client
	hosts  []string
}

func newOrderServer(hosts []string) *orderServer {
	if len(hosts) == 0 {
		hosts = []string{defaultPetstoreStore}
	}
	trimmed := make([]string, len(hosts))
	for i, h := range hosts {
		trimmed[i] = strings.TrimRight(h, "/")
	}
	return &orderServer{
		byCorr: make(map[string]confirmation),
		client: &http.Client{Timeout: 15 * time.Second},
		hosts:  trimmed,
	}
}

const (
	petstoreLocalBase    = "http://localhost:8090/api/v3"
	petstoreHostedBase   = "https://petstore3.swagger.io/api/v3"
	defaultPetstoreStore = petstoreLocalBase
)

// resolvePetstoreBase maps -petstore local|hosted to an origin.
// urlOverride (-petstore-url) wins when set.
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

func (s *orderServer) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	corr := correlationID(r)
	if corr == "" {
		http.Error(w, "missing orderCorrelationId / orderRequestId header", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	petID, err := petIDFromBody(body)
	if err != nil || petID == 0 {
		http.Error(w, "json body must include petId", http.StatusBadRequest)
		return
	}

	order, err := s.placeHosted(r.Context(), petID)
	if err != nil {
		log.Printf("place-order corr=%s pet=%d: %v", corr, petID, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	s.mu.Lock()
	s.byCorr[corr] = order
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":       true,
		"orderRequestId": corr,
	})
}

func (s *orderServer) handleConfirmOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	corr := correlationID(r)
	if corr == "" {
		http.Error(w, "missing orderCorrelationId / orderRequestId", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	c, ok := s.byCorr[corr]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "confirmation not ready", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"headers": map[string]any{"orderRequestId": corr},
		"payload": map[string]any{"orderId": c.OrderID},
	})
}

func nextOrderID() int64 {
	if orderIDs.Load() == 0 {
		orderIDs.CompareAndSwap(0, time.Now().UnixMilli())
	}
	n := orderIDs.Add(1)
	if n <= 0 {
		return 1
	}
	return n
}

func (s *orderServer) placeHosted(ctx context.Context, petID int64) (confirmation, error) {
	postedID := nextOrderID()
	payload, _ := json.Marshal(map[string]any{
		"id":       postedID,
		"petId":    petID,
		"quantity": 1,
		"status":   "placed",
		"complete": false,
	})
	var lastErr error
	for _, base := range s.hosts {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/store/order", bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("%s: HTTP %d %s", base, resp.StatusCode, bytes.TrimSpace(raw))
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var decoded map[string]any
		if err := dec.Decode(&decoded); err != nil {
			lastErr = err
			continue
		}
		id, _ := asInt64(decoded["id"])
		if id == 0 {
			id = postedID
		}
		status, _ := decoded["status"].(string)
		log.Printf("placed order %d for pet %d via %s status=%s", id, petID, base, status)
		return confirmation{OrderID: id, PetID: petID, Status: status, Raw: decoded}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no petstore host configured")
	}
	return confirmation{}, lastErr
}

func correlationID(r *http.Request) string {
	for _, k := range []string{"Ordercorrelationid", "Orderrequestid", "orderCorrelationId", "orderRequestId"} {
		if v := r.Header.Get(k); v != "" {
			return v
		}
	}
	return r.URL.Query().Get("orderCorrelationId")
}

func petIDFromBody(raw []byte) (int64, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0, err
	}
	if payload, ok := m["payload"].(map[string]any); ok {
		if id, ok := asInt64(payload["petId"]); ok {
			return id, nil
		}
	}
	id, _ := asInt64(m["petId"])
	return id, nil
}

func asInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	case string:
		n, err := json.Number(t).Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
