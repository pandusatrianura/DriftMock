package diff

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pandusatrianura/driftmock/internal/model"
	"github.com/pandusatrianura/driftmock/internal/storage"
)

// RunnerOptions configures execution of the contract drift check suite.
type RunnerOptions struct {
	TargetBaseURL string
	Timeout       time.Duration
	HTTPClient    *http.Client
	DiffOptions   DiffOptions
}

// Runner iterates over stored stubs, queries the live target, and collects semantic diff reports.
type Runner struct {
	store   storage.Store
	differ  *Differ
	options RunnerOptions
	client  *http.Client
}

// NewRunner creates an initialized Runner instance.
func NewRunner(store storage.Store, opts RunnerOptions) *Runner {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
		}
	}

	return &Runner{
		store:   store,
		differ:  NewDiffer(opts.DiffOptions),
		options: opts,
		client:  client,
	}
}

// Run executes the contract drift check against all stubs in storage.
func (r *Runner) Run(ctx context.Context) ([]*model.DiffReport, error) {
	stubs, err := r.store.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list stubs: %w", err)
	}

	reports := make([]*model.DiffReport, 0, len(stubs))

	for _, stub := range stubs {
		select {
		case <-ctx.Done():
			return reports, ctx.Err()
		default:
		}

		report, err := r.checkSingleStub(ctx, stub)
		if err != nil {
			// Record network/transport errors as breaking diff items
			report = &model.DiffReport{
				Method: stub.Request.Method,
				Path:   stub.Request.Path,
				StubID: stub.ID,
				Items: []model.DiffItem{
					NewBreakingDiff(
						"network",
						fmt.Sprintf("target request failed: %v", err),
						"reachable",
						"unreachable",
					),
				},
			}
		}

		reports = append(reports, report)
	}

	return reports, nil
}

func (r *Runner) checkSingleStub(ctx context.Context, stub *model.Stub) (*model.DiffReport, error) {
	baseURL := strings.TrimRight(r.options.TargetBaseURL, "/")
	reqPath := strings.TrimLeft(stub.Request.Path, "/")
	fullURL := fmt.Sprintf("%s/%s", baseURL, reqPath)

	// Append query parameters if present
	if len(stub.Request.Query) > 0 {
		q := url.Values(stub.Request.Query)
		fullURL = fmt.Sprintf("%s?%s", fullURL, q.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, stub.Request.Method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP request for %s %s: %w", stub.Request.Method, fullURL, err)
	}

	// Copy non-redacted request headers
	for k, vv := range stub.Request.Headers {
		for _, v := range vv {
			if v != "[REDACTED]" {
				req.Header.Add(k, v)
			}
		}
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read live response body: %w", err)
	}

	// Decompress gzip if live server responds with gzip
	decompressed := decompressIfNeeded(resp.Header.Get("Content-Encoding"), bodyBytes)

	return r.differ.CompareStub(stub, resp.StatusCode, decompressed)
}

func decompressIfNeeded(encoding string, data []byte) []byte {
	if strings.EqualFold(strings.TrimSpace(encoding), "gzip") && len(data) > 0 {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return data
		}
		defer reader.Close()
		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return data
		}
		return decompressed
	}
	return data
}

// CompareResponse is a convenience method to compare a stub directly with in-memory response data.
func (r *Runner) CompareResponse(stub *model.Stub, statusCode int, body []byte) (*model.DiffReport, error) {
	return r.differ.CompareStub(stub, statusCode, body)
}
