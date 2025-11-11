package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	configv1 "github.com/openshift/api/config/v1"
	projectv1 "github.com/openshift/api/project/v1"
	routev1 "github.com/openshift/api/route/v1"
	templatev1 "github.com/openshift/api/template/v1"
	userv1 "github.com/openshift/api/user/v1"
	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func AddToScheme() error {
	addToSchemes := append(runtime.SchemeBuilder{},
		toolchainv1alpha1.AddToScheme,
		olmv1.AddToScheme,
		olmv1alpha1.AddToScheme,
		rbacv1.AddToScheme,
		routev1.Install,
		userv1.Install,
		projectv1.Install,
		corev1.AddToScheme,
		configv1.Install,
		templatev1.Install)
	return addToSchemes.AddToScheme(scheme.Scheme)
}

var (
	DefaultNewClient               = NewClient
	DefaultNewClientFromRestConfig = NewClientFromRestConfig
)

func NewClient(token, apiEndpoint string) (runtimeclient.Client, error) {
	return NewClientWithTransport(token, apiEndpoint, newTlsVerifySkippingTransport())
}

func NewClientWithTransport(token, apiEndpoint string, transport http.RoundTripper) (runtimeclient.Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(apiEndpoint, "")
	if err != nil {
		return nil, err
	}

	cfg.Transport = transport
	cfg.BearerToken = token
	cfg.QPS = 40.0
	cfg.Burst = 50
	cfg.Timeout = 60 * time.Second

	return NewClientFromRestConfig(cfg)
}

func NewClientFromRestConfig(cfg *rest.Config) (runtimeclient.Client, error) {
	if err := AddToScheme(); err != nil {
		return nil, err
	}

	cl, err := runtimeclient.New(cfg, runtimeclient.Options{})
	if err != nil {
		return nil, fmt.Errorf("cannot create client: %w", err)
	}

	return cl, nil
}

// NewKubeClientFromKubeConfig initializes a runtime client starting from a KubeConfig file path.
func NewKubeClientFromKubeConfig(kubeConfigPath string) (cl runtimeclient.Client, err error) {
	var kubeConfig *clientcmdapi.Config
	var clientConfig *rest.Config

	kubeConfig, err = clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return cl, err
	}
	clientConfig, err = clientcmd.NewDefaultClientConfig(*kubeConfig, nil).ClientConfig()
	if err != nil {
		return cl, err
	}
	cl, err = NewClientFromRestConfig(clientConfig)
	if err != nil {
		return cl, err
	}

	return cl, err
}

func newTlsVerifySkippingTransport() http.RoundTripper {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // nolint: gosec
		},
	}
}

func PatchUserSignup(ctx *clicontext.CommandContext, name string, changeUserSignup func(*toolchainv1alpha1.UserSignup) (bool, error), afterMessage string) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	userSignup, err := GetUserSignup(cl, cfg.OperatorNamespace, name)
	if err != nil {
		return err
	}
	patched := userSignup.DeepCopy()
	if shouldUpdate, err := changeUserSignup(patched); !shouldUpdate || err != nil {
		return err
	}
	if err := cl.Patch(context.TODO(), patched, runtimeclient.MergeFrom(userSignup)); err != nil {
		return err
	}

	ctx.Printlnf(afterMessage)
	return nil
}

func GetUserSignup(cl runtimeclient.Client, namespace, name string) (*toolchainv1alpha1.UserSignup, error) {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	userSignup := &toolchainv1alpha1.UserSignup{}
	if err := cl.Get(context.TODO(), namespacedName, userSignup); err != nil {
		return nil, err
	}
	return userSignup, nil
}

func PatchMasterUserRecord(ctx *clicontext.CommandContext, name string, changeMasterUserRecord func(*toolchainv1alpha1.MasterUserRecord) (bool, error), afterMessage string) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	mur, err := GetMasterUserRecord(cl, cfg.OperatorNamespace, name)
	if err != nil {
		return err
	}
	patched := mur.DeepCopy()
	if shouldUpdate, err := changeMasterUserRecord(patched); !shouldUpdate || err != nil {
		return err
	}
	if err := cl.Patch(context.TODO(), patched, runtimeclient.MergeFrom(mur)); err != nil {
		return err
	}

	ctx.Printlnf(afterMessage)
	return nil
}

func GetMasterUserRecord(cl runtimeclient.Client, namespace, name string) (*toolchainv1alpha1.MasterUserRecord, error) {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	obj := &toolchainv1alpha1.MasterUserRecord{}
	if err := cl.Get(context.TODO(), namespacedName, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func PatchSpace(ctx *clicontext.CommandContext, name string, changeSpace func(*toolchainv1alpha1.Space) (bool, error), afterMessage string) error {
	cfg, err := configuration.LoadClusterConfig(ctx, configuration.HostName)
	if err != nil {
		return err
	}
	cl, err := ctx.NewClient(cfg.Token, cfg.ServerAPI)
	if err != nil {
		return err
	}
	space, err := GetSpace(cl, cfg.OperatorNamespace, name)
	if err != nil {
		return err
	}
	patched := space.DeepCopy()
	if shouldUpdate, err := changeSpace(patched); !shouldUpdate || err != nil {
		return err
	}
	if err := cl.Patch(context.TODO(), patched, runtimeclient.MergeFrom(space)); err != nil {
		return err
	}

	ctx.Printlnf(afterMessage)
	return nil
}

func GetSpace(cl runtimeclient.Client, namespace, name string) (*toolchainv1alpha1.Space, error) {
	namespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	obj := &toolchainv1alpha1.Space{}
	if err := cl.Get(context.TODO(), namespacedName, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

type SpaceBindingMatchingLabel func(runtimeclient.MatchingLabels)

func ForSpace(spaceName string) SpaceBindingMatchingLabel {
	return func(labels runtimeclient.MatchingLabels) {
		labels[toolchainv1alpha1.SpaceBindingSpaceLabelKey] = spaceName
	}
}

func ForMasterUserRecord(murName string) SpaceBindingMatchingLabel {
	return func(labels runtimeclient.MatchingLabels) {
		labels[toolchainv1alpha1.SpaceBindingMasterUserRecordLabelKey] = murName
	}
}

func ListSpaceBindings(cl runtimeclient.Client, namespace string, opts ...SpaceBindingMatchingLabel) ([]toolchainv1alpha1.SpaceBinding, error) {
	spacebindings := &toolchainv1alpha1.SpaceBindingList{}
	matchingLabels := runtimeclient.MatchingLabels{}
	for _, apply := range opts {
		apply(matchingLabels)
	}
	if err := cl.List(context.TODO(), spacebindings, runtimeclient.InNamespace(namespace), matchingLabels); err != nil {
		return nil, err
	}
	return spacebindings.Items, nil
}

func GetNSTemplateTier(cfg configuration.ClusterConfig, cl runtimeclient.Client, name string) (*toolchainv1alpha1.NSTemplateTier, error) {
	namespacedName := types.NamespacedName{
		Namespace: cfg.OperatorNamespace,
		Name:      name,
	}
	obj := &toolchainv1alpha1.NSTemplateTier{}
	if err := cl.Get(context.TODO(), namespacedName, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func GetUserTier(cfg configuration.ClusterConfig, cl runtimeclient.Client, name string) (*toolchainv1alpha1.UserTier, error) {
	namespacedName := types.NamespacedName{
		Namespace: cfg.OperatorNamespace,
		Name:      name,
	}
	obj := &toolchainv1alpha1.UserTier{}
	if err := cl.Get(context.TODO(), namespacedName, obj); err != nil {
		return nil, err
	}
	return obj, nil
}
