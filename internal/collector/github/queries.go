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

package github

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/hasura/go-graphql-client"

	"github.com/ossf/criticality_score/v2/internal/githubapi"
)

const (
	legacyReleaseLookbackDays = 365
	legacyReleaseLookback     = legacyReleaseLookbackDays * 24 * time.Hour
	legacyCommitLookback      = 365 * 24 * time.Hour
)

type basicRepoData struct {
	Name      string
	URL       string
	MirrorURL string

	Owner           struct{ Login string }
	LicenseInfo     struct{ Name string }
	PrimaryLanguage struct{ Name string }

	CreatedAt time.Time
	UpdatedAt time.Time

	DefaultBranchRef struct {
		Target struct {
			Commit struct { // this is the last commit
				AuthoredDate  time.Time
				RecentCommits struct {
					TotalCount int
				} `graphql:"recentcommits:history(since:$legacyCommitLookback)"`
			} `graphql:"... on Commit"`
		}
	}

	StargazerCount   int
	HasIssuesEnabled bool
	IsArchived       bool
	IsDisabled       bool
	IsEmpty          bool
	IsMirror         bool

	Watchers struct{ TotalCount int }

	Tags struct {
		TotalCount int
	} `graphql:"refs(refPrefix:\"refs/tags/\")"`
}

func queryBasicRepoData(ctx context.Context, client *graphql.Client, u *url.URL) (*basicRepoData, error) {
	// Search based on owner and repo name becaues the `repository` query
	// better handles changes in ownership and repository name than the
	// `resource` query.
	// TODO - consider improving support for scp style urls and urls ending in .git
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	owner := parts[0]
	name := parts[1]
	s := &struct {
		Repository basicRepoData `graphql:"repository(owner: $repositoryOwner, name: $repositoryName)"`
	}{}
	now := time.Now().UTC()
	vars := map[string]any{
		"repositoryOwner":      graphql.String(owner),
		"repositoryName":       graphql.String(name),
		"legacyCommitLookback": githubapi.GitTimestamp{Time: now.Add(-legacyCommitLookback)},
	}
	if err := client.Query(ctx, s, vars); err != nil {
		return nil, err
	}
	return &s.Repository, nil
}
