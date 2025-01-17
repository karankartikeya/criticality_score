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

// Package githubmentions provides a Collector that returns a Set for the
// number of mentions a given repository has in commit messages as returned by
// GitHub's search interface.
//
// This signal formed the basis of the original version of dependent count,
// however it is a noisy, unreliable signal.
package githubmentions

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-github/v47/github"

	"github.com/ossf/criticality_score/v2/internal/collector/projectrepo"
	"github.com/ossf/criticality_score/v2/internal/collector/signal"
	"github.com/ossf/criticality_score/v2/internal/githubapi"
)

type mentionSet struct {
	MentionCount signal.Field[int] `signal:"github_mention_count,legacy"`
}

func (s *mentionSet) Namespace() signal.Namespace {
	return signal.Namespace("github_mentions")
}

type Source struct {
	client *githubapi.Client
}

func NewSource(c *githubapi.Client) signal.Source {
	return &Source{
		client: c,
	}
}

func (c *Source) EmptySet() signal.Set {
	return &mentionSet{}
}

func (c *Source) IsSupported(r projectrepo.Repo) bool {
	return true
}

func (c *Source) Get(ctx context.Context, r projectrepo.Repo, _ string) (signal.Set, error) {
	s := &mentionSet{}
	if c, err := c.githubSearchTotalCommitMentions(ctx, r.URL()); err != nil {
		return nil, err
	} else {
		s.MentionCount.Set(c)
	}
	return s, nil
}

func (c *Source) githubSearchTotalCommitMentions(ctx context.Context, u *url.URL) (int, error) {
	repoName := strings.Trim(u.Path, "/")
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 1},
	}
	commits, _, err := c.client.Rest().Search.Commits(ctx, fmt.Sprintf("\"%s\"", repoName), opts)
	if err != nil {
		return 0, err
	}
	return commits.GetTotal(), nil
}
