package pool

import (
	"github.com/criyle/go-sandbox/container"
	"github.com/criyle/go-sandbox/pkg/cgroup"
)

// EnvironmentBuilder defines the abstract builder for container environment
type EnvironmentBuilder interface {
	Build() (container.Environment, error)
}

// CgroupBuilder builds cgroup for runner
type CgroupBuilder interface {
	Build() (cg *cgroup.Cgroup, err error)
}
