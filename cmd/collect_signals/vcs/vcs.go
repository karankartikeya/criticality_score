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

package vcs

import (
	"runtime/debug"
)

const (
	MissingCommitID = "-missing-"
	commitIDKey     = "vcs.revision"
)

var commitID string

func fetchCommitID() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return MissingCommitID
	}

	for _, setting := range info.Settings {
		if setting.Key == commitIDKey {
			return setting.Value
		}
	}

	return MissingCommitID
}

// CommitID returns the vcs commit ID embedded in the binary when the
// -buildvcs flag is set while building.
func CommitID() string {
	if commitID == "" {
		commitID = fetchCommitID()
	}
	return commitID
}
