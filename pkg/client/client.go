package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"reflect"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	"github.com/kubesaw/ksctl/pkg/configuration"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	"github.com/kubesaw/ksctl/pkg/ioutils"

	"github.com/ghodss/yaml"
	configv1 "github.com/openshift/api/config/v1"
	projectv1 "github.com/openshift/api/project/v1"
	routev1 "github.com/openshift/api/route/v1"
	templatev1 "github.com/openshift/api/template/v1"
	userv1 "github.com/openshift/api/user/v1"
	olmv1 "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	errs "github.com/pkg/errors"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
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

var DefaultNewClient = NewClient

func NewClient(token, apiEndpoint string) (runtimeclient.Client, error) {
	return NewClientWitTransport(token, apiEndpoint, &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // nolint: gosec
		},
	})
}

func NewClientWitTransport(token, apiEndpoint string, transport http.RoundTripper) (runtimeclient.Client, error) {
	if err := AddToScheme(); err != nil {
		return nil, err
	}
	cfg, err := clientcmd.BuildConfigFromFlags(apiEndpoint, "")
	if err != nil {
		return nil, err
	}

	cfg.Transport = transport
	cfg.BearerToken = string(token)
	cfg.QPS = 40.0
	cfg.Burst = 50
	cfg.Timeout = 60 * time.Second

	cl, err := runtimeclient.New(cfg, runtimeclient.Options{})
	if err != nil {
		return nil, errs.Wrap(err, "cannot create client")
	}

	return cl, nil
}

var DefaultNewRESTClient = NewRESTClient

func NewRESTClient(token, apiEndpoint string) (*rest.RESTClient, error) {
	if err := AddToScheme(); err != nil {
		return nil, err
	}
	config := &rest.Config{
		BearerToken: token,
		Host:        apiEndpoint,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // nolint: gosec
			},
		},
		Timeout: 60 * time.Second,
		// These fields need to be set when using the REST client ¯\_(ツ)_/¯
		ContentConfig: rest.ContentConfig{
			GroupVersion:         &authv1.SchemeGroupVersion,
			NegotiatedSerializer: scheme.Codecs,
		},
	}
	return rest.RESTClientFor(config)
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
	userSignup, err := GetUserSignup(cl, cfg.SandboxNamespace, name)
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
	mur, err := GetMasterUserRecord(cl, cfg.SandboxNamespace, name)
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
	space, err := GetSpace(cl, cfg.SandboxNamespace, name)
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
		Namespace: cfg.SandboxNamespace,
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
		Namespace: cfg.SandboxNamespace,
		Name:      name,
	}
	obj := &toolchainv1alpha1.UserTier{}
	if err := cl.Get(context.TODO(), namespacedName, obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// Ensure creates or updates the given object and returns if the object was either created or updated (which means
// that no error occurred and the administrator confirmed execution of the action)
func Ensure(term ioutils.Terminal, cl runtimeclient.Client, obj runtimeclient.Object) (bool, error) {
	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	return ensure(term, cl, namespacedName, obj)
}

func ensure(term ioutils.Terminal, cl runtimeclient.Client, namespacedName types.NamespacedName, obj runtimeclient.Object) (bool, error) {
	content, err := yaml.Marshal(obj)
	if err != nil {
		return false, err
	}
	resourceKind := obj.GetObjectKind().GroupVersionKind().Kind
	// if GVK is not defined in the given object, then let's use its type
	if resourceKind == "" {
		resourceKind = reflect.TypeOf(obj).Elem().Name()
	}
	term.PrintContextSeparatorWithBodyf(string(content), "Using %s resource:", resourceKind)

	existing := obj.DeepCopyObject().(runtimeclient.Object)
	if err := cl.Get(context.TODO(), namespacedName, existing); err != nil && !apierrors.IsNotFound(err) {
		return false, err
	} else if err == nil {
		term.Printlnf("There is an already existing %s with the same name: %s", resourceKind, namespacedName)
		if !term.AskForConfirmation(ioutils.WithDangerZoneMessagef(
			"update of the already created "+resourceKind, "update the %s with the hard-coded version?", resourceKind)) {
			return false, nil
		}
		metaNew, err := meta.Accessor(obj)
		if err != nil {
			return false, errs.Wrapf(err, "cannot get metadata from %+v", obj)
		}
		metaExisting, err := meta.Accessor(existing)
		if err != nil {
			return false, errs.Wrapf(err, "cannot get metadata from %+v", existing)
		}
		metaNew.SetResourceVersion(metaExisting.GetResourceVersion())
		// make sure that when updating a 'service' object, we retain its existing the `spec.clusterIP` field,
		// otherwise we get the following error: `Service "prometheus" is invalid: spec.clusterIP: Invalid value: "": field is immutable`
		if err := commonclient.RetainClusterIP(obj, existing); err != nil {
			return false, err
		}

		if err := cl.Update(context.TODO(), obj); err != nil {
			return false, err
		}
		term.Printlnf("\nThe '%s' %s has been updated", namespacedName.Name, resourceKind)
		return true, nil
	}

	if !term.AskForConfirmation(ioutils.WithMessagef("create the %s resource with the name %s ?", resourceKind, namespacedName)) {
		return false, nil
	}
	if err := cl.Create(context.TODO(), obj); err != nil {
		return false, err
	}
	term.Printlnf("\nThe '%s' %s has been created", namespacedName, resourceKind)
	return true, nil
}

// Create creates the resource only if it does not exist yet (ie, if a resource of the same kind in the same namespace/name doesn't exist yet)
func Create(term ioutils.Terminal, cl runtimeclient.Client, obj runtimeclient.Object) error {
	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	objCopy := obj.DeepCopyObject().(runtimeclient.Object)
	if err := cl.Get(context.TODO(), namespacedName, objCopy); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if apierrors.IsNotFound(err) {
		if err := cl.Create(context.TODO(), obj); err != nil {
			return err
		}
		term.Printlnf("\nThe '%s' %s has been created", namespacedName, reflect.TypeOf(obj).Elem().Name())
		return nil
	}
	term.Printlnf("\nThe '%s' %s already exists", namespacedName, reflect.TypeOf(obj).Elem().Name())
	return nil
}

// GetRouteURL return the scheme+host of the route with the given namespaced name.
// Since routes may take a bit of time to be available, this func uses a wait loop
// to make sure that the route was created, or fails after a timeout.
func GetRouteURL(term ioutils.Terminal, cl runtimeclient.Client, namespacedName types.NamespacedName) (string, error) {
	term.Printlnf("Waiting for '%s' route to be available...", namespacedName.Name)
	route := routev1.Route{}
	if err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := cl.Get(context.TODO(), namespacedName, &route); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
		if len(route.Status.Ingress) == 0 {
			return false, nil // route is not ready yet
		}
		return true, nil
	}); err != nil {
		return "", errs.Wrapf(err, "unable to get route to %s", namespacedName)
	}
	scheme := "https"
	if route.Spec.TLS == nil || *route.Spec.TLS == (routev1.TLSConfig{}) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, route.Spec.Host, route.Spec.Path), nil
}

var timeout = 5 * time.Second
var retryInterval = 200 * time.Millisecond
