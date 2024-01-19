// Copyright 2023 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package cloud_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/madlambda/spells/assert"
	"github.com/terramate-io/terramate/cloud"
	"github.com/terramate-io/terramate/cloud/deployment"
	"github.com/terramate-io/terramate/cloud/drift"
	"github.com/terramate-io/terramate/cloud/stack"
	"github.com/terramate-io/terramate/cloud/testserver/cloudstore"
	. "github.com/terramate-io/terramate/cmd/terramate/e2etests/internal/runner"
	"github.com/terramate-io/terramate/test"
	"github.com/terramate-io/terramate/test/sandbox"
)

type listCloudTestcase struct {
	name       string
	layout     []string
	repository string
	stacks     []cloudstore.Stack
	flags      []string
	workingDir string
	perPage    int
	want       RunExpected
}

func TestCloudListUnhealthy(t *testing.T) {
	t.Parallel()

	for _, tc := range []listCloudTestcase{
		{
			name:       "local repository is not permitted with --cloud-status=",
			layout:     []string{"s:s1:id=s1"},
			repository: test.TempDir(t),
			flags:      []string{`--cloud-status=unhealthy`},
			want: RunExpected{
				Status:      1,
				StderrRegex: "unhealthy status filter does not work with filesystem based remotes",
			},
		},
		{
			name: "no cloud stacks, no status flag, return local stacks",
			layout: []string{
				"s:s1",
				"s:s2",
			},
			want: RunExpected{
				Stdout: nljoin("s1", "s2"),
			},
		},
		{
			name: "no cloud stacks, asking for unhealthy stacks: return nothing",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			flags: []string{"--cloud-status=unhealthy"},
		},
		{
			name: "1 cloud stack healthy, others absent, asking for unhealthy: return nothing",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.OK,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.OK,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
		},
		{
			name: "1 cloud stack healthy, others absent, asking for ok: return ok",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.OK,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.OK,
					},
				},
			},
			flags: []string{`--cloud-status=ok`},
			want: RunExpected{
				Stdout: nljoin("s1"),
			},
		},
		{
			name: "1 cloud stack ok, others absent, asking for healthy: return ok",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.OK,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.OK,
					},
				},
			},
			flags: []string{`--cloud-status=healthy`},
			want: RunExpected{
				Stdout: nljoin("s1"),
			},
		},
		{
			name: "1 cloud stack failed but different repository, asking for unhealthy: return nothing",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "gitlab.com/unknown-io/other",
					},
					State: cloudstore.StackState{
						Status:           stack.Failed,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.OK,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
		},
		{
			name: "1 cloud stack drifted, other absent, asking for unhealthy: return drifted",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Drifted,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.Drifted,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
			want: RunExpected{
				Stdout: nljoin("s1"),
			},
		},
		{
			name: "1 cloud stack failed, other absent, asking for failed: return failed",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Failed,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.Drifted,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
			want: RunExpected{
				Stdout: nljoin("s1"),
			},
		},
		{
			name: "1 cloud stack failed, other ok, asking for unhealthy: return failed",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Failed,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.OK,
					},
				},
				{
					Stack: cloud.Stack{
						MetaID:     "s2",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.OK,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.OK,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
			want: RunExpected{
				Stdout: nljoin("s1"),
			},
		},
		{
			name:   "no local stacks, 2 unhealthy stacks, return nothing",
			layout: []string{},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Failed,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.OK,
					},
				},
				{
					Stack: cloud.Stack{
						MetaID:     "s2",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Drifted,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.Drifted,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
		},
		{
			name: "2 local stacks, 2 same unhealthy stacks, return both",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Failed,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.OK,
					},
				},
				{
					Stack: cloud.Stack{
						MetaID:     "s2",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Drifted,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.Drifted,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
			want: RunExpected{
				Stdout: nljoin("s1", "s2"),
			},
		},
		{
			name: "stacks without id are ignored",
			layout: []string{
				"s:s1:id=s1",
				"s:s2:id=s2",
				"s:stack-without-id",
			},
			stacks: []cloudstore.Stack{
				{
					Stack: cloud.Stack{
						MetaID:     "s1",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Failed,
						DeploymentStatus: deployment.Failed,
						DriftStatus:      drift.OK,
					},
				},
				{
					Stack: cloud.Stack{
						MetaID:     "s2",
						Repository: "github.com/terramate-io/terramate",
					},
					State: cloudstore.StackState{
						Status:           stack.Drifted,
						DeploymentStatus: deployment.OK,
						DriftStatus:      drift.Drifted,
					},
				},
			},
			flags: []string{`--cloud-status=unhealthy`},
			want: RunExpected{
				Stdout: nljoin("s1", "s2"),
			},
		},
		paginationTestcase(10), // default per_page
		paginationTestcase(3),
		paginationTestcase(17),
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := cloudstore.LoadDatastore(testserverJSONFile)
			assert.NoError(t, err)
			addr := startFakeTMCServer(t, store)

			s := sandbox.New(t)
			s.BuildTree(tc.layout)
			repository := tc.repository
			if repository == "" {
				repository = "github.com/terramate-io/terramate"
			}
			s.Git().SetRemoteURL("origin", repository)
			if len(tc.layout) > 0 {
				s.Git().CommitAll("all stacks committed")
			}

			org := store.MustOrgByName("terramate")
			for _, st := range tc.stacks {
				_, err := store.UpsertStack(org.UUID, st)
				assert.NoError(t, err)
			}
			env := RemoveEnv(os.Environ(), "CI")
			env = append(env, "TMC_API_URL=http://"+addr, "CI=")
			if tc.perPage != 0 {
				env = append(env, "TMC_API_PAGESIZE="+strconv.Itoa(tc.perPage))
			}
			cli := NewCLI(t, filepath.Join(s.RootDir(), tc.workingDir), env...)
			args := []string{"list"}
			args = append(args, tc.flags...)
			result := cli.Run(args...)
			AssertRunResult(t, result, tc.want)
		})
	}
}

func paginationTestcase(perPage int) listCloudTestcase {
	const nstacks = 100

	var layout []string
	var stacks []cloudstore.Stack
	var names []string
	for i := 1; i <= nstacks; i++ {
		stackname := "s" + strconv.Itoa(i)
		names = append(names, stackname)
		layout = append(layout, "stack:"+stackname+":id="+stackname)
		stacks = append(stacks, cloudstore.Stack{
			Stack: cloud.Stack{
				MetaID:     stackname,
				Repository: "github.com/terramate-io/terramate",
			},
			State: cloudstore.StackState{
				Status:           stack.Failed,
				DeploymentStatus: deployment.Failed,
				DriftStatus:      drift.OK,
			},
		})
	}
	sort.Strings(names)
	return listCloudTestcase{
		name:    "paginated case",
		layout:  layout,
		stacks:  stacks,
		perPage: perPage,
		flags:   []string{`--cloud-status=unhealthy`},
		want: RunExpected{
			Stdout: nljoin(names...),
		},
	}
}
