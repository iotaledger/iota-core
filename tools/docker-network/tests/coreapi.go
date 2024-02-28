package tests

import (
	"testing"
)

func (d *DockerTestFramework) requestFromClients(testFunc func(*testing.T, string)) {
	for alias := range d.nodes {
		testFunc(d.Testing, alias)
	}
}
