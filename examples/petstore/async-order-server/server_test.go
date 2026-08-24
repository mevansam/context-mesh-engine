// Copyright 2026 Fidelity Investments. All rights reserved.
// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPetIDFromBody(t *testing.T) {
	id, err := petIDFromBody([]byte(`{"petId": 10}`))
	if err != nil || id != 10 {
		t.Fatalf("flat: id=%d err=%v", id, err)
	}
	id, err = petIDFromBody([]byte(`{"payload":{"petId": 7}}`))
	if err != nil || id != 7 {
		t.Fatalf("nested: id=%d err=%v", id, err)
	}
}

func TestPlaceAndConfirm(t *testing.T) {
	var placed []byte
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/store/order" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		placed, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"petId":10,"status":"placed"}`))
	}))
	t.Cleanup(hosted.Close)

	s := newOrderServer([]string{hosted.URL})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /place-order", s.handlePlaceOrder)
	mux.HandleFunc("GET /confirm-order", s.handleConfirmOrder)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/place-order", strings.NewReader(`{"petId":10}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("orderCorrelationId", "corr-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("place status=%d body=%s", resp.StatusCode, b)
	}
	var body map[string]any
	if err := json.Unmarshal(placed, &body); err != nil {
		t.Fatal(err)
	}
	if body["petId"] != float64(10) {
		t.Fatalf("hosted body = %s", placed)
	}
	postedID, ok := asInt64(body["id"])
	if !ok || postedID == 0 {
		t.Fatalf("expected non-zero posted id, body = %s", placed)
	}

	creq, _ := http.NewRequest(http.MethodGet, ts.URL+"/confirm-order", nil)
	creq.Header.Set("orderCorrelationId", "corr-1")
	cresp, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp.Body.Close()
	if cresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(cresp.Body)
		t.Fatalf("confirm status=%d body=%s", cresp.StatusCode, b)
	}
	var conf map[string]any
	if err := json.NewDecoder(cresp.Body).Decode(&conf); err != nil {
		t.Fatal(err)
	}
	payload, _ := conf["payload"].(map[string]any)
	if payload["orderId"] != float64(42) {
		t.Fatalf("confirm = %#v", conf)
	}
}

func TestPlaceAndConfirm_LargeOrderID(t *testing.T) {
	const want int64 = 9056269108963012608
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":9056269108963012608,"petId":10,"status":"placed"}`))
	}))
	t.Cleanup(hosted.Close)

	s := newOrderServer([]string{hosted.URL})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /place-order", s.handlePlaceOrder)
	mux.HandleFunc("GET /confirm-order", s.handleConfirmOrder)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/place-order", strings.NewReader(`{"petId":10}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("orderCorrelationId", "corr-big")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("place status=%d", resp.StatusCode)
	}

	creq, _ := http.NewRequest(http.MethodGet, ts.URL+"/confirm-order", nil)
	creq.Header.Set("orderCorrelationId", "corr-big")
	cresp, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp.Body.Close()
	raw, _ := io.ReadAll(cresp.Body)
	if !strings.Contains(string(raw), "9056269108963012608") {
		t.Fatalf("confirm lost precision: %s", raw)
	}
	if strings.Contains(string(raw), "9056269108963013000") {
		t.Fatalf("confirm rounded id: %s", raw)
	}
	s.mu.Lock()
	got := s.byCorr["corr-big"].OrderID
	s.mu.Unlock()
	if got != want {
		t.Fatalf("stored id = %d", got)
	}
}

func TestPlaceOrder_ZeroResponseIDUsesPostedID(t *testing.T) {
	var posted map[string]any
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &posted)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":0,"petId":10,"status":"placed"}`))
	}))
	t.Cleanup(hosted.Close)

	s := newOrderServer([]string{hosted.URL})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /place-order", s.handlePlaceOrder)
	mux.HandleFunc("GET /confirm-order", s.handleConfirmOrder)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/place-order", strings.NewReader(`{"petId":10}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("orderCorrelationId", "corr-zero")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("place status=%d body=%s", resp.StatusCode, b)
	}
	postedID, ok := asInt64(posted["id"])
	if !ok || postedID == 0 {
		t.Fatalf("expected non-zero posted id, got %#v", posted)
	}

	creq, _ := http.NewRequest(http.MethodGet, ts.URL+"/confirm-order", nil)
	creq.Header.Set("orderCorrelationId", "corr-zero")
	cresp, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp.Body.Close()
	if cresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(cresp.Body)
		t.Fatalf("confirm status=%d body=%s", cresp.StatusCode, b)
	}
	var conf map[string]any
	if err := json.NewDecoder(cresp.Body).Decode(&conf); err != nil {
		t.Fatal(err)
	}
	payload, _ := conf["payload"].(map[string]any)
	got, ok := asInt64(payload["orderId"])
	if !ok || got != postedID {
		t.Fatalf("confirm orderId=%v posted=%d conf=%#v", payload["orderId"], postedID, conf)
	}
}

func TestResolvePetstoreBase(t *testing.T) {
	got, err := resolvePetstoreBase("local", "")
	if err != nil || got != petstoreLocalBase {
		t.Fatalf("local: %q %v", got, err)
	}
	got, err = resolvePetstoreBase("hosted", "")
	if err != nil || got != petstoreHostedBase {
		t.Fatalf("hosted: %q %v", got, err)
	}
	got, err = resolvePetstoreBase("local", "http://example:9/api/v3/")
	if err != nil || got != "http://example:9/api/v3" {
		t.Fatalf("override: %q %v", got, err)
	}
	if _, err := resolvePetstoreBase("other", ""); err == nil {
		t.Fatal("expected error for unknown -petstore")
	}
}
