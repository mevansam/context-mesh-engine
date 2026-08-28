// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package apiv1

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	iapi "github.com/mevansam/context-mesh-engine/internal/api"
	"github.com/mevansam/context-mesh-engine/internal/plans"
)

// PlansController serves POST /plans/... and GET /openapi.
// GET /openapi (catalog index) is always registered. Plan execute
// routes and per-plan OpenAPI are registered when catalog is non-nil.
type PlansController struct {
	catalog *plans.Catalog
	runner  *plans.Runner
	meta    plans.OpenAPIMeta
	logger  *slog.Logger
}

// NewPlansController returns a REST controller for Arazzo plans.
func NewPlansController(catalog *plans.Catalog, runner *plans.Runner, meta plans.OpenAPIMeta, logger *slog.Logger) *PlansController {
	if logger == nil {
		logger = slog.Default()
	}
	return &PlansController{catalog: catalog, runner: runner, meta: meta, logger: logger}
}

// Register implements api.Controller.
func (c *PlansController) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi", c.openapiCatalog)
	if c.catalog == nil {
		return
	}
	if c.runner != nil && c.runner.QueryEnabled() {
		mux.HandleFunc("POST /plans/query", c.postQuery)
	}
	mux.HandleFunc("POST /plans/{planId}/{workflowId}", c.postLatest)
	mux.HandleFunc("POST /plans/{planId}/{version}/{workflowId}", c.postVersioned)
	mux.HandleFunc("GET /openapi/{planId}", c.openapiLatest)
	mux.HandleFunc("GET /openapi/{planId}/{version}", c.openapiVersioned)
}

func (c *PlansController) postQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string         `json:"query"`
		Data  map[string]any `json:"data"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			iapi.WriteError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if body.Data != nil {
			body.Data = iapi.CanonicalJSON(body.Data).(map[string]any)
		}
	}
	ctx, err := c.runner.EnrichContext(r.Context(), plans.RequestSourceFromHTTP(r))
	if err != nil {
		c.writeRunError(w, err)
		return
	}
	res, err := c.runner.Query(ctx, body.Query, body.Data)
	if err != nil {
		c.writeRunError(w, err)
		return
	}
	iapi.WriteJSON(w, http.StatusOK, res)
}

func (c *PlansController) postLatest(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")
	wf := r.PathValue("workflowId")
	e, ok := c.catalog.Latest(planID)
	if !ok {
		iapi.WriteError(w, http.StatusNotFound, plans.ErrNotFound.Error())
		return
	}
	c.execute(w, r, e.PlanID, e.Version, wf)
}

func (c *PlansController) postVersioned(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")
	seg := r.PathValue("version")
	wf := r.PathValue("workflowId")
	e, ok := c.catalog.GetBySegment(planID, seg)
	if !ok {
		iapi.WriteError(w, http.StatusNotFound, plans.ErrNotFound.Error())
		return
	}
	c.execute(w, r, e.PlanID, e.Version, wf)
}

func (c *PlansController) execute(w http.ResponseWriter, r *http.Request, planID, version, workflowID string) {
	inputs, err := decodeJSONObject(w, r)
	if err != nil {
		iapi.WriteError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	ctx, err := c.runner.EnrichContext(r.Context(), plans.RequestSourceFromHTTP(r))
	if err != nil {
		c.writeRunError(w, err)
		return
	}
	res, err := c.runner.Run(ctx, planID, version, workflowID, inputs)
	if err != nil {
		c.writeRunError(w, err)
		return
	}
	iapi.WriteJSON(w, http.StatusOK, res)
}

func decodeJSONObject(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return nil, nil
	}
	inputs, err := iapi.DecodeMap(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	return inputs, nil
}

func (c *PlansController) openapiCatalog(w http.ResponseWriter, r *http.Request) {
	query := c.runner != nil && c.runner.QueryEnabled()
	b, err := plans.CatalogOpenAPIJSON(c.catalog, query, c.meta)
	if err != nil {
		c.logger.Error("openapi catalog generation failed", "err", err)
		iapi.WriteError(w, http.StatusInternalServerError, plans.ErrInternal.Error())
		return
	}
	writeOpenAPIBytes(w, b)
}

func (c *PlansController) openapiLatest(w http.ResponseWriter, r *http.Request) {
	e, ok := c.catalog.Latest(r.PathValue("planId"))
	if !ok {
		iapi.WriteError(w, http.StatusNotFound, plans.ErrNotFound.Error())
		return
	}
	c.writeOpenAPI(w, e, true)
}

func (c *PlansController) openapiVersioned(w http.ResponseWriter, r *http.Request) {
	e, ok := c.catalog.GetBySegment(r.PathValue("planId"), r.PathValue("version"))
	if !ok {
		iapi.WriteError(w, http.StatusNotFound, plans.ErrNotFound.Error())
		return
	}
	c.writeOpenAPI(w, e, false)
}

func (c *PlansController) writeOpenAPI(w http.ResponseWriter, e *plans.Entry, latest bool) {
	b, err := plans.OpenAPIJSON(e, latest, c.meta)
	if err != nil {
		c.logger.Error("openapi generation failed", "err", err)
		iapi.WriteError(w, http.StatusInternalServerError, plans.ErrInternal.Error())
		return
	}
	writeOpenAPIBytes(w, b)
}

func writeOpenAPIBytes(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (c *PlansController) writeRunError(w http.ResponseWriter, err error) {
	pub := plans.LogAndPublic(c.logger, err)
	iapi.WriteError(w, pub.Status, pub.Message)
}
