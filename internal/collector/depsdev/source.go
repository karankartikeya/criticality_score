// Copyright 2022 Criticality Score Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package depsdev

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"go.uber.org/zap"

	"github.com/ossf/criticality_score/v2/internal/collector/projectrepo"
	"github.com/ossf/criticality_score/v2/internal/collector/signal"
)

const (
	defaultLocation = "US"
)

type depsDevSet struct {
	DependentCount signal.Field[int] `signal:"dependent_count"`
}

func (s *depsDevSet) Namespace() signal.Namespace {
	return "depsdev"
}

type depsDevSource struct {
	logger     *zap.Logger
	dependents *dependents
}

func (c *depsDevSource) EmptySet() signal.Set {
	return &depsDevSet{}
}

func (c *depsDevSource) IsSupported(r projectrepo.Repo) bool {
	_, t := parseRepoURL(r.URL())
	return t != ""
}

func (c *depsDevSource) Get(ctx context.Context, r projectrepo.Repo, jobID string) (signal.Set, error) {
	var s depsDevSet
	n, t := parseRepoURL(r.URL())
	if t == "" {
		return &s, nil
	}
	c.logger.With(zap.String("url", r.URL().String())).Debug("Fetching deps.dev dependent count")
	deps, found, err := c.dependents.Count(ctx, n, t, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deps.dev dependent count: %w", err)
	}
	if found {
		s.DependentCount.Set(deps)
	}
	return &s, nil
}

// NewSource creates a new Source for gathering data from deps.dev.
//
// TODO add options to configure the dataset:
//   - force dataset re-creation (-update-strategy = always,stale,weekly,monthly,never)
//   - force dataset destruction (-depsdev-destroy-data)
func NewSource(ctx context.Context, logger *zap.Logger, projectID, datasetName string, datasetTTL time.Duration) (signal.Source, error) {
	if projectID == "" {
		projectID = bigquery.DetectProjectID
	}
	gcpClient, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create bigquery client: %w", err)
	}
	// Set the location
	gcpClient.Location = defaultLocation

	dependents, err := NewDependents(ctx, gcpClient, logger, datasetName, datasetTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create deps.dev dependents: %w", err)
	}

	return &depsDevSource{
		logger:     logger,
		dependents: dependents,
	}, nil
}

func parseRepoURL(u *url.URL) (projectName, projectType string) {
	switch hn := u.Hostname(); hn {
	case "github.com":
		return strings.Trim(u.Path, "/"), "GITHUB"
	default:
		return "", ""
	}
}
