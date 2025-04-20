module github.com/nxsre/go-rsh

go 1.23.0

toolchain go1.24.1

require (
	code.cloudfoundry.org/tlsconfig v0.22.0
	github.com/alphadose/haxmap v1.4.1
	github.com/avast/retry-go/v4 v4.6.1
	github.com/denisbrodbeck/machineid v1.0.1
	github.com/gin-gonic/gin v1.10.0
	github.com/google/uuid v1.6.0
	github.com/jhump/grpctunnel v0.3.0
	github.com/kos-v/dsnparser v1.1.0
	github.com/mattn/go-shellwords v1.0.12
	google.golang.org/grpc v1.71.0
	k8s.io/klog/v2 v2.130.1
)

require (
	github.com/bytedance/sonic v1.12.7 // indirect
	github.com/bytedance/sonic/loader v0.2.2 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fullstorydev/grpchan v1.1.1 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.20.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	golang.org/x/arch v0.8.0 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/exp v0.0.0-20221031165847-c99f073a8326 // indirect
	golang.org/x/net v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/creack/pty v1.1.18
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-tty v0.0.7
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/term v0.30.0
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/protobuf v1.36.6
)

replace k8s.io/api => k8s.io/api v0.28.8

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.28.8

replace k8s.io/apimachinery => k8s.io/apimachinery v0.28.8

replace k8s.io/apiserver => k8s.io/apiserver v0.28.8

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.28.8

replace k8s.io/client-go => k8s.io/client-go v0.28.8

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.28.8

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.28.8

replace k8s.io/code-generator => k8s.io/code-generator v0.28.8

replace k8s.io/component-base => k8s.io/component-base v0.28.8

replace k8s.io/component-helpers => k8s.io/component-helpers v0.28.8

replace k8s.io/controller-manager => k8s.io/controller-manager v0.28.8

replace k8s.io/cri-api => k8s.io/cri-api v0.28.8

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.28.8

replace k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.28.8

replace k8s.io/endpointslice => k8s.io/endpointslice v0.28.8

replace k8s.io/kms => k8s.io/kms v0.28.8

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.28.8

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.28.8

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.28.8

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.28.8

replace k8s.io/kubectl => k8s.io/kubectl v0.28.8

replace k8s.io/kubelet => k8s.io/kubelet v0.28.8

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.28.8

replace k8s.io/metrics => k8s.io/metrics v0.28.8

replace k8s.io/mount-utils => k8s.io/mount-utils v0.28.8

replace k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.28.8

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.28.8

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.28.8

replace k8s.io/sample-controller => k8s.io/sample-controller v0.28.8
