package dcos

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/dcos/dcos-diagnostics/util"
)

// As per comments in https://jira.mesosphere.com/browse/COPS-4413
// calls to Mesos should be given relatively long timeouts to work reliably
const mesosHTTPTimeout = 10 * time.Second

// nodeFinder interface allows chain finding methods
type nodeFinder interface {
	Find() ([]Node, error)
}

// Find masters via dns. Used to Find master nodes from agents.
type findMastersInExhibitor struct {
	url  string
	next nodeFinder

	// getFn takes url and timeout and returns a read body, HTTP status code and error.
	getFn func(string, time.Duration) ([]byte, int, error)
}

type exhibitorNodeResponse struct {
	Code        int
	Description string
	Hostname    string
	IsLeader    bool
}

func (f *findMastersInExhibitor) findMesosMasters() (nodes []Node, err error) {
	if f.getFn == nil {
		return nodes, errors.New("could not initialize HTTP GET function. Make sure you set getFn in the constructor")
	}

	body, statusCode, err := f.getFn(f.url, mesosHTTPTimeout)
	if err != nil {
		return nodes, err
	}
	if statusCode != http.StatusOK {
		return nodes, fmt.Errorf("GET %s failed, status code: %d, body: %s", f.url, statusCode, body)
	}

	var exhibitorNodesResponse []exhibitorNodeResponse
	if err := json.Unmarshal(body, &exhibitorNodesResponse); err != nil {
		return nodes, err
	}
	if len(exhibitorNodesResponse) == 0 {
		return nodes, errors.New("master nodes not found in exhibitor")
	}

	for _, exhibitorNodeResponse := range exhibitorNodesResponse {
		nodes = append(nodes, Node{
			Role:   MasterRole,
			IP:     exhibitorNodeResponse.Hostname,
			Leader: exhibitorNodeResponse.IsLeader,
		})
	}
	return nodes, nil
}

func (f *findMastersInExhibitor) Find() (nodes []Node, err error) {
	nodes, err = f.findMesosMasters()
	if err == nil {
		logrus.Debug("Found masters in exhibitor")
		return nodes, nil
	}
	// try next provider if it is available
	if f.next != nil {
		logrus.Warning(err)
		return f.next.Find()
	}
	return nodes, err
}

// NodesNotFoundError is a custom error called when nodes are not found.
type NodesNotFoundError struct {
	msg string
}

func (n NodesNotFoundError) Error() string {
	return n.msg
}

// Find agents by resolving dns entry
type findNodesInDNS struct {
	forceTLS  bool
	dnsRecord string
	role      string
	next      nodeFinder

	// getFn takes url and timeout and returns a read body, HTTP status code and error.
	getFn func(string, time.Duration) ([]byte, int, error)
}

// Agent response json format
type agentsResponse struct {
	Agents []struct {
		Hostname   string `json:"hostname"`
		Attributes struct {
			PublicIP string `json:"public_ip"`
		} `json:"attributes"`
	} `json:"slaves"`
}

func (f *findNodesInDNS) resolveDomain() (ips []string, err error) {
	return net.LookupHost(f.dnsRecord)
}

func (f *findNodesInDNS) getMesosMasters() (nodes []Node, err error) {
	ips, err := f.resolveDomain()
	if err != nil {
		return nodes, err
	}
	if len(ips) == 0 {
		return nodes, errors.New("Could not resolve " + f.dnsRecord)
	}

	for _, ip := range ips {
		nodes = append(nodes, Node{
			Role: MasterRole,
			IP:   ip,
		})
	}
	return nodes, nil
}

func (f *findNodesInDNS) getMesosAgents() (nodes []Node, err error) {
	if f.getFn == nil {
		return nodes, errors.New("Could not initialize HTTP GET function. Make sure you set getFn in constractor")
	}
	leaderIps, err := f.resolveDomain()
	if err != nil {
		return nodes, err
	}
	if len(leaderIps) == 0 {
		return nodes, errors.New("Could not resolve " + f.dnsRecord)
	}

	url, err := util.UseTLSScheme(fmt.Sprintf("http://%s:5050/slaves", leaderIps[0]), f.forceTLS)
	if err != nil {
		return nodes, err
	}

	body, statusCode, err := f.getFn(url, mesosHTTPTimeout)
	if err != nil {
		return nodes, err
	}
	if statusCode != http.StatusOK {
		return nodes, fmt.Errorf("GET %s failed, status code %d, body: %s", url, statusCode, body)
	}

	var sr agentsResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nodes, err
	}

	for _, agent := range sr.Agents {
		role := AgentRole

		// if a node has "attributes": {"public_ip": "true"} we consider it to be a public agent
		if agent.Attributes.PublicIP == "true" {
			role = AgentPublicRole
		}
		nodes = append(nodes, Node{
			Role: role,
			IP:   agent.Hostname,
		})
	}
	return nodes, nil
}

func (f *findNodesInDNS) dispatchGetNodesByRole() (nodes []Node, err error) {
	if f.role == MasterRole {
		return f.getMesosMasters()
	}
	if f.role != AgentRole {
		return nodes, fmt.Errorf("%s role is incorrect, must be %s or %s", f.role, MasterRole, AgentRole)
	}
	return f.getMesosAgents()
}

func (f *findNodesInDNS) Find() (nodes []Node, err error) {
	nodes, err = f.dispatchGetNodesByRole()
	if err == nil {
		logrus.Debugf("Found %s nodes by resolving %s", f.role, f.dnsRecord)
		return nodes, err
	}
	if f.next != nil {
		logrus.Warning(err)
		return f.next.Find()
	}
	return nodes, err
}
