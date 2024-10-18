package generate

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/apis"
	commontest "github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/kubesaw/ksctl/pkg/ioutils"
	v1 "github.com/openshift/api/template/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var expectedTiers = []string{
	"appstudio",
	"appstudiolarge",
	"appstudio-env",
}

func sourceTierName(tier string) string {
	if tier == "appstudiolarge" {
		return "appstudio"
	}
	return tier
}

func nsTypes(tier string) []string {
	switch tier {
	case "appstudio", "appstudiolarge":
		return []string{"tenant"}
	case "appstudio-env":
		return []string{"env"}
	default:
		return []string{"not-expected"}
	}
}

func roles(tier string) []string {
	switch tier {
	case "appstudio", "appstudio-env", "appstudiolarge":
		return []string{"admin", "maintainer", "contributor"}
	default:
		return []string{"not-expected"}
	}
}

func TestGenerateNSTemplateTiers(t *testing.T) {

	s := scheme.Scheme
	err := apis.AddToScheme(s)
	require.NoError(t, err)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	buffy := bytes.NewBuffer(nil)
	term := ioutils.NewTerminal(buffy, buffy)

	for _, tierToUpdate := range []string{"appstudio", "appstudio-env"} {
		outTempDir, err := os.MkdirTemp("", "generate-tiers-test-outdir-")
		require.NoError(t, err)
		t.Logf("out directory: %s", outTempDir)
		sourceDir, err := os.MkdirTemp("", "generate-tiers-test-source-")
		require.NoError(t, err)
		t.Logf("source directory: %s", sourceDir)
		copyTemplates(t, sourceDir, "")

		// when
		err = NSTemplateTiers(term, sourceDir, outTempDir, commontest.HostOperatorNs)

		// then
		require.NoError(t, err)

		templateRefs := verifyTierFiles(t, outTempDir, sourceDir, "", nil)

		t.Run(fmt.Sprintf("when '%s' tier templates are modified", tierToUpdate), func(t *testing.T) {
			// given
			copyTemplates(t, sourceDir, tierToUpdate)

			// when
			err = NSTemplateTiers(term, sourceDir, outTempDir, commontest.HostOperatorNs)

			// then
			require.NoError(t, err)

			verifyTierFiles(t, outTempDir, sourceDir, tierToUpdate, &templateRefs)
		})
	}

	t.Run("failed to read files", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "generate-tiers-test-")
		require.NoError(t, err)

		// when
		err = NSTemplateTiers(term, "/does/not/exist", outTempDir, commontest.HostOperatorNs)

		// then
		require.Error(t, err)
		assert.Equal(t, "lstat /does/not/exist: no such file or directory", err.Error()) // error occurred while creating TierTemplate resources
	})

	t.Run("failed to process wrong files", func(t *testing.T) {
		// given
		outTempDir, err := os.MkdirTemp("", "generate-tiers-test-")
		require.NoError(t, err)

		// when
		err = NSTemplateTiers(term, "../../../test-resources/dummy.openshiftapps.com/", outTempDir, commontest.HostOperatorNs)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unable to init NSTemplateTier generator")
	})
}

type templateRefs struct {
	clusterResourcesTmplRef map[string]string
	namespaceTmplRefs       map[string][]string
	spaceRoleTmplRefs       map[string]map[string]string
}

func verifyTierFiles(t *testing.T, outTempDir, sourceDir, updatedTier string, oldTemplateRefs *templateRefs) templateRefs {

	nstemplateTiers, err := inKStructure(t, outTempDir, "").listObjects("", "NSTemplateTier", &toolchainv1alpha1.NSTemplateTier{})
	require.NoError(t, err)
	expectedTemplateRefs := templateRefs{
		clusterResourcesTmplRef: map[string]string{},
		namespaceTmplRefs:       map[string][]string{},
		spaceRoleTmplRefs:       map[string]map[string]string{},
	}
	require.Len(t, nstemplateTiers, len(expectedTiers))
	for _, tier := range expectedTiers {
		t.Run("TierTemplate for tier "+tier, func(t *testing.T) {
			for _, nsTypeName := range nsTypes(tier) {
				t.Run("ns-"+nsTypeName, func(t *testing.T) {
					oldTemplateRef := ""
					if updatedTier == sourceTierName(tier) {
						for _, nsRef := range oldTemplateRefs.namespaceTmplRefs[tier] {
							if strings.Contains(nsRef, nsTypeName) {
								oldTemplateRef = nsRef
								break
							}
						}
						require.NotEmpty(t, oldTemplateRef)
					}
					sourceFile := filepath.Join(sourceDir, tier, fmt.Sprintf("ns_%s.yaml", nsTypeName))
					templateName := verifyTierTemplate(t, outTempDir, tier, nsTypeName, sourceFile, oldTemplateRef)
					expectedTemplateRefs.namespaceTmplRefs[tier] = append(expectedTemplateRefs.namespaceTmplRefs[tier], templateName)
				})
			}
			for _, role := range roles(tier) {
				t.Run("spacerole-"+role, func(t *testing.T) {
					oldTemplateRef := ""
					if updatedTier == sourceTierName(tier) {
						oldTemplateRef = oldTemplateRefs.spaceRoleTmplRefs[tier][role]
					}
					sourceFile := filepath.Join(sourceDir, tier, fmt.Sprintf("spacerole_%s.yaml", role))
					roleName := verifyTierTemplate(t, outTempDir, tier, role, sourceFile, oldTemplateRef)
					if expectedTemplateRefs.spaceRoleTmplRefs[tier] == nil {
						expectedTemplateRefs.spaceRoleTmplRefs[tier] = map[string]string{}
					}
					expectedTemplateRefs.spaceRoleTmplRefs[tier][role] = roleName
				})
			}
			t.Run("clusterresources", func(t *testing.T) {
				oldTemplateRef := ""
				if updatedTier == sourceTierName(tier) {
					oldTemplateRef = oldTemplateRefs.clusterResourcesTmplRef[tier]
				}
				sourceFile := filepath.Join(sourceDir, tier, "cluster.yaml")
				templateName := verifyTierTemplate(t, outTempDir, tier, "clusterresources", sourceFile, oldTemplateRef)
				expectedTemplateRefs.clusterResourcesTmplRef[tier] = templateName
			})
		})
	}
	// verify that each NSTemplateTier has the ClusterResources, Namespaces and SpaceRoles `TemplateRef` set as expectedTemplateRefs

	for _, tierName := range expectedTiers {
		t.Run("NSTemplateTier for tier "+tierName, func(t *testing.T) {
			// verify tier configuration
			tierTemplates, err := inKStructure(t, outTempDir, "").listObjects(tierName, "NSTemplateTier", &toolchainv1alpha1.NSTemplateTier{})
			require.NoError(t, err)
			require.Len(t, tierTemplates, 1)
			nsTmplTier := tierTemplates[0].(*toolchainv1alpha1.NSTemplateTier)
			require.Equal(t, tierName, nsTmplTier.Name)

			require.NotNil(t, nsTmplTier.Spec.ClusterResources)
			assert.Equal(t, expectedTemplateRefs.clusterResourcesTmplRef[nsTmplTier.Name], nsTmplTier.Spec.ClusterResources.TemplateRef)
			actualNamespaceTmplRefs := []string{}
			for _, ns := range nsTmplTier.Spec.Namespaces {
				actualNamespaceTmplRefs = append(actualNamespaceTmplRefs, ns.TemplateRef)
			}
			assert.ElementsMatch(t, expectedTemplateRefs.namespaceTmplRefs[nsTmplTier.Name], actualNamespaceTmplRefs)

			require.Len(t, nsTmplTier.Spec.SpaceRoles, len(expectedTemplateRefs.spaceRoleTmplRefs[nsTmplTier.Name]))
			for role, templateRef := range expectedTemplateRefs.spaceRoleTmplRefs[nsTmplTier.Name] {
				assert.Equal(t, nsTmplTier.Spec.SpaceRoles[role].TemplateRef, templateRef)
			}
		})
	}
	return expectedTemplateRefs
}

func copyTemplates(t *testing.T, destination, tierToUpdate string) {
	sourceDir, err := filepath.Abs("../../../test-resources/nstemplatetiers/")
	require.NoError(t, err)
	err = filepath.WalkDir(sourceDir, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		suffix, _ := strings.CutPrefix(path, sourceDir)
		newPath := filepath.Join(destination, suffix)
		if dirEntry.IsDir() {
			return os.MkdirAll(newPath, 0744)
		}
		file, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if tierToUpdate != "" && strings.Contains(path, tierToUpdate+string(filepath.Separator)) && filepath.Base(path) != "tier.yaml" {
			file = []byte(strings.Replace(string(file), "metadata:", `metadata:
  annotations:
    modified-by: "test"`, 1))
		}
		return os.WriteFile(newPath, file, 0600)
	})
	require.NoError(t, err)
}

func verifyTierTemplate(t *testing.T, outDir, tierName, typeName, sourceFile, oldTemplateRef string) string {
	tierTemplates, err := inKStructure(t, outDir, "").listObjects(tierName, "TierTemplate", &toolchainv1alpha1.TierTemplate{})
	require.NoError(t, err)

	currentTemplateRef := ""
	oldtemplateFileFound := false
	for _, templateObj := range tierTemplates {
		template := templateObj.(*toolchainv1alpha1.TierTemplate)
		if template.Spec.TierName == tierName && template.Spec.Type == typeName {
			if oldTemplateRef != template.Name {
				splitName := strings.Split(template.Name[len(tierName)+1:], "-")
				require.Len(t, splitName, 3)
				assert.Equal(t, tierName, template.Name[:len(tierName)])
				assert.Equal(t, typeName, splitName[0])
				assert.Equal(t, fmt.Sprintf("%s-%s", splitName[1], splitName[2]), template.Spec.Revision)
				assert.Equal(t, tierName, template.Spec.TierName)
				assert.Equal(t, typeName, template.Spec.Type)
				assert.Equal(t, commontest.HostOperatorNs, template.Namespace)
				assert.NotEmpty(t, template.Spec.Template)
				assert.NotEmpty(t, template.Spec.Template.Name)

				if tierName != "appstudiolarge" {
					sourceTemplateContent, err := os.ReadFile(sourceFile)
					require.NoError(t, err)
					sourceTemplate := &v1.Template{}
					err = yaml.Unmarshal(sourceTemplateContent, sourceTemplate)
					require.NoError(t, err)
					assert.Equal(t, *sourceTemplate, template.Spec.Template)
				}

				currentTemplateRef = template.Name
			} else if oldTemplateRef != "" {
				oldtemplateFileFound = true
			}
		}
	}
	if currentTemplateRef == "" {
		require.Fail(t, fmt.Sprintf("the TierTemplate for NSTemplateTier '%s' and of the type '%s' wasn't found in dir '%s'", tierName, typeName, outDir))
	}
	if oldTemplateRef != "" && !oldtemplateFileFound {
		require.Fail(t, fmt.Sprintf("the old TierTemplate '%s' for NSTemplateTier '%s' and of the type '%s' wasn't found in dir '%s'", oldTemplateRef, tierName, typeName, outDir))
	}
	return currentTemplateRef
}
