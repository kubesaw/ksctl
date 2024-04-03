package generate

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubesaw/ksctl/pkg/configuration"
	v1 "github.com/openshift/api/user/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestEnsureObject(t *testing.T) {
	// given
	for _, clusterType := range configuration.ClusterTypes {
		t.Run("for cluster type "+clusterType.String(), func(t *testing.T) {

			t.Run("for User object", func(t *testing.T) {
				verifyEnsureManifest(t, clusterType, &v1.User{})
			})

			t.Run("for ServiceAccount object", func(t *testing.T) {
				verifyEnsureManifest(t, clusterType, &corev1.ServiceAccount{})
			})
		})
	}
}

func prepareObjects(t *testing.T, name string, namespace string, object runtimeclient.Object) (runtimeclient.Object, runtimeclient.Object) {
	gvks, _, err := scheme.Scheme.ObjectKinds(object)
	require.NoError(t, err)
	require.Len(t, gvks, 1)

	toBeStored := object.DeepCopyObject().(runtimeclient.Object)
	if gvks[0].Kind != "User" {
		toBeStored.SetNamespace(namespace)
	}
	toBeStored.SetName(name)

	expectedWithTypeMeta := toBeStored.DeepCopyObject().(runtimeclient.Object)
	expectedWithTypeMeta.GetObjectKind().SetGroupVersionKind(gvks[0])

	return toBeStored, expectedWithTypeMeta
}

func verifyEnsureManifest(t *testing.T, clusterType configuration.ClusterType, object runtimeclient.Object) {
	for _, namespace := range []string{"johnspace", "second-namespace", ""} {
		t.Run("for namespace "+namespace, func(t *testing.T) {
			// given
			ctx := newSetupContextWithDefaultFiles(t, nil)
			cache := objectsCache{}
			toBeStored, expected := prepareObjects(t, "john", namespace, object)

			// when
			err := cache.ensureObject(newFakeClusterContext(ctx, clusterType), toBeStored, nil)

			// then
			require.NoError(t, err)
			actual := object.DeepCopyObject().(runtimeclient.Object)
			inObjectCache(t, ctx.outDir, clusterType.String(), cache).
				assertObject(toBeStored.GetNamespace(), "john", actual, func() {
					assert.Equal(t, expected, actual)
				})

			verifyUpdates(t, newFakeClusterContext(ctx, clusterType), cache, object, toBeStored, expected, clusterType.String())

			t.Run("second resource", func(t *testing.T) {
				// given
				toBeStored2, expected2 := prepareObjects(t, "second", namespace, object)

				// when
				err := cache.ensureObject(newFakeClusterContext(ctx, clusterType), toBeStored2, nil)

				// then
				require.NoError(t, err)
				actual := object.DeepCopyObject().(runtimeclient.Object)
				inObjectCache(t, ctx.outDir, clusterType.String(), cache).
					assertObject(toBeStored.GetNamespace(), "second", actual, func() {
						assert.Equal(t, expected2, actual)
					})

				t.Run("no change when update function fails", func(t *testing.T) {
					// when
					err := cache.ensureObject(newFakeClusterContext(ctx, clusterType), toBeStored2, func(object runtimeclient.Object) (bool, error) {
						object.SetLabels(map[string]string{"dummy-key": "dummy-value"})
						return true, fmt.Errorf("some errror")
					})

					// then
					require.Error(t, err)
					actual := object.DeepCopyObject().(runtimeclient.Object)
					inObjectCache(t, ctx.outDir, clusterType.String(), cache).
						assertObject(toBeStored.GetNamespace(), "second", actual, func() {
							assert.Equal(t, expected2, actual)
						})
				})
			})

			t.Run("fails for missing name", func(t *testing.T) {
				// given
				invalid := expected.DeepCopyObject().(runtimeclient.Object)
				invalid.SetName("")

				// when
				err := cache.ensureObject(newFakeClusterContext(ctx, clusterType), invalid.DeepCopyObject().(runtimeclient.Object), nil)

				// then
				require.Error(t, err)
			})

			t.Run("when applied for the other cluster type", func(t *testing.T) {
				t.Run("single-cluster mode disabled", func(t *testing.T) {
					// given
					toBeStored, expected := prepareObjects(t, "john", namespace, object)
					cache := objectsCache{}
					require.NoError(t, cache.ensureObject(newFakeClusterContext(ctx, clusterType), toBeStored, nil))

					// when
					err := cache.ensureObject(newFakeClusterContext(ctx, clusterType.TheOtherType()), toBeStored, nil)

					// then
					require.NoError(t, err)
					actual := object.DeepCopyObject().(runtimeclient.Object)
					inObjectCache(t, ctx.outDir, clusterType.String(), cache).
						assertObject(toBeStored.GetNamespace(), "john", actual, func() {
							assert.Equal(t, expected, actual)
						})
					actual2 := object.DeepCopyObject().(runtimeclient.Object)
					inObjectCache(t, ctx.outDir, clusterType.TheOtherType().String(), cache).
						assertObject(toBeStored.GetNamespace(), "john", actual2, func() {
							assert.Equal(t, expected, actual2)
						})
					inObjectCache(t, ctx.outDir, "base", cache).
						assertObjectDoesNotExist(toBeStored.GetNamespace(), "john", object)
				})

				t.Run("single-cluster mode enabled", func(t *testing.T) {
					// given
					ctx := newSetupContextWithDefaultFiles(t, nil)
					ctx.setupFlags.singleCluster = true

					t.Run("update after move to base", func(t *testing.T) {
						// given
						toBeStored, expected := prepareObjects(t, "john", namespace, object)
						cache := objectsCache{}
						require.NoError(t, cache.ensureObject(newFakeClusterContext(ctx, clusterType), toBeStored, nil))

						// when
						err := cache.ensureObject(newFakeClusterContext(ctx, clusterType.TheOtherType()), toBeStored, nil)

						// then
						require.NoError(t, err)
						inObjectCache(t, ctx.outDir, clusterType.String(), cache).
							assertObjectDoesNotExist(toBeStored.GetNamespace(), "john", object)
						inObjectCache(t, ctx.outDir, clusterType.TheOtherType().String(), cache).
							assertObjectDoesNotExist(toBeStored.GetNamespace(), "john", object)
						baseActual := object.DeepCopyObject().(runtimeclient.Object)
						inObjectCache(t, ctx.outDir, "base", cache).
							assertObject(toBeStored.GetNamespace(), "john", baseActual, func() {
								assert.Equal(t, expected, baseActual)
							})

						verifyUpdates(t, newFakeClusterContext(ctx, clusterType), cache, object, toBeStored, expected, "base")
					})

					t.Run("update while moving to base", func(t *testing.T) {
						// given
						toBeStored, expected := prepareObjects(t, "john", namespace, object)
						modifiedSA := expected.DeepCopyObject().(runtimeclient.Object)
						modifiedSA.SetLabels(map[string]string{"dummy-key": "dummy-value"})
						cache := objectsCache{}
						require.NoError(t, cache.ensureObject(newFakeClusterContext(ctx, clusterType), toBeStored, nil))

						// when
						err := cache.ensureObject(newFakeClusterContext(ctx, clusterType.TheOtherType()), toBeStored, func(object runtimeclient.Object) (bool, error) {
							object.SetLabels(map[string]string{"dummy-key": "dummy-value"})
							return true, nil
						})

						// then
						require.NoError(t, err)
						inObjectCache(t, ctx.outDir, clusterType.String(), cache).
							assertObjectDoesNotExist(toBeStored.GetNamespace(), "john", object)
						inObjectCache(t, ctx.outDir, clusterType.TheOtherType().String(), cache).
							assertObjectDoesNotExist(toBeStored.GetNamespace(), "john", object)
						baseActual := object.DeepCopyObject().(runtimeclient.Object)
						inObjectCache(t, ctx.outDir, "base", cache).
							assertObject(toBeStored.GetNamespace(), "john", baseActual, func() {
								assert.Equal(t, modifiedSA, baseActual)
							})
					})
				})
			})
		})
	}
}

func verifyUpdates(t *testing.T, ctx *clusterContext, cache objectsCache, object, toBeStored, expected runtimeclient.Object, expRootDir string) {
	t.Run("when manifest should not be updated", func(t *testing.T) {

		for _, noUpdateFunc := range []func(runtimeclient.Object) (bool, error){nil, func(object runtimeclient.Object) (bool, error) {
			object.SetLabels(map[string]string{"dummy-key": "dummy-value"})
			return false, nil
		}} {
			// when
			err := cache.ensureObject(ctx, toBeStored, noUpdateFunc)

			// then
			require.NoError(t, err)
			actual := object.DeepCopyObject().(runtimeclient.Object)
			inObjectCache(t, ctx.outDir, expRootDir, cache).
				assertObject(toBeStored.GetNamespace(), "john", actual, func() {
					assert.Equal(t, expected, actual)
				})
		}

		t.Run("when manifest should be updated", func(t *testing.T) {
			// given
			modifiedSA := expected.DeepCopyObject().(runtimeclient.Object)
			modifiedSA.SetLabels(map[string]string{"dummy-key": "dummy-value"})

			// when
			err := cache.ensureObject(ctx, toBeStored, func(object runtimeclient.Object) (bool, error) {
				object.SetLabels(map[string]string{"dummy-key": "dummy-value"})
				return true, nil
			})

			// then
			require.NoError(t, err)
			actual := object.DeepCopyObject().(runtimeclient.Object)
			inObjectCache(t, ctx.outDir, expRootDir, cache).
				assertObject(toBeStored.GetNamespace(), "john", actual, func() {
					assert.Equal(t, modifiedSA, actual)
				})
		})
	})
}

func TestWriteManifests(t *testing.T) {
	// given
	ctx := newSetupContextWithDefaultFiles(t, nil)
	cache := objectsCache{}
	for _, clusterType := range configuration.ClusterTypes {
		for _, namespace := range []string{"johnspace", "second-namespace", ""} {
			clusterCtx := newFakeClusterContext(ctx, clusterType.TheOtherType())
			user, _ := prepareObjects(t, "john", namespace, &v1.User{})
			require.NoError(t, cache.storeObject(clusterCtx, user))
			sa, _ := prepareObjects(t, "john", namespace, &corev1.ServiceAccount{})
			require.NoError(t, cache.storeObject(clusterCtx, sa))
		}
	}

	// when
	err := cache.writeManifests(ctx)

	// then
	require.NoError(t, err)
	for path, expObject := range cache {
		obj, err := scheme.Scheme.New(expObject.GetObjectKind().GroupVersionKind())
		require.NoError(t, err)
		object := obj.(runtimeclient.Object)
		assertObjectAsFile(t, path, expObject.GetNamespace(), expObject.GetName(), object, func() {
			assert.Equal(t, expObject, object)
		})

		splitPath := strings.Split(strings.TrimPrefix(path, ctx.outDir), string(filepath.Separator))
		assertKustomizationFiles(t, ctx.outDir, splitPath[1], path)
	}
}

func TestWriteManifest(t *testing.T) {
	for _, rootDir := range []string{"host", "member", "base"} {
		t.Run("for root dir "+rootDir, func(t *testing.T) {
			// given
			ctx := newSetupContextWithDefaultFiles(t, nil)
			path := filepath.Join(ctx.outDir, rootDir, "test", "resource.yaml")
			_, expectedObject := prepareObjects(t, "john", "john-comp", &corev1.ServiceAccount{})

			// when
			err := writeManifest(ctx, path, expectedObject)

			// then
			require.NoError(t, err)
			sa := &corev1.ServiceAccount{}
			assertObjectAsFile(t, path, expectedObject.GetNamespace(), expectedObject.GetName(), sa, func() {
				assert.Equal(t, expectedObject, sa)
			})

			splitPath := strings.Split(strings.TrimPrefix(path, ctx.outDir), string(filepath.Separator))
			assertKustomizationFiles(t, ctx.outDir, splitPath[1], path)
		})
	}
}
