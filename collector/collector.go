package collector

import (
	"bytes"
	"context"
	"fmt"
	goio "io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/dcos/dcos-diagnostics/io"
	"github.com/dcos/dcos-diagnostics/units"
)

// Collector is the interface to abstract data collection from different sources
type Collector interface {
	// Name returns the Name of this collector
	Name() string
	// Optional returns true if Collector is not mandatory and failures should be ignored
	Optional() bool
	// Collect returns collected data
	Collect(ctx context.Context) (goio.ReadCloser, error)
}

// Cmd is a struct implementing Collector interface. It collects command output for given command configured with Cmd field
type Cmd struct {
	name     string
	optional bool
	cmd      []string
}

func NewCmd(name string, optional bool, cmd []string) *Cmd {
	return &Cmd{
		name:     name,
		optional: optional,
		cmd:      cmd,
	}
}

func (c Cmd) Name() string {
	return c.name
}

func (c Cmd) Optional() bool {
	return c.optional
}

func (c Cmd) Collect(ctx context.Context) (goio.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, c.cmd[0], c.cmd[1:]...)
	output, err := cmd.CombinedOutput()
	return ioutil.NopCloser(bytes.NewReader(output)), err
}

// Systemd is a struct implementing Collector interface. It collects journal logs for given unit
type Systemd struct {
	name     string
	optional bool
	unitName string
	duration time.Duration
}

func NewSystemd(name string, optional bool, unitName string, duration time.Duration) *Systemd {
	return &Systemd{
		name:     name,
		optional: optional,
		unitName: unitName,
		duration: duration,
	}
}

func (c Systemd) Name() string {
	return c.name
}

func (c Systemd) Optional() bool {
	return c.optional
}

func (c Systemd) Collect(ctx context.Context) (goio.ReadCloser, error) {
	rc, err := units.ReadJournalOutputSince(ctx, c.unitName, c.duration)

	if err != nil {
		return nil, fmt.Errorf("could not read %s logs from journal: %s", c.unitName, err)
	}

	return rc, err
}

// Endpoint is a struct implementing Collector interface. It collects HTTP response for given url
type Endpoint struct {
	name     string
	optional bool
	client   *http.Client
	url      string
}

func NewEndpoint(name string, optional bool, url string, client *http.Client) *Endpoint {
	return &Endpoint{
		name:     name,
		optional: optional,
		url:      url,
		client:   client,
	}
}

func (c Endpoint) Name() string {
	return c.name
}

func (c Endpoint) Optional() bool {
	return c.optional
}

func (c Endpoint) Collect(ctx context.Context) (goio.ReadCloser, error) {
	url := c.url
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create a new HTTP request: %s", err)
	}
	request = request.WithContext(ctx)

	resp, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not fetch url %s: %s", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		errMsg := fmt.Sprintf("unable to fetch %s. Return code %d.", url, resp.StatusCode)

		body, e := ioutil.ReadAll(resp.Body)
		if e != nil {
			return nil, fmt.Errorf("%s Could not read body: %s", errMsg, e)
		}

		return nil, fmt.Errorf("%s Body: %s", errMsg, string(body))
	}

	return resp.Body, err
}

type File struct {
	name     string
	optional bool
	filePath string
}

func NewFile(name string, optional bool, filePath string) *File {
	return &File{
		name:     name,
		optional: optional,
		filePath: filePath,
	}
}

func (c File) Name() string {
	return c.name
}

func (c File) Optional() bool {
	return c.optional
}

func (c File) Collect(ctx context.Context) (goio.ReadCloser, error) {
	r, err := os.Open(c.filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %s", c.Name(), err)
	}
	return io.ReadCloserWithContext(ctx, r), nil
}
