// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

// Package github implements a client SDK for the Github API.
package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/terramate-io/terramate/errors"
)

const (
	// ErrNotFound indicates the resource does not exists.
	ErrNotFound errors.Kind = "resource not found (HTTP Status: 404)"
	// ErrUnprocessableEntity indicates the entity cannot be processed for any reason.
	ErrUnprocessableEntity errors.Kind = "entity cannot be processed (HTTP Status: 422)"
)

const (
	// Domain is the default GitHub domain.
	Domain = "github.com"
	// APIDomain is the default GitHub API domain.
	APIDomain = "api." + Domain
	// APIBaseURL is the default base url for the GitHub API.
	APIBaseURL = "https://" + APIDomain
)

type (
	// Client is a Github HTTP client wrapper.
	Client struct {
		// BaseURL is the base URL used to construct the final URL of endpoints.
		// If not set, then api.github.com is used.
		BaseURL string

		// HTTPClient sets the HTTP client used and then allows for advanced
		// connection reuse schemes. If not set, a new http.Client is used.
		HTTPClient *http.Client

		// Token is the Github token (usually provided by the GH_TOKEN environment
		// variable.
		Token string
	}

	// OIDCVars is the variables used for issuing new OIDC tokens.
	OIDCVars struct {
		ReqURL   string
		ReqToken string
	}
)

// OIDCToken requests a new OIDC token.
func (c *Client) OIDCToken(ctx context.Context, cfg OIDCVars) (token string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.ReqURL, nil)
	if err != nil {
		return "", errors.E(err, "creating Github OIDC request")
	}

	req.Header.Set("Authorization", "Bearer "+cfg.ReqToken)

	data, err := c.doGetWithReq(req)
	if err != nil {
		return "", errors.E(err, "reading Github OIDC response body")
	}

	type response struct {
		Value string `json:"value"`
	}

	var tokresp response
	err = json.Unmarshal(data, &tokresp)
	if err != nil {
		return "", errors.E(err, "unmarshaling Github OIDC JSON response")
	}

	return tokresp.Value, nil
}

func (c *Client) doGetWithReq(req *http.Request) ([]byte, error) {
	client := c.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.E(err, "requesting GET %s", req.URL)
	}

	defer func() {
		err = errors.L(err, resp.Body.Close()).AsError()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.E(err, "reading response body")
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.E(ErrNotFound, "retrieving %s", req.URL)
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		return nil, errors.E(ErrUnprocessableEntity, "retrieving %s", req.URL)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.E("unexpected status code: %s while getting %s", resp.Status, req.URL)
	}
	return data, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{}
	}
	return c.HTTPClient
}
