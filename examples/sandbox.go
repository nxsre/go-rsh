package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	internalapi "k8s.io/cri-api/pkg/apis"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sandboxByCreated []*pb.PodSandbox

func (a sandboxByCreated) Len() int      { return len(a) }
func (a sandboxByCreated) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a sandboxByCreated) Less(i, j int) bool {
	return a[i].CreatedAt > a[j].CreatedAt
}

// RunPodSandbox sends a RunPodSandboxRequest to the server, and parses
// the returned RunPodSandboxResponse.
func RunPodSandbox(client internalapi.RuntimeService, config *pb.PodSandboxConfig, runtime string) (string, error) {
	request := &pb.RunPodSandboxRequest{
		Config:         config,
		RuntimeHandler: runtime,
	}
	logrus.Debugf("RunPodSandboxRequest: %v", request)

	r, err := InterruptableRPC(context.Background(), func(ctx context.Context) (string, error) {
		return client.RunPodSandbox(ctx, config, runtime)
	})
	logrus.Debugf("RunPodSandboxResponse: %v", r)

	if err != nil {
		return "", err
	}

	return r, nil
}

// StopPodSandbox sends a StopPodSandboxRequest to the server, and parses
// the returned StopPodSandboxResponse.
func StopPodSandbox(client internalapi.RuntimeService, id string) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	logrus.Debugf("Stopping pod sandbox: %s", id)

	if _, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
		return nil, client.StopPodSandbox(ctx, id)
	}); err != nil {
		return err
	}

	fmt.Printf("Stopped sandbox %s\n", id)

	return nil
}

// RemovePodSandbox sends a RemovePodSandboxRequest to the server, and parses
// the returned RemovePodSandboxResponse.
func RemovePodSandbox(client internalapi.RuntimeService, id string) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	logrus.Debugf("Removing pod sandbox: %s", id)

	if _, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
		return nil, client.RemovePodSandbox(ctx, id)
	}); err != nil {
		return err
	}

	fmt.Printf("Removed sandbox %s\n", id)

	return nil
}

// marshalPodSandboxStatus converts pod sandbox status into string and converts
// the timestamps into readable format.
func marshalPodSandboxStatus(ps *pb.PodSandboxStatus) (string, error) {
	statusStr, err := protobufObjectToJSON(ps)
	if err != nil {
		return "", err
	}

	jsonMap := make(map[string]interface{})

	err = json.Unmarshal([]byte(statusStr), &jsonMap)
	if err != nil {
		return "", err
	}

	jsonMap["createdAt"] = time.Unix(0, ps.CreatedAt).Format(time.RFC3339Nano)

	return marshalMapInOrder(jsonMap, *ps)
}

// podSandboxStatus sends a PodSandboxStatusRequest to the server, and parses
// the returned PodSandboxStatusResponse.
//
//nolint:dupl // pods and containers are similar, but still different
func podSandboxStatus(client internalapi.RuntimeService, ids []string, output string, quiet bool, tmplStr string) error {
	verbose := !(quiet)

	if output == "" { // default to json output
		output = outputTypeJSON
	}

	if len(ids) == 0 {
		return errors.New("ID cannot be empty")
	}

	statuses := []statusData{}

	for _, id := range ids {
		request := &pb.PodSandboxStatusRequest{
			PodSandboxId: id,
			Verbose:      verbose,
		}
		logrus.Debugf("PodSandboxStatusRequest: %v", request)

		r, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.PodSandboxStatusResponse, error) {
			return client.PodSandboxStatus(ctx, id, verbose)
		})

		logrus.Debugf("PodSandboxStatusResponse: %v", r)

		if err != nil {
			return fmt.Errorf("get pod sandbox status: %w", err)
		}

		statusJSON, err := marshalPodSandboxStatus(r.Status)
		if err != nil {
			return fmt.Errorf("marshal pod sandbox status: %w", err)
		}

		if output == outputTypeTable {
			outputPodSandboxStatusTable(r, verbose)
		} else {
			statuses = append(statuses, statusData{json: statusJSON, info: r.Info})
		}
	}

	return outputStatusData(statuses, output, tmplStr)
}

func outputPodSandboxStatusTable(r *pb.PodSandboxStatusResponse, verbose bool) {
	// output in table format by default.
	fmt.Printf("ID: %s\n", r.Status.Id)

	if r.Status.Metadata != nil {
		if r.Status.Metadata.Name != "" {
			fmt.Printf("Name: %s\n", r.Status.Metadata.Name)
		}

		if r.Status.Metadata.Uid != "" {
			fmt.Printf("UID: %s\n", r.Status.Metadata.Uid)
		}

		if r.Status.Metadata.Namespace != "" {
			fmt.Printf("Namespace: %s\n", r.Status.Metadata.Namespace)
		}

		fmt.Printf("Attempt: %v\n", r.Status.Metadata.Attempt)
	}

	fmt.Printf("Status: %s\n", r.Status.State)
	ctm := time.Unix(0, r.Status.CreatedAt)
	fmt.Printf("Created: %v\n", ctm)

	if r.Status.Network != nil {
		fmt.Printf("IP Addresses: %v\n", r.Status.Network.Ip)

		for _, ip := range r.Status.Network.AdditionalIps {
			fmt.Printf("Additional IP: %v\n", ip.Ip)
		}
	}

	if r.Status.Labels != nil {
		fmt.Println("Labels:")

		for _, k := range getSortedKeys(r.Status.Labels) {
			fmt.Printf("\t%s -> %s\n", k, r.Status.Labels[k])
		}
	}

	if r.Status.Annotations != nil {
		fmt.Println("Annotations:")

		for _, k := range getSortedKeys(r.Status.Annotations) {
			fmt.Printf("\t%s -> %s\n", k, r.Status.Annotations[k])
		}
	}

	if verbose {
		fmt.Printf("Info: %v\n", r.GetInfo())
	}
}

// ListPodSandboxes sends a ListPodSandboxRequest to the server, and parses
// the returned ListPodSandboxResponse.
func ListPodSandboxes(client internalapi.RuntimeService, opts *listOptions) ([]*pb.PodSandbox, error) {
	filter := &pb.PodSandboxFilter{}
	if opts.id != "" {
		filter.Id = opts.id
	}

	if opts.state != "" {
		st := &pb.PodSandboxStateValue{}
		st.State = pb.PodSandboxState_SANDBOX_NOTREADY

		switch strings.ToLower(opts.state) {
		case "ready":
			st.State = pb.PodSandboxState_SANDBOX_READY
			filter.State = st
		case "notready":
			st.State = pb.PodSandboxState_SANDBOX_NOTREADY
			filter.State = st
		default:
			log.Fatalf("--state should be ready or notready")
		}
	}

	if opts.labels != nil {
		filter.LabelSelector = opts.labels
	}

	request := &pb.ListPodSandboxRequest{
		Filter: filter,
	}
	logrus.Debugf("ListPodSandboxRequest: %v", request)

	r, err := InterruptableRPC(context.Background(), func(ctx context.Context) ([]*pb.PodSandbox, error) {
		return client.ListPodSandbox(ctx, filter)
	})
	logrus.Debugf("ListPodSandboxResponse: %v", r)

	if err != nil {
		return nil, fmt.Errorf("call list sandboxes RPC: %w", err)
	}

	return getSandboxesList(r, opts), nil
}

// OutputPodSandboxes sends a ListPodSandboxRequest to the server, and parses
// the returned ListPodSandboxResponse for output.
func OutputPodSandboxes(client internalapi.RuntimeService, opts *listOptions) error {
	r, err := ListPodSandboxes(client, opts)
	if err != nil {
		return fmt.Errorf("list pod sandboxes: %w", err)
	}

	switch opts.output {
	case outputTypeJSON:
		return outputProtobufObjAsJSON(&pb.ListPodSandboxResponse{Items: r})
	case outputTypeYAML:
		return outputProtobufObjAsYAML(&pb.ListPodSandboxResponse{Items: r})
	case outputTypeTable:
	// continue; output will be generated after the switch block ends.
	default:
		return fmt.Errorf("unsupported output format %q", opts.output)
	}

	display := newDefaultTableDisplay()
	if !opts.verbose && !opts.quiet {
		display.AddRow([]string{
			columnPodID,
			columnCreated,
			columnState,
			columnName,
			columnNamespace,
			columnAttempt,
			columnPodRuntime,
		})
	}

	c := cases.Title(language.Und)

	for _, pod := range r {
		if opts.quiet {
			fmt.Printf("%s\n", pod.Id)

			continue
		}

		if !opts.verbose {
			createdAt := time.Unix(0, pod.CreatedAt)
			ctm := units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"

			id := pod.Id
			if !opts.noTrunc {
				id = getTruncatedID(id, "")
			}

			display.AddRow([]string{
				id,
				ctm,
				convertPodState(pod.State),
				pod.Metadata.Name,
				pod.Metadata.Namespace,
				strconv.FormatUint(uint64(pod.Metadata.Attempt), 10),
				getSandboxesRuntimeHandler(pod),
			})

			continue
		}

		fmt.Printf("ID: %s\n", pod.Id)

		if pod.Metadata != nil {
			if pod.Metadata.Name != "" {
				fmt.Printf("Name: %s\n", pod.Metadata.Name)
			}

			if pod.Metadata.Uid != "" {
				fmt.Printf("UID: %s\n", pod.Metadata.Uid)
			}

			if pod.Metadata.Namespace != "" {
				fmt.Printf("Namespace: %s\n", pod.Metadata.Namespace)
			}

			if pod.Metadata.Attempt != 0 {
				fmt.Printf("Attempt: %v\n", pod.Metadata.Attempt)
			}
		}

		fmt.Printf("Status: %s\n", convertPodState(pod.State))
		ctm := time.Unix(0, pod.CreatedAt)
		fmt.Printf("Created: %v\n", ctm)

		if pod.Labels != nil {
			fmt.Println("Labels:")

			for _, k := range getSortedKeys(pod.Labels) {
				fmt.Printf("\t%s -> %s\n", k, pod.Labels[k])
			}
		}

		if pod.Annotations != nil {
			fmt.Println("Annotations:")

			for _, k := range getSortedKeys(pod.Annotations) {
				fmt.Printf("\t%s -> %s\n", k, pod.Annotations[k])
			}
		}

		fmt.Printf("%s: %s\n",
			c.String(columnPodRuntime),
			getSandboxesRuntimeHandler(pod))

		fmt.Println()
	}

	display.Flush()

	return nil
}

func convertPodState(state pb.PodSandboxState) string {
	switch state {
	case pb.PodSandboxState_SANDBOX_READY:
		return "Ready"
	case pb.PodSandboxState_SANDBOX_NOTREADY:
		return "NotReady"
	default:
		log.Fatalf("Unknown pod state %q", state)

		return ""
	}
}

func getSandboxesRuntimeHandler(sandbox *pb.PodSandbox) string {
	if sandbox.RuntimeHandler == "" {
		return "(default)"
	}

	return sandbox.RuntimeHandler
}

func getSandboxesList(sandboxesList []*pb.PodSandbox, opts *listOptions) []*pb.PodSandbox {
	filtered := []*pb.PodSandbox{}

	for _, p := range sandboxesList {
		// Filter by pod name/namespace regular expressions.
		if p.Metadata != nil && matchesRegex(opts.nameRegexp, p.Metadata.Name) &&
			matchesRegex(opts.podNamespaceRegexp, p.Metadata.Namespace) {
			filtered = append(filtered, p)
		}
	}

	sort.Sort(sandboxByCreated(filtered))

	n := len(filtered)
	if opts.latest {
		n = 1
	}

	if opts.last > 0 {
		n = opts.last
	}

	n = func(a, b int) int {
		if a < b {
			return a
		}

		return b
	}(n, len(filtered))

	return filtered[:n]
}
