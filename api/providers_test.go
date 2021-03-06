package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadCollectors(t *testing.T) {
	t.Parallel()
	tools := new(MockedTools)

	tools.On("GetNodeRole").Return("master", nil)
	if runtime.GOOS != GoosWindows && runtime.GOOS != GoosDarwin {
		tools.On("GetUnitNames").Return([]string{"dcos-diagnostics"}, nil)
	}
	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.NoError(t, err)

	if runtime.GOOS != GoosWindows && runtime.GOOS != GoosDarwin {
		assert.Len(t, got, 15)
	} else {
		assert.Len(t, got, 14)
	}
	expected := []string{
		"5050-master_state-summary.json",
		"5050-registrar_1__registry.json",
		"uri_not_avail.txt",
		"5050-system_stats_json.json",
		"dcos-diagnostics-health.json",
		"var/lib/dcos/exhibitor/zookeeper/snapshot/myid",
		"var/lib/dcos/exhibitor/conf/zoo.cfg",
		"not/existing/file",
		"dmesg_-T.output",
		"ps_aux_ww_Z.output",
		"binsh_-c_cat etc*-release.output",
		"systemctl_list-units_dcos*.output",
		"echo_OK.output",
		"does_not_exist.output",
	}
	if runtime.GOOS != GoosWindows && runtime.GOOS != GoosDarwin {
		expected = append([]string{"dcos-diagnostics"}, expected...)
	}
	for i, c := range got {
		assert.Equal(t, expected[i], c.Name())
	}
}

func TestLoadCollectors_GetNodeRoleErrors(t *testing.T) {
	t.Parallel()
	tools := new(MockedTools)

	tools.On("GetNodeRole").Return("master", errors.New("some error"))
	tools.On("GetUnitNames").Return([]string{"dcos-diagnostics"}, nil)

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.EqualError(t, err, "could not get role: some error")
	assert.Empty(t, got)
}

func TestLoadCollectors_GetUnitNamesErrors(t *testing.T) {
	if runtime.GOOS == GoosWindows || runtime.GOOS == GoosDarwin {
		t.Skip("skipping test; GetUnitNames is not called on Windows.")
	}

	t.Parallel()
	tools := new(MockedTools)

	tools.On("GetUnitNames").Return([]string{}, errors.New("some error"))

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.EqualError(t, err, "could load systemd collectors: could not get unit names: some error")
	assert.Empty(t, got)
}

func TestLoadCollectors_GetNodeRoleReturnsInvalidRole(t *testing.T) {
	t.Parallel()
	tools := new(MockedTools)

	tools.On("GetNodeRole").Return("invalid", nil)
	tools.On("GetUnitNames").Return([]string{"dcos-diagnostics"}, nil)

	cfg := testCfg()
	cfg.FlagDiagnosticsBundleEndpointsConfigFiles = []string{
		filepath.Join("testdata", "endpoint-config.json"),
	}

	got, err := LoadCollectors(cfg, tools, http.DefaultClient)

	assert.EqualError(t, err, "incorrect role invalid, must be: master, agent or agent_public")
	assert.Empty(t, got)
}
