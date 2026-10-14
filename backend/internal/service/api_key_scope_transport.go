package service

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/tidwall/gjson"
)

// CheckUpstreamModelScope runs immediately before network/plugin execution, so
// exact/wildcard account mappings, compact mappings and protocol retries cannot
// move a request across provider categories. It never mutates model names.
func CheckUpstreamModelScope(req *http.Request) error {
	if req == nil || ModelScopeFromContext(req.Context()) == ModelScopeAll {
		return nil
	}
	if !ValidModelScope(ModelScopeFromContext(req.Context())) {
		return ErrModelScopeDenied
	}
	var models []string
	if req.URL != nil {
		path := req.URL.Path
		if i := strings.LastIndex(path, "/models/"); i >= 0 {
			model := path[i+len("/models/"):]
			if action := strings.IndexByte(model, ':'); action >= 0 {
				model = model[:action]
			}
			models = append(models, model)
		}
		// Bedrock stores the final invocation model in the URL, not JSON.
		if i := strings.Index(path, "/model/"); i >= 0 {
			model := strings.SplitN(path[i+len("/model/"):], "/", 2)[0]
			if decoded, err := url.PathUnescape(model); err == nil {
				model = decoded
			}
			models = append(models, model)
		}
		if model := req.URL.Query().Get("model"); model != "" {
			models = append(models, model)
		}
	}
	if req.Body != nil && req.Body != http.NoBody {
		var body []byte
		var err error
		if req.GetBody != nil {
			copyBody, copyErr := req.GetBody()
			if copyErr != nil {
				return copyErr
			}
			body, err = io.ReadAll(copyBody)
			_ = copyBody.Close()
		} else {
			body, err = io.ReadAll(req.Body)
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		if err != nil {
			return err
		}
		models = append(models, requestmodel.FromBodyCandidates("", req.Header.Get("Content-Type"), body)...)
		models = append(models, requestmodel.FromBodyCandidates("/v1/live", req.Header.Get("Content-Type"), body)...)
		// Google v1internal envelopes put generation parameters under request.
		if nested := gjson.GetBytes(body, "request"); nested.IsObject() {
			models = append(models, requestmodel.FromBodyCandidates("", "application/json", []byte(nested.Raw))...)
		}
		// Grok's internal generation endpoint names this field modelName.
		if model := gjson.GetBytes(body, "modelName").String(); model != "" {
			models = append(models, model)
		}
	}
	return CheckModelScope(req.Context(), models...)
}

type modelScopeUpstream struct{ HTTPUpstream }

func withModelScopeUpstream(upstream HTTPUpstream) HTTPUpstream {
	if upstream == nil {
		return nil
	}
	if _, ok := upstream.(*modelScopeUpstream); ok {
		return upstream
	}
	return &modelScopeUpstream{HTTPUpstream: upstream}
}

func (u *modelScopeUpstream) Do(req *http.Request, proxy string, accountID int64, concurrency int) (*http.Response, error) {
	if err := CheckUpstreamModelScope(req); err != nil {
		return nil, err
	}
	return u.HTTPUpstream.Do(req, proxy, accountID, concurrency)
}

func (u *modelScopeUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if err := CheckUpstreamModelScope(req); err != nil {
		return nil, err
	}
	return u.HTTPUpstream.DoWithTLS(req, proxy, accountID, concurrency, profile)
}
