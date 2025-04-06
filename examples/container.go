package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/go-units"
	godigest "github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
	internalapi "k8s.io/cri-api/pkg/apis"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubelet/pkg/types"
	"log"
	goruntime "runtime"
	"sigs.k8s.io/cri-tools/pkg/framework"
	"sort"
	"strconv"
	"strings"
	"time"
)

type containerByCreated []*pb.Container

func (a containerByCreated) Len() int      { return len(a) }
func (a containerByCreated) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a containerByCreated) Less(i, j int) bool {
	return a[i].CreatedAt > a[j].CreatedAt
}

type createOptions struct {
	// podID of the container
	podID string

	// the config and pod options
	*runOptions
}

type runOptions struct {
	// configPath is path to the config for container
	configPath string

	// podConfig is path to the config for sandbox
	podConfig string

	// the create timeout
	timeout time.Duration

	// the image pull options
	*pullOptions
}

type pullOptions struct {
	// pull the image on container creation; overrides default
	withPull bool

	// creds is string in the format `USERNAME[:PASSWORD]` for accessing the
	// registry during image pull
	creds string

	// auth is a base64 encoded 'USERNAME[:PASSWORD]' string used for
	// authentication with a registry when pulling an image
	auth string

	// Username to use for accessing the registry
	// password will be requested on the command line
	username string

	// timeout is the maximum time used for the image pull
	timeout time.Duration
}

// RunContainer starts a container in the provided sandbox.
func RunContainer(
	iClient internalapi.ImageManagerService,
	rClient internalapi.RuntimeService,
	opts runOptions,
	runtime string,
) error {
	// Create the pod
	podSandboxConfig, err := loadPodSandboxConfig(opts.podConfig)
	if err != nil {
		return fmt.Errorf("load podSandboxConfig: %w", err)
	}
	// set the timeout for the RunPodSandbox request to 0, because the
	// timeout option is documented as being for container creation.
	podID, err := RunPodSandbox(rClient, podSandboxConfig, runtime)
	if err != nil {
		return fmt.Errorf("run pod sandbox: %w", err)
	}

	// Create the container
	containerOptions := createOptions{podID, &opts}

	ctrID, err := CreateContainer(iClient, rClient, containerOptions)
	if err != nil {
		return fmt.Errorf("creating container failed: %w", err)
	}

	// Start the container
	err = StartContainer(rClient, ctrID)
	if err != nil {
		return fmt.Errorf("starting the container %q: %w", ctrID, err)
	}

	return nil
}

// CreateContainer sends a CreateContainerRequest to the server, and parses
// the returned CreateContainerResponse.
func CreateContainer(
	iClient internalapi.ImageManagerService,
	rClient internalapi.RuntimeService,
	opts createOptions,
) (string, error) {
	config, err := loadContainerConfig(opts.configPath)
	if err != nil {
		return "", err
	}

	var podConfig *pb.PodSandboxConfig
	if opts.podConfig != "" {
		podConfig, err = loadPodSandboxConfig(opts.podConfig)
		if err != nil {
			return "", err
		}
	}

	image := config.GetImage().GetImage()
	if config.Image.UserSpecifiedImage == "" {
		config.Image.UserSpecifiedImage = image
	}

	// When there is a with-pull request or the image default mode is to
	// pull-image-on-create(true) and no-pull was not set we pull the image when
	// they ask for a create as a helper on the cli to reduce extra steps. As a
	// reminder if the image is already in cache only the manifest will be pulled
	// down to verify.
	if opts.withPull {
		auth, err := getAuth(opts.creds, opts.auth, opts.username)
		if err != nil {
			return "", err
		}

		// Try to pull the image before container creation
		images := []string{image}
		logrus.Infof("Pulling container image: %s", image)

		// Add possible OCI volume mounts
		for _, m := range config.Mounts {
			if m.Image != nil && m.Image.Image != "" {
				logrus.Infof("Pulling image %s to be mounted to container path: %s", image, m.ContainerPath)
				images = append(images, m.Image.Image)
			}
		}

		for _, image := range images {
			if _, err := PullImageWithSandbox(iClient, image, auth, podConfig, config.GetImage().GetAnnotations(), opts.pullOptions.timeout); err != nil {
				return "", err
			}
		}
	}

	request := &pb.CreateContainerRequest{
		PodSandboxId:  opts.podID,
		Config:        config,
		SandboxConfig: podConfig,
	}
	logrus.Debugf("CreateContainerRequest: %v", request)

	r, err := InterruptableRPC(context.Background(), func(ctx context.Context) (string, error) {
		return rClient.CreateContainer(ctx, opts.podID, config, podConfig)
	})
	logrus.Debugf("CreateContainerResponse: %v", r)

	if err != nil {
		return "", err
	}

	return r, nil
}

// StartContainer sends a StartContainerRequest to the server, and parses
// the returned StartContainerResponse.
func StartContainer(client internalapi.RuntimeService, id string) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	if _, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
		return nil, client.StartContainer(ctx, id)
	}); err != nil {
		return err
	}

	fmt.Println(id)

	return nil
}

type updateOptions struct {
	// (Windows only) Number of CPUs available to the container.
	CPUCount int64
	// (Windows only) Portion of CPU cycles specified as a percentage * 100.
	CPUMaximum int64
	// CPU CFS (Completely Fair Scheduler) period. Default: 0 (not specified).
	CPUPeriod int64
	// CPU CFS (Completely Fair Scheduler) quota. Default: 0 (not specified).
	CPUQuota int64
	// CPU shares (relative weight vs. other containers). Default: 0 (not specified).
	CPUShares int64
	// Memory limit in bytes. Default: 0 (not specified).
	MemoryLimitInBytes int64
	// OOMScoreAdj adjusts the oom-killer score. Default: 0 (not specified).
	OomScoreAdj int64
	// CpusetCpus constrains the allowed set of logical CPUs. Default: "" (not specified).
	CpusetCpus string
	// CpusetMems constrains the allowed set of memory nodes. Default: "" (not specified).
	CpusetMems string
}

// UpdateContainerResources sends an UpdateContainerResourcesRequest to the server, and parses
// the returned UpdateContainerResourcesResponse.
func UpdateContainerResources(client internalapi.RuntimeService, id string, opts *updateOptions) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	request := &pb.UpdateContainerResourcesRequest{
		ContainerId: id,
	}
	if goruntime.GOOS != framework.OSWindows {
		request.Linux = &pb.LinuxContainerResources{
			CpuPeriod:          opts.CPUPeriod,
			CpuQuota:           opts.CPUQuota,
			CpuShares:          opts.CPUShares,
			CpusetCpus:         opts.CpusetCpus,
			CpusetMems:         opts.CpusetMems,
			MemoryLimitInBytes: opts.MemoryLimitInBytes,
			OomScoreAdj:        opts.OomScoreAdj,
		}
	} else {
		request.Windows = &pb.WindowsContainerResources{
			CpuCount:           opts.CPUCount,
			CpuMaximum:         opts.CPUMaximum,
			CpuShares:          opts.CPUShares,
			MemoryLimitInBytes: opts.MemoryLimitInBytes,
		}
	}

	logrus.Debugf("UpdateContainerResourcesRequest: %v", request)
	resources := &pb.ContainerResources{Linux: request.Linux, Windows: request.Windows}

	if _, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
		return nil, client.UpdateContainerResources(ctx, id, resources)
	}); err != nil {
		return err
	}

	fmt.Println(id)

	return nil
}

// StopContainer sends a StopContainerRequest to the server, and parses
// the returned StopContainerResponse.
func StopContainer(client internalapi.RuntimeService, id string, timeout int64) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	logrus.Debugf("Stopping container: %s (timeout = %v)", id, timeout)

	if _, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
		return nil, client.StopContainer(ctx, id, timeout)
	}); err != nil {
		return err
	}

	fmt.Println(id)

	return nil
}

// CheckpointContainer sends a CheckpointContainerRequest to the server.
func CheckpointContainer(
	rClient internalapi.RuntimeService,
	id string,
	export string,
) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	request := &pb.CheckpointContainerRequest{
		ContainerId: id,
		Location:    export,
	}
	logrus.Debugf("CheckpointContainerRequest: %v", request)

	_, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.ImageFsInfoResponse, error) {
		return nil, rClient.CheckpointContainer(ctx, request)
	})
	if err != nil {
		return err
	}

	fmt.Println(id)

	return nil
}

// RemoveContainer sends a RemoveContainerRequest to the server, and parses
// the returned RemoveContainerResponse.
func RemoveContainer(client internalapi.RuntimeService, id string) error {
	if id == "" {
		return errors.New("ID cannot be empty")
	}

	logrus.Debugf("Removing container: %s", id)

	if _, err := InterruptableRPC(context.Background(), func(ctx context.Context) (any, error) {
		return nil, client.RemoveContainer(ctx, id)
	}); err != nil {
		return err
	}

	fmt.Println(id)

	return nil
}

// marshalContainerStatus converts container status into string and converts
// the timestamps into readable format.
func marshalContainerStatus(cs *pb.ContainerStatus) (string, error) {
	statusStr, err := protobufObjectToJSON(cs)
	if err != nil {
		return "", err
	}

	jsonMap := make(map[string]interface{})

	err = json.Unmarshal([]byte(statusStr), &jsonMap)
	if err != nil {
		return "", err
	}

	jsonMap["createdAt"] = time.Unix(0, cs.CreatedAt).Format(time.RFC3339Nano)

	var startedAt, finishedAt time.Time
	if cs.State != pb.ContainerState_CONTAINER_CREATED {
		// If container is not in the created state, we have tried and
		// started the container. Set the startedAt.
		startedAt = time.Unix(0, cs.StartedAt)
	}

	if cs.State == pb.ContainerState_CONTAINER_EXITED ||
		(cs.State == pb.ContainerState_CONTAINER_UNKNOWN && cs.FinishedAt > 0) {
		// If container is in the exit state, set the finishedAt.
		// Or if container is in the unknown state and FinishedAt > 0, set the finishedAt
		finishedAt = time.Unix(0, cs.FinishedAt)
	}

	jsonMap["startedAt"] = startedAt.Format(time.RFC3339Nano)
	jsonMap["finishedAt"] = finishedAt.Format(time.RFC3339Nano)

	return marshalMapInOrder(jsonMap, *cs)
}

// containerStatus sends a ContainerStatusRequest to the server, and parses
// the returned ContainerStatusResponse.
//
//nolint:dupl // pods and containers are similar, but still different
func containerStatus(client internalapi.RuntimeService, ids []string, output, tmplStr string, quiet bool) error {
	verbose := !(quiet)

	if output == "" { // default to json output
		output = outputTypeJSON
	}

	if len(ids) == 0 {
		return errors.New("ID cannot be empty")
	}

	statuses := []statusData{}

	for _, id := range ids {
		request := &pb.ContainerStatusRequest{
			ContainerId: id,
			Verbose:     verbose,
		}
		logrus.Debugf("ContainerStatusRequest: %v", request)

		r, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.ContainerStatusResponse, error) {
			return client.ContainerStatus(ctx, id, verbose)
		})
		logrus.Debugf("ContainerStatusResponse: %v", r)

		if err != nil {
			return fmt.Errorf("get container status: %w", err)
		}

		statusJSON, err := marshalContainerStatus(r.Status)
		if err != nil {
			return fmt.Errorf("marshal container status: %w", err)
		}

		if output == outputTypeTable {
			outputContainerStatusTable(r, verbose)
		} else {
			statuses = append(statuses, statusData{json: statusJSON, info: r.Info})
		}
	}

	return outputStatusData(statuses, output, tmplStr)
}

func outputContainerStatusTable(r *pb.ContainerStatusResponse, verbose bool) {
	fmt.Printf("ID: %s\n", r.Status.Id)

	if r.Status.Metadata != nil {
		if r.Status.Metadata.Name != "" {
			fmt.Printf("Name: %s\n", r.Status.Metadata.Name)
		}

		if r.Status.Metadata.Attempt != 0 {
			fmt.Printf("Attempt: %v\n", r.Status.Metadata.Attempt)
		}
	}

	fmt.Printf("State: %s\n", r.Status.State)
	ctm := time.Unix(0, r.Status.CreatedAt)
	fmt.Printf("Created: %v\n", units.HumanDuration(time.Now().UTC().Sub(ctm))+" ago")

	if r.Status.State != pb.ContainerState_CONTAINER_CREATED {
		stm := time.Unix(0, r.Status.StartedAt)
		fmt.Printf("Started: %v\n", units.HumanDuration(time.Now().UTC().Sub(stm))+" ago")
	}

	if r.Status.State == pb.ContainerState_CONTAINER_EXITED {
		if r.Status.FinishedAt > 0 {
			ftm := time.Unix(0, r.Status.FinishedAt)
			fmt.Printf("Finished: %v\n", units.HumanDuration(time.Now().UTC().Sub(ftm))+" ago")
		}

		fmt.Printf("Exit Code: %v\n", r.Status.ExitCode)
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

// ListContainers sends a ListContainerRequest to the server, and parses
// the returned ListContainerResponse.
func ListContainers(runtimeClient internalapi.RuntimeService, imageClient internalapi.ImageManagerService, opts *listOptions) ([]*pb.Container, error) {
	filter := &pb.ContainerFilter{}
	if opts.id != "" {
		filter.Id = opts.id
	}

	if opts.podID != "" {
		filter.PodSandboxId = opts.podID
	}

	st := &pb.ContainerStateValue{}
	if !opts.all && opts.state == "" {
		st.State = pb.ContainerState_CONTAINER_RUNNING
		filter.State = st
	}

	if opts.state != "" {
		st.State = pb.ContainerState_CONTAINER_UNKNOWN

		switch strings.ToLower(opts.state) {
		case "created":
			st.State = pb.ContainerState_CONTAINER_CREATED
			filter.State = st
		case "running":
			st.State = pb.ContainerState_CONTAINER_RUNNING
			filter.State = st
		case "exited":
			st.State = pb.ContainerState_CONTAINER_EXITED
			filter.State = st
		case "unknown":
			st.State = pb.ContainerState_CONTAINER_UNKNOWN
			filter.State = st
		default:
			log.Fatalf("--state should be one of created, running, exited or unknown")
		}
	}

	if opts.latest || opts.last > 0 {
		// Do not filter by state if latest/last is specified.
		filter.State = nil
	}

	if opts.labels != nil {
		filter.LabelSelector = opts.labels
	}

	r, err := InterruptableRPC(context.Background(), func(ctx context.Context) ([]*pb.Container, error) {
		return runtimeClient.ListContainers(ctx, filter)
	})
	logrus.Debugf("ListContainerResponse: %v", r)

	if err != nil {
		return nil, fmt.Errorf("call list containers RPC: %w", err)
	}

	return getContainersList(imageClient, r, opts)
}

// OutputContainers sends a ListContainerRequest to the server, and parses
// the returned ListContainerResponse for output.
func OutputContainers(runtimeClient internalapi.RuntimeService, imageClient internalapi.ImageManagerService, opts *listOptions) error {
	r, err := ListContainers(runtimeClient, imageClient, opts)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	switch opts.output {
	case outputTypeJSON:
		return outputProtobufObjAsJSON(&pb.ListContainersResponse{Containers: r})
	case outputTypeYAML:
		return outputProtobufObjAsYAML(&pb.ListContainersResponse{Containers: r})
	case outputTypeTable:
	// continue; output will be generated after the switch block ends.
	default:
		return fmt.Errorf("unsupported output format %q", opts.output)
	}

	display := newDefaultTableDisplay()
	if !opts.verbose && !opts.quiet {
		display.AddRow([]string{columnContainer, columnImage, columnCreated, columnState, columnName, columnAttempt, columnPodID, columnPodName, columnNamespace})
	}

	for _, c := range r {
		if opts.quiet {
			fmt.Printf("%s\n", c.Id)

			continue
		}

		createdAt := time.Unix(0, c.CreatedAt)
		ctm := units.HumanDuration(time.Now().UTC().Sub(createdAt)) + " ago"
		podNamespace := getPodNamespaceFromLabels(c.Labels)

		if !opts.verbose {
			id := c.Id
			podID := c.PodSandboxId

			var image string
			if c.Image != nil {
				image = c.Image.Image
			}

			if !opts.noTrunc {
				id = getTruncatedID(id, "")
				podID = getTruncatedID(podID, "")
				// Now c.Image.Image is imageID in kubelet.
				if digest, err := godigest.Parse(image); err == nil {
					image = getTruncatedID(digest.String(), string(digest.Algorithm())+":")
				}
			}

			if opts.resolveImagePath {
				orig, err := getRepoImage(imageClient, image)
				if err != nil {
					return fmt.Errorf("failed to fetch repo image %w", err)
				}

				image = orig
			}

			podName := getPodNameFromLabels(c.Labels)
			display.AddRow([]string{
				id, image, ctm, convertContainerState(c.State), c.Metadata.Name,
				strconv.FormatUint(uint64(c.Metadata.Attempt), 10), podID, podName, podNamespace,
			})

			continue
		}

		fmt.Printf("ID: %s\n", c.Id)
		fmt.Printf("PodID: %s\n", c.PodSandboxId)
		fmt.Printf("Namespace: %s\n", podNamespace)

		if c.Metadata != nil {
			if c.Metadata.Name != "" {
				fmt.Printf("Name: %s\n", c.Metadata.Name)
			}

			fmt.Printf("Attempt: %v\n", c.Metadata.Attempt)
		}

		fmt.Printf("State: %s\n", convertContainerState(c.State))

		if c.Image != nil {
			fmt.Printf("Image: %s\n", c.Image.Image)
		}

		fmt.Printf("Created: %v\n", ctm)

		if c.Labels != nil {
			fmt.Println("Labels:")

			for _, k := range getSortedKeys(c.Labels) {
				fmt.Printf("\t%s -> %s\n", k, c.Labels[k])
			}
		}

		if c.Annotations != nil {
			fmt.Println("Annotations:")

			for _, k := range getSortedKeys(c.Annotations) {
				fmt.Printf("\t%s -> %s\n", k, c.Annotations[k])
			}
		}

		fmt.Println()
	}

	display.Flush()

	return nil
}

func convertContainerState(state pb.ContainerState) string {
	switch state {
	case pb.ContainerState_CONTAINER_CREATED:
		return "Created"
	case pb.ContainerState_CONTAINER_RUNNING:
		return "Running"
	case pb.ContainerState_CONTAINER_EXITED:
		return "Exited"
	case pb.ContainerState_CONTAINER_UNKNOWN:
		return "Unknown"
	default:
		log.Fatalf("Unknown container state %q", state)

		return ""
	}
}

func getPodNameFromLabels(labels map[string]string) string {
	return getFromLabels(labels, types.KubernetesPodNameLabel)
}

func getPodNamespaceFromLabels(labels map[string]string) string {
	return getFromLabels(labels, types.KubernetesPodNamespaceLabel)
}

func getFromLabels(labels map[string]string, label string) string {
	value, ok := labels[label]
	if ok {
		return value
	}

	return "unknown"
}

func getContainersList(imageClient internalapi.ImageManagerService, containersList []*pb.Container, opts *listOptions) ([]*pb.Container, error) {
	filtered := []*pb.Container{}

	for _, c := range containersList {
		if match, err := matchesImage(imageClient, opts.image, c.GetImage().GetImage()); err != nil {
			return nil, fmt.Errorf("check image match: %w", err)
		} else if !match {
			continue
		}

		podNamespace := getPodNamespaceFromLabels(c.Labels)
		// Filter by pod name/namespace regular expressions.
		if c.Metadata != nil &&
			matchesRegex(opts.nameRegexp, c.Metadata.Name) &&
			matchesRegex(opts.podNamespaceRegexp, podNamespace) {
			filtered = append(filtered, c)
		}
	}

	sort.Sort(containerByCreated(filtered))

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

	return filtered[:n], nil
}
