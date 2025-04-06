package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
	internalapi "k8s.io/cri-api/pkg/apis"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
	"regexp"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"
)

type imageByRef []*pb.Image

func (a imageByRef) Len() int      { return len(a) }
func (a imageByRef) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a imageByRef) Less(i, j int) bool {
	if len(a[i].RepoTags) > 0 && len(a[j].RepoTags) > 0 {
		return a[i].RepoTags[0] < a[j].RepoTags[0]
	}

	if len(a[i].RepoDigests) > 0 && len(a[j].RepoDigests) > 0 {
		return a[i].RepoDigests[0] < a[j].RepoDigests[0]
	}

	return a[i].Id < a[j].Id
}

// ImageStatus sends an ImageStatusRequest to the server, and parses
// the returned ImageStatusResponse.
func ImageStatus(client internalapi.ImageManagerService, image string, verbose bool) (*pb.ImageStatusResponse, error) {
	request := &pb.ImageStatusRequest{
		Image:   &pb.ImageSpec{Image: image},
		Verbose: verbose,
	}
	logrus.Debugf("ImageStatusRequest: %v", request)

	res, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.ImageStatusResponse, error) {
		return client.ImageStatus(ctx, request.Image, request.Verbose)
	})
	if err != nil {
		return nil, err
	}

	logrus.Debugf("ImageStatusResponse: %v", res)

	return res, nil
}

func getAuth(creds, auth, username string) (*pb.AuthConfig, error) {
	if username != "" {
		fmt.Print("Enter Password:")

		bytePassword, err := term.ReadPassword(int(syscall.Stdin)) //nolint:unconvert // required for windows

		fmt.Print("\n")

		if err != nil {
			return nil, err
		}

		password := string(bytePassword)

		return &pb.AuthConfig{
			Username: username,
			Password: password,
		}, nil
	}

	if creds != "" && auth != "" {
		return nil, errors.New("both `--creds` and `--auth` are specified")
	}

	if creds != "" {
		username, password, err := parseCreds(creds)
		if err != nil {
			return nil, err
		}

		return &pb.AuthConfig{
			Username: username,
			Password: password,
		}, nil
	}

	if auth != "" {
		return &pb.AuthConfig{
			Auth: auth,
		}, nil
	}

	return nil, nil
}

func outputImageFsInfoTable(r *pb.ImageFsInfoResponse) {
	tablePrintFileSystem := func(fileLabel string, filesystem []*pb.FilesystemUsage) {
		fmt.Printf("%s Filesystem \n", fileLabel)

		for i, val := range filesystem {
			fmt.Printf("TimeStamp[%d]: %d\n", i, val.Timestamp)
			fmt.Printf("Disk[%d]: %s\n", i, units.HumanSize(float64(val.UsedBytes.GetValue())))
			fmt.Printf("Inodes[%d]: %d\n", i, val.InodesUsed.GetValue())
			fmt.Printf("Mountpoint[%d]: %s\n", i, val.FsId.Mountpoint)
		}
	}
	// otherwise output in table format
	tablePrintFileSystem("Container", r.ContainerFilesystems)
	tablePrintFileSystem("Image", r.ImageFilesystems)
}

func parseCreds(creds string) (username, password string, err error) {
	if creds == "" {
		return "", "", errors.New("credentials can't be empty")
	}

	up := strings.SplitN(creds, ":", 2)
	if len(up) == 1 {
		return up[0], "", nil
	}

	if up[0] == "" {
		return "", "", errors.New("username can't be empty")
	}

	return up[0], up[1], nil
}

func normalizeRepoDigest(repoDigests []string) (repo, digest string) {
	if len(repoDigests) == 0 {
		return "<none>", "<none>"
	}

	repoDigestPair := strings.Split(repoDigests[0], "@")
	if len(repoDigestPair) != 2 {
		return "errorName", "errorRepoDigest"
	}

	return repoDigestPair[0], repoDigestPair[1]
}

// PullImageWithSandbox sends a PullImageRequest to the server, and parses
// the returned PullImageResponse.
func PullImageWithSandbox(client internalapi.ImageManagerService, image string, auth *pb.AuthConfig, sandbox *pb.PodSandboxConfig, ann map[string]string, timeout time.Duration) (*pb.PullImageResponse, error) {
	request := &pb.PullImageRequest{
		Image: &pb.ImageSpec{
			Image:       image,
			Annotations: ann,
		},
		Auth:          auth,
		SandboxConfig: sandbox,
	}
	logrus.Debugf("PullImageRequest: %v", request)

	if timeout < 0 {
		return nil, errors.New("timeout should be bigger than 0")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if timeout > 0 {
		logrus.Debugf("Using pull context with timeout of %s", timeout)

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	res, err := InterruptableRPC(ctx, func(ctx context.Context) (string, error) {
		return client.PullImage(ctx, request.Image, request.Auth, request.SandboxConfig)
	})
	if err != nil {
		return nil, err
	}

	resp := &pb.PullImageResponse{ImageRef: res}
	logrus.Debugf("PullImageResponse: %v", resp)

	return resp, nil
}

// ListImages sends a ListImagesRequest to the server, and parses
// the returned ListImagesResponse.
func ListImages(client internalapi.ImageManagerService, nameFilter string, conditionFilters []string) (*pb.ListImagesResponse, error) {
	request := &pb.ListImagesRequest{Filter: &pb.ImageFilter{Image: &pb.ImageSpec{Image: nameFilter}}}
	logrus.Debugf("ListImagesRequest: %v", request)

	res, err := InterruptableRPC(context.Background(), func(ctx context.Context) ([]*pb.Image, error) {
		return client.ListImages(ctx, request.Filter)
	})
	if err != nil {
		return nil, err
	}

	resp := &pb.ListImagesResponse{Images: res}
	logrus.Debugf("ListImagesResponse: %v", resp)

	sort.Sort(imageByRef(resp.Images))

	if len(conditionFilters) > 0 && len(resp.Images) > 0 {
		resp.Images, err = filterImagesList(resp.Images, conditionFilters)
		if err != nil {
			return nil, fmt.Errorf("filter images: %w", err)
		}
	}

	return resp, nil
}

// Ideally repo tag should always be image:tag.
// The repoTags is nil when pulling image by repoDigest,Then we will show image name instead.
func normalizeRepoTagPair(repoTags []string, imageName string) (repoTagPairs [][]string) {
	const none = "<none>"
	if len(repoTags) == 0 {
		repoTagPairs = append(repoTagPairs, []string{imageName, none})

		return
	}

	for _, repoTag := range repoTags {
		idx := strings.LastIndex(repoTag, ":")
		if idx == -1 {
			repoTagPairs = append(repoTagPairs, []string{"errorRepoTag", "errorRepoTag"})

			continue
		}

		name := repoTag[:idx]
		if name == none {
			name = imageName
		}

		repoTagPairs = append(repoTagPairs, []string{name, repoTag[idx+1:]})
	}

	return
}

// filterImagesList filter images based on --filter flag.
func filterImagesList(imageList []*pb.Image, filters []string) ([]*pb.Image, error) {
	filtered := []*pb.Image{}
	filtered = append(filtered, imageList...)

	for _, filter := range filters {
		switch {
		case strings.HasPrefix(filter, "before="):
			reversedList := filtered
			slices.Reverse(reversedList)
			filtered = filterByBeforeSince(strings.TrimPrefix(filter, "before="), reversedList)
			slices.Reverse(filtered)
		case strings.HasPrefix(filter, "dangling="):
			filtered = filterByDangling(strings.TrimPrefix(filter, "dangling="), filtered)
		case strings.HasPrefix(filter, "reference="):
			var err error
			if filtered, err = filterByReference(strings.TrimPrefix(filter, "reference="), filtered); err != nil {
				return []*pb.Image{}, err
			}
		case strings.HasPrefix(filter, "since="):
			filtered = filterByBeforeSince(strings.TrimPrefix(filter, "since="), filtered)
		default:
			return []*pb.Image{}, fmt.Errorf("unknown filter flag: %s", filter)
		}
	}

	return filtered, nil
}

func filterByBeforeSince(filterValue string, imageList []*pb.Image) []*pb.Image {
	filtered := []*pb.Image{}

	for _, img := range imageList {
		// Filter by <image-name>[:<tag>]
		if strings.Contains(filterValue, ":") && !strings.Contains(filterValue, "@") {
			imageName, _ := normalizeRepoDigest(img.RepoDigests)

			repoTagPairs := normalizeRepoTagPair(img.RepoTags, imageName)
			if strings.Join(repoTagPairs[0], ":") == filterValue {
				break
			}

			filtered = append(filtered, img)
		}
		// Filter by <image id>
		if !strings.Contains(filterValue, ":") && !strings.Contains(filterValue, "@") {
			if strings.HasPrefix(img.Id, filterValue) {
				break
			}

			filtered = append(filtered, img)
		}
		// Filter by <image@sha>
		if strings.Contains(filterValue, ":") && strings.Contains(filterValue, "@") {
			if len(img.RepoDigests) > 0 {
				if strings.HasPrefix(img.RepoDigests[0], filterValue) {
					break
				}

				filtered = append(filtered, img)
			}
		}
	}

	return filtered
}

func filterByReference(filterValue string, imageList []*pb.Image) ([]*pb.Image, error) {
	filtered := []*pb.Image{}

	re, err := regexp.Compile(filterValue)
	if err != nil {
		return filtered, err
	}

	for _, img := range imageList {
		imgName, _ := normalizeRepoDigest(img.RepoDigests)
		if re.MatchString(imgName) || imgName == filterValue {
			filtered = append(filtered, img)
		}
	}

	return filtered, nil
}

func filterByDangling(filterValue string, imageList []*pb.Image) []*pb.Image {
	filtered := []*pb.Image{}

	for _, img := range imageList {
		if filterValue == "true" && len(img.RepoTags) == 0 {
			filtered = append(filtered, img)
		}

		if filterValue == "false" && len(img.RepoTags) > 0 {
			filtered = append(filtered, img)
		}
	}

	return filtered
}

// RemoveImage sends a RemoveImageRequest to the server, and parses
// the returned RemoveImageResponse.
func RemoveImage(client internalapi.ImageManagerService, image string) error {
	if image == "" {
		return errors.New("ImageID cannot be empty")
	}

	request := &pb.RemoveImageRequest{Image: &pb.ImageSpec{Image: image}}
	logrus.Debugf("RemoveImageRequest: %v", request)

	_, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.RemoveImageResponse, error) {
		return nil, client.RemoveImage(ctx, request.Image)
	})

	return err
}

// ImageFsInfo sends an ImageStatusRequest to the server, and parses
// the returned ImageFsInfoResponse.
func ImageFsInfo(client internalapi.ImageManagerService) (*pb.ImageFsInfoResponse, error) {
	res, err := InterruptableRPC(context.Background(), func(ctx context.Context) (*pb.ImageFsInfoResponse, error) {
		return client.ImageFsInfo(ctx)
	})
	if err != nil {
		return nil, err
	}

	resp := &pb.ImageFsInfoResponse{
		ImageFilesystems:     res.GetImageFilesystems(),
		ContainerFilesystems: res.GetContainerFilesystems(),
	}
	logrus.Debugf("ImageFsInfoResponse: %v", resp)

	return resp, nil
}
