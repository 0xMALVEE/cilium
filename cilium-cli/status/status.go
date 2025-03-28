// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cilium/cilium/api/v1/models"
	"github.com/cilium/cilium/cilium-cli/defaults"
	"github.com/cilium/cilium/pkg/lock"
)

const (
	Red     = "\033[31m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Green   = "\033[32m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	Reset   = "\033[0m"
)

const (
	OutputJSON    = "json"
	OutputSummary = "summary"
)

// MapCount is a map to count number of occurrences of a string
type MapCount map[string]int

// MapMapCount is a map of MapCount indexed by string
type MapMapCount map[string]MapCount

// PodStateCount counts the number of pods in the k8s cluster
type PodsCount struct {
	// All is the number of all pods in the k8s cluster
	All int `json:"all"`

	// ByCilium is the number of all the pods in the k8s cluster
	ByCilium int `json:"by_cilium"`
}

// PodStateCount counts the number of pods in a particular state
type PodStateCount struct {
	// Type is the type of deployment ("Deployment", "DaemonSet", ...)
	Type string

	// Desired is the number of desired pods to be scheduled
	Desired int

	// Ready is the number of ready pods
	Ready int

	// Available is the number of available pods
	Available int

	// Unavailable is the number of unavailable pods
	Unavailable int
}

type PodStateMap map[string]PodStateCount

type CiliumStatusMap map[string]*models.StatusResponse
type CiliumEndpointsMap map[string][]*models.Endpoint

type ErrorCount struct {
	Errors   []error
	Warnings []error
	Disabled bool
}

type ErrorCountMap map[string]*ErrorCount

type ErrorCountMapMap map[string]ErrorCountMap

// custom marshaling for ErrorCount
func (e *ErrorCount) MarshalJSON() ([]byte, error) {
	// create an alias to avoid recursion
	type Alias ErrorCount
	return json.Marshal(&struct {
		Errors   []string
		Warnings []string
		*Alias
	}{
		Errors:   errorsToStrings(e.Errors),
		Warnings: errorsToStrings(e.Warnings),
		Alias:    (*Alias)(e),
	})
}

func errorsToStrings(errs []error) []string {
	strs := make([]string, len(errs))
	for i, err := range errs {
		strs[i] = err.Error()
	}
	return strs
}

// Status is the overall status of Cilium
type Status struct {
	// ImageCount is a map counting the number of images in use indexed by
	// the image name
	ImageCount MapMapCount `json:"image_count,omitempty"`

	// PhaseCount is a map counting the number of pods in each phase
	// (running, failing, scheduled, ...)
	PhaseCount MapMapCount `json:"phase_count,omitempty"`

	// PodState counts the number of pods matching conditions such as
	// desired, ready, available, and unavailable
	PodState PodStateMap `json:"pod_state,omitempty"`

	// PodsCount is the number of pods in the k8s cluster
	// all pods, and pods managed by cilium
	PodsCount PodsCount `json:"pods_count,omitempty"`

	CiliumStatus CiliumStatusMap `json:"cilium_status,omitempty"`

	// CiliumEndpoints contains the information about the endpoints managed
	// by each Cilium agent.
	CiliumEndpoints CiliumEndpointsMap `json:"cilium_endpoints,omitempty"`

	// Errors is the aggregated errors and warnings of all pods of a
	// particular deployment type
	Errors ErrorCountMapMap `json:"errors,omitempty"`

	// CollectionErrors is the errors that accumulated while collecting the
	// status
	CollectionErrors []error `json:"collection_errors,omitempty"`

	// HelmChartVersion is the Helm chart version that is currently installed.
	// For Helm mode only.
	HelmChartVersion string `json:"helm_chart_version,omitempty"`

	ConfigErrors []string `json:"config_errors,omitempty"`

	mutex *lock.Mutex
}

func newStatus() *Status {
	return &Status{
		ImageCount:      MapMapCount{},
		PhaseCount:      MapMapCount{},
		PodState:        PodStateMap{},
		PodsCount:       PodsCount{},
		CiliumStatus:    CiliumStatusMap{},
		CiliumEndpoints: CiliumEndpointsMap{},
		Errors:          ErrorCountMapMap{},
		mutex:           &lock.Mutex{},
	}
}

func (s *Status) aggregatedErrorCount(deployment, pod string) *ErrorCount {
	m := s.Errors[deployment]
	if m == nil {
		m = ErrorCountMap{}
		s.Errors[deployment] = m
	}

	if m[pod] == nil {
		m[pod] = &ErrorCount{}
	}

	return m[pod]
}

func (s *Status) SetDisabled(deployment, pod string, disabled bool) {
	m := s.aggregatedErrorCount(deployment, pod)
	m.Disabled = disabled
}

func (s *Status) AddAggregatedError(deployment, pod string, err error) {
	m := s.aggregatedErrorCount(deployment, pod)
	m.Errors = append(m.Errors, err)
}

func (s *Status) AddAggregatedWarning(deployment, pod string, warning error) {
	m := s.aggregatedErrorCount(deployment, pod)
	m.Warnings = append(m.Warnings, warning)
}

func (s *Status) CollectionError(err error) {
	s.CollectionErrors = append(s.CollectionErrors, err)
}

func (s *Status) parseCiliumSubsystemState(deployment, podName, subsystem, state, msg string) {
	switch strings.ToLower(state) {
	case "warning":
		s.AddAggregatedWarning(deployment, podName, fmt.Errorf("%s: %s", subsystem, msg))
	case "failure":
		s.AddAggregatedError(deployment, podName, fmt.Errorf("%s: %s", subsystem, msg))
	}
}

func (s *Status) totalErrors() (total int) {
	for _, pods := range s.Errors {
		for _, pod := range pods {
			total += len(pod.Errors)
		}
	}
	return total
}

func (s *Status) totalWarnings() (total int) {
	for _, pods := range s.Errors {
		for _, pod := range pods {
			total += len(pod.Warnings)
		}
	}
	return total
}

func (s *Status) parseCiliumSubsystemStatus(deployment, podName, subsystem string, status *models.Status) {
	if status != nil {
		s.parseCiliumSubsystemState(deployment, podName, subsystem, status.State, status.Msg)
	}
}

func (s *Status) parseStatusResponse(deployment, podName string, r *models.StatusResponse, err error) {
	if err != nil {
		s.AddAggregatedError(deployment, podName, fmt.Errorf("unable to retrieve cilium status: %w", err))
		return
	}

	if r.Cilium != nil {
		s.parseCiliumSubsystemStatus(deployment, podName, "Cilium", r.Cilium)
	}

	if r.Cluster != nil {
		s.parseCiliumSubsystemStatus(deployment, podName, "Health", r.Cluster.CiliumHealth)
	}

	if r.Hubble != nil {
		s.parseCiliumSubsystemState(deployment, podName, "Hubble", r.Hubble.State, r.Hubble.Msg)
	}

	if r.Kubernetes != nil {
		s.parseCiliumSubsystemState(deployment, podName, "Kubernetes", r.Kubernetes.State, r.Kubernetes.Msg)
	}

	if r.Kvstore != nil {
		s.parseCiliumSubsystemStatus(deployment, podName, "Kvstore", r.Kvstore)
	}

	if r.AuthCertificateProvider != nil {
		s.parseCiliumSubsystemStatus(deployment, podName, "AuthCertificateProvider", r.AuthCertificateProvider)
	}

	if len(r.Controllers) > 0 {
		for _, ctrl := range r.Controllers {
			if ctrl.Status == nil || ctrl.Status.ConsecutiveFailureCount == 0 {
				continue
			}

			s.AddAggregatedError(deployment, podName, fmt.Errorf("controller %s is failing since %s (%dx): %s",
				ctrl.Name,
				time.Since(time.Time(ctrl.Status.LastFailureTimestamp)).Truncate(time.Second).String(),
				ctrl.Status.ConsecutiveFailureCount,
				ctrl.Status.LastFailureMsg))
		}
	}
}

func (s *Status) parseEndpointsResponse(deployment, podName string, eps []*models.Endpoint, err error) {
	if err != nil {
		s.AddAggregatedError(deployment, podName, fmt.Errorf("unable to retrieve cilium endpoint information: %w", err))
		return
	}

	var notReady uint
	for _, ep := range eps {
		if ep != nil && ep.Status != nil && ep.Status.State != nil &&
			*ep.Status.State != models.EndpointStateReady {
			notReady++
		}
	}

	if notReady > 0 {
		s.AddAggregatedWarning(deployment, podName, fmt.Errorf("%d endpoints are not ready", notReady))
	}
}

func (s *Status) statusSummary(name string) (text string) {
	var errors, warnings int
	if a := s.Errors[name]; a != nil {
		var disabled bool
		for _, c := range a {
			errors += len(c.Errors)
			warnings += len(c.Warnings)

			if c.Disabled {
				disabled = true
			}
		}

		var s []string
		if errors > 0 {
			s = append(s, Red+fmt.Sprintf("%d errors", errors)+Reset)
		}

		if warnings > 0 {
			s = append(s, Yellow+fmt.Sprintf("%d warnings", warnings)+Reset)
		}

		if disabled {
			s = append(s, Cyan+"disabled"+Reset)
		}

		text = strings.Join(s, ", ")
	}

	if text == "" {
		text = Green + "OK" + Reset
	}

	return
}

func formatPhaseCount(m MapCount) string {
	var items []string
	for phase, count := range m {
		color := ""
		switch phase {
		case "Failed", "Unknown":
			color = Red
		case "Running", "Pending", "Succeeded":
			color = Green
		}

		items = append(items, fmt.Sprintf("%s: "+color+"%d"+Reset, phase, count))
	}
	return strings.Join(items, ", ")
}

func formatPodsCount(count PodsCount) string {
	return fmt.Sprintf("%d/%d managed by Cilium", count.ByCilium, count.All)
}

func (c PodStateCount) Format() string {
	var items []string

	if c.Desired > 0 {
		items = append(items, fmt.Sprintf("Desired: %d", c.Desired))
	}

	if c.Ready > 0 {
		color := Green
		if c.Ready < c.Desired {
			color = Yellow
		}
		items = append(items, fmt.Sprintf("Ready: "+color+"%d/%d"+Reset, c.Ready, c.Desired))
	}

	if c.Available > 0 {
		color := Green
		if c.Ready < c.Desired {
			color = Yellow
		}
		items = append(items, fmt.Sprintf("Available: "+color+"%d/%d"+Reset, c.Available, c.Desired))
	}

	if c.Unavailable > 0 {
		items = append(items, fmt.Sprintf("Unavailable: "+Red+"%d/%d"+Reset, c.Unavailable, c.Desired))
	}

	return strings.Join(items, ", ")
}

func (s *Status) Format() string {
	if s == nil {
		return ""
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 4, ' ', 0)

	fmt.Fprint(w, Yellow+"    /¯¯\\\n")
	fmt.Fprint(w, Cyan+" /¯¯"+Yellow+"\\__/"+Green+"¯¯\\"+Reset+"\tCilium:\t"+s.statusSummary(defaults.AgentDaemonSetName)+"\n")
	fmt.Fprint(w, Cyan+" \\__"+Red+"/¯¯\\"+Green+"__/"+Reset+"\tOperator:\t"+s.statusSummary(defaults.OperatorDeploymentName)+"\n")
	fmt.Fprint(w, Green+" /¯¯"+Red+"\\__/"+Magenta+"¯¯\\"+Reset+"\tEnvoy DaemonSet:\t"+envoyStatusSummary(s.statusSummary(defaults.EnvoyDaemonSetName))+"\n")
	fmt.Fprint(w, Green+" \\__"+Blue+"/¯¯\\"+Magenta+"__/"+Reset+"\tHubble Relay:\t"+s.statusSummary(defaults.RelayDeploymentName)+"\n")
	fmt.Fprint(w, Blue+Blue+Blue+"    \\__/"+Reset+"\tClusterMesh:\t"+s.statusSummary(defaults.ClusterMeshDeploymentName)+"\n")
	fmt.Fprint(w, "\n")

	for _, name := range slices.Sorted(maps.Keys(s.PodState)) {
		podState := s.PodState[name]
		fmt.Fprintf(w, "%s\t%s\t%s\n", podState.Type, name, podState.Format())
	}

	header := "Containers:"
	for _, name := range slices.Sorted(maps.Keys(s.PhaseCount)) {
		fmt.Fprintf(w, "%s\t%s\t%s\n", header, name, formatPhaseCount(s.PhaseCount[name]))
		header = ""
	}

	fmt.Fprintf(w, "Cluster Pods:\t%s\n", formatPodsCount(s.PodsCount))

	fmt.Fprintf(w, "Helm chart version:\t%s\n", s.HelmChartVersion)

	header = "Image versions"
	for _, name := range slices.Sorted(maps.Keys(s.ImageCount)) {
		for _, image := range slices.Sorted(maps.Keys(s.ImageCount[name])) {
			fmt.Fprintf(w, "%s\t%s\t%s: %d\n", header, name, image, s.ImageCount[name][image])
			header = ""
		}
	}

	header = "Errors:"
	for _, deployment := range slices.Sorted(maps.Keys(s.Errors)) {
		pods := s.Errors[deployment]
		for _, pod := range slices.Sorted(maps.Keys(pods)) {
			for _, err := range pods[pod].Errors {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", header, deployment, pod, err)
				header = ""
			}
		}
	}

	header = "Warnings:"
	for _, deployment := range slices.Sorted(maps.Keys(s.Errors)) {
		pods := s.Errors[deployment]
		for _, pod := range slices.Sorted(maps.Keys(pods)) {
			for _, err := range pods[pod].Warnings {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", header, deployment, pod, err)
				header = ""
			}
		}
	}

	header = "Configuration:"
	for _, msg := range s.ConfigErrors {
		for line := range strings.SplitSeq(msg, "\n") {
			fmt.Fprintf(w, "%s\t \t%s\n", header, line)
			header = ""
		}
	}

	w.Flush()

	return buf.String()
}

// envoyStatusSummary adds some more context to the default `disabled` - mainly to prevent confusion.
// This might get removed once the DaemonSet mode becomes the only available option.
func envoyStatusSummary(statusSummary string) string {
	if strings.Contains(statusSummary, "disabled") {
		return strings.Replace(statusSummary, "disabled", "disabled (using embedded mode)", 1)
	}

	return statusSummary
}
