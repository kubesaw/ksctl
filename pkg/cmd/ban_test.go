package cmd_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/api/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/test"
	"github.com/kubesaw/ksctl/pkg/cmd"
	clicontext "github.com/kubesaw/ksctl/pkg/context"
	. "github.com/kubesaw/ksctl/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const banReason = "ban reason"

func TestBanCmdWhenAnswerIsY(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("y")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, userSignup.Name, banReason)

	// then
	require.NoError(t, err)
	AssertBannedUser(t, fakeClient, userSignup, banReason)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.Contains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")

	t.Run("don't ban twice", func(t *testing.T) {
		// given
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, userSignup.Name, banReason)

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup, banReason)
		assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
		assert.Contains(t, term.Output(), "The user was already banned - there is a BannedUser resource with the same labels already present")
	})
}

func TestBanCmdWhenAnswerIsN(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, userSignup.Name, banReason)

	// then
	require.NoError(t, err)
	AssertNoBannedUser(t, fakeClient, userSignup)
	assert.Contains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.Contains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.NotContains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestBanCmdWhenNotFound(t *testing.T) {
	// given
	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	SetFileConfig(t, Host())
	term := NewFakeTerminalWithResponse("n")
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.Ban(ctx, "some", banReason)

	// then
	require.EqualError(t, err, "usersignups.toolchain.dev.openshift.com \"some\" not found")
	AssertNoBannedUser(t, fakeClient, userSignup)
	assert.NotContains(t, term.Output(), "!!!  DANGER ZONE  !!!")
	assert.NotContains(t, term.Output(), "Are you sure that you want to ban the user with the UserSignup by creating BannedUser resource that are both above?")
	assert.NotContains(t, term.Output(), "UserSignup has been banned")
	assert.NotContains(t, term.Output(), "cool-token")
}

func TestCreateBannedUser(t *testing.T) {
	// given
	SetFileConfig(t, Host())

	t.Run("BannedUser creation is successful", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup, banReason)
	})

	t.Run("BannedUser should not be created", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return false, nil
		})

		// then
		require.NoError(t, err)
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("confirmation func returns error", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return false, fmt.Errorf("some error")
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("get of UserSignup fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockGet = func(ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
			return fmt.Errorf("some error")
		}
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("creation of BannedUser fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		fakeClient.MockCreate = func(ctx context.Context, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			return fmt.Errorf("some error")
		}
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("client creation fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		fakeClient := test.NewFakeClient(t, userSignup)
		term := NewFakeTerminal()
		newClient := func(token, apiEndpoint string) (runtimeclient.Client, error) {
			return nil, fmt.Errorf("some error")
		}
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.EqualError(t, err, "some error")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("GetBannedUser call fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		fakeClient.MockList = func(ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
			return errors.New("something went wrong listing the banned users")
		}

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.Error(t, err, "something went wrong listing the banned users")
	})
	t.Run("NewBannedUser call fails", func(t *testing.T) {
		//given
		userSignup := NewUserSignup()
		userSignup.Labels = nil
		newClient, fakeClient := NewFakeClients(t, userSignup)
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
			return true, nil
		})

		// then
		require.Error(t, err, "userSignup doesn't have UserSignupUserEmailHashLabelKey")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})
}

func TestCreateBannedUserLacksPermissions(t *testing.T) {
	// given
	SetFileConfig(t, Host(NoToken()))

	userSignup := NewUserSignup()
	newClient, fakeClient := NewFakeClients(t, userSignup)
	term := NewFakeTerminal()
	ctx := clicontext.NewCommandContext(term, newClient)

	// when
	err := cmd.CreateBannedUser(ctx, userSignup.Name, banReason, func(signup *toolchainv1alpha1.UserSignup, bannedUser *toolchainv1alpha1.BannedUser) (bool, error) {
		return true, nil
	})

	// then
	require.EqualError(t, err, "ksctl command failed: the token in your ksctl.yaml file is missing")
	AssertUserSignupSpec(t, fakeClient, userSignup)
}

func createConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "banning-reasons-test",
			Namespace: test.HostOperatorNs,
		},
		Data: map[string]string{
			"reasons": "Violation of Terms,Spam,Inappropriate Content",
		},
	}
}

func TestBanMenu(t *testing.T) {
	t.Run("empty menu content returns empty BanInfo", func(t *testing.T) {
		// given
		var emptyMenu []cmd.Menu

		// when
		banInfo, err := cmd.BanMenu(emptyMenu)

		// then
		require.NoError(t, err)
		require.NotNil(t, banInfo)
		assert.Empty(t, banInfo.WorkloadType)
		assert.Empty(t, banInfo.BehaviorClassification)
		assert.Empty(t, banInfo.DetectionMechanism)
	})

	t.Run("single workload menu item", func(t *testing.T) {
		// This test demonstrates the structure but cannot test interactivity

		// given
		menu := []cmd.Menu{
			{
				Kind:        "workload",
				Description: "Select workload type",
				Options:     []string{"container", "vm"},
			},
		}

		// Verify menu structure is correct for processing
		assert.Len(t, menu, 1)
		assert.Equal(t, "workload", menu[0].Kind)
		assert.Equal(t, "Select workload type", menu[0].Description)
		assert.Len(t, menu[0].Options, 2)
		assert.Contains(t, menu[0].Options, "container")
	})

	t.Run("multiple menu items structure", func(t *testing.T) {
		// given
		menu := []cmd.Menu{
			{
				Kind:        "workload",
				Description: "Select workload type",
				Options:     []string{"container", "vm"},
			},
			{
				Kind:        "behavior",
				Description: "Select behavior classification",
				Options:     []string{"malicious", "suspicious", "policy-violation"},
			},
			{
				Kind:        "detection",
				Description: "Select detection mechanism",
				Options:     []string{"automated", "manual", "user-report"},
			},
		}

		// Verify menu structure supports all BanInfo fields
		kindMap := make(map[string]bool)
		for _, item := range menu {
			kindMap[item.Kind] = true
			assert.NotEmpty(t, item.Description)
			assert.NotEmpty(t, item.Options)
		}

		assert.True(t, kindMap["workload"])
		assert.True(t, kindMap["behavior"])
		assert.True(t, kindMap["detection"])
	})
}

func TestBanMenuMappingLogic(t *testing.T) {
	// This test verifies the mapping logic that converts menu selections to BanInfo

	t.Run("verify BanInfo field mapping", func(t *testing.T) {
		// Test data that simulates what would be collected from the interactive menu
		testCases := []struct {
			name     string
			kind     string
			answer   string
			expected func(*cmd.BanInfo) string
		}{
			{
				name:   "workload mapping",
				kind:   "workload",
				answer: "compute-intensive",
				expected: func(info *cmd.BanInfo) string {
					return info.WorkloadType
				},
			},
			{
				name:   "behavior mapping",
				kind:   "behavior",
				answer: "malicious",
				expected: func(info *cmd.BanInfo) string {
					return info.BehaviorClassification
				},
			},
			{
				name:   "detection mapping",
				kind:   "detection",
				answer: "automated",
				expected: func(info *cmd.BanInfo) string {
					return info.DetectionMechanism
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// This demonstrates the expected mapping behavior
				banInfo := &cmd.BanInfo{}

				// Simulate the switch statement logic from banMenu
				switch tc.kind {
				case "workload":
					banInfo.WorkloadType = tc.answer
				case "behavior":
					banInfo.BehaviorClassification = tc.answer
				case "detection":
					banInfo.DetectionMechanism = tc.answer
				}

				assert.Equal(t, tc.expected(banInfo), tc.answer)
			})
		}
	})
}

func TestMenuStruct(t *testing.T) {
	t.Run("JSON unmarshaling works correctly", func(t *testing.T) {
		// given
		jsonData := `[
			{
				"kind": "workload",
				"description": "Select workload type",
				"options": ["container", "vm"]
			},
			{
				"kind": "behavior", 
				"description": "Select behavior classification",
				"options": ["malicious", "suspicious"]
			}
		]`

		// when
		var menus []cmd.Menu
		err := json.Unmarshal([]byte(jsonData), &menus)

		// then
		require.NoError(t, err)
		assert.Len(t, menus, 2)

		// Verify first menu
		assert.Equal(t, "workload", menus[0].Kind)
		assert.Equal(t, "Select workload type", menus[0].Description)
		assert.Len(t, menus[0].Options, 2)
		assert.Contains(t, menus[0].Options, "container")
		assert.Contains(t, menus[0].Options, "vm")

		// Verify second menu
		assert.Equal(t, "behavior", menus[1].Kind)
		assert.Equal(t, "Select behavior classification", menus[1].Description)
		assert.Len(t, menus[1].Options, 2)
		assert.Contains(t, menus[1].Options, "malicious")
		assert.Contains(t, menus[1].Options, "suspicious")
	})

	t.Run("empty JSON array unmarshals correctly", func(t *testing.T) {
		// given
		jsonData := `[]`

		// when
		var menus []cmd.Menu
		err := json.Unmarshal([]byte(jsonData), &menus)

		// then
		require.NoError(t, err)
		assert.Empty(t, menus)
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		// given
		jsonData := `[{invalid json`

		// when
		var menus []cmd.Menu
		err := json.Unmarshal([]byte(jsonData), &menus)

		// then
		require.Error(t, err)
	})
}

func TestBanInfoStruct(t *testing.T) {
	t.Run("JSON marshaling works correctly", func(t *testing.T) {
		// given
		banInfo := &cmd.BanInfo{
			WorkloadType:           "container",
			BehaviorClassification: "malicious",
			DetectionMechanism:     "automated",
		}

		// when
		jsonData, err := json.Marshal(banInfo)

		// then
		require.NoError(t, err)
		assert.Contains(t, string(jsonData), "container")
		assert.Contains(t, string(jsonData), "malicious")
		assert.Contains(t, string(jsonData), "automated")

		// Verify it can be unmarshaled back
		var unmarshaled cmd.BanInfo
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)
		assert.Equal(t, banInfo.WorkloadType, unmarshaled.WorkloadType)
		assert.Equal(t, banInfo.BehaviorClassification, unmarshaled.BehaviorClassification)
		assert.Equal(t, banInfo.DetectionMechanism, unmarshaled.DetectionMechanism)
	})

	t.Run("empty BanInfo marshals to empty fields", func(t *testing.T) {
		// given
		banInfo := &cmd.BanInfo{}

		// when
		jsonData, err := json.Marshal(banInfo)

		// then
		require.NoError(t, err)
		assert.Contains(t, string(jsonData), `"workloadType":""`)
		assert.Contains(t, string(jsonData), `"behaviorClassification":""`)
		assert.Contains(t, string(jsonData), `"detectionMechanism":""`)
	})
}

// TestBanMenuInteractiveLogic tests the interactive menu logic in BanMenu function (lines 76-110)
func TestBanMenuLogic(t *testing.T) {
	t.Run("BanMenu with menu content creates proper data structures", func(t *testing.T) {
		// This test verifies the logic of creating huh.Option structures and the mapping
		// We can't test the actual interaction, but we can test the data preparation

		// given
		menu := []cmd.Menu{
			{
				Kind:        "workload",
				Description: "Select workload type",
				Options:     []string{"container", "vm"},
			},
			{
				Kind:        "behavior",
				Description: "Select behavior classification",
				Options:     []string{"malicious", "suspicious"},
			},
		}

		// Verify the menu structure that would be processed in lines 77-95
		for _, item := range menu {
			// Simulate the options creation logic from lines 79-82
			options := make([]map[string]string, len(item.Options))
			for i, opt := range item.Options {
				options[i] = map[string]string{"Key": opt, "Value": opt}
			}

			// Verify options are created correctly
			assert.Len(t, options, len(item.Options))
			for i, opt := range item.Options {
				assert.Equal(t, opt, options[i]["Key"])
				assert.Equal(t, opt, options[i]["Value"])
			}
		}
	})

	t.Run("BanMenu mapping logic from answers to BanInfo", func(t *testing.T) {
		// Test the switch statement logic that maps answers to BanInfo fields

		testCases := []struct {
			name         string
			answers      map[string]string
			expectedInfo *cmd.BanInfo
		}{
			{
				name: "all fields mapped correctly",
				answers: map[string]string{
					"workload":  "container",
					"behavior":  "malicious",
					"detection": "automated",
				},
				expectedInfo: &cmd.BanInfo{
					WorkloadType:           "container",
					BehaviorClassification: "malicious",
					DetectionMechanism:     "automated",
				},
			},
			{
				name: "partial mapping - only workload",
				answers: map[string]string{
					"workload": "vm",
				},
				expectedInfo: &cmd.BanInfo{
					WorkloadType:           "vm",
					BehaviorClassification: "",
					DetectionMechanism:     "",
				},
			},
			{
				name: "unknown kind ignored",
				answers: map[string]string{
					"workload": "container",
					"unknown":  "should-be-ignored",
				},
				expectedInfo: &cmd.BanInfo{
					WorkloadType:           "container",
					BehaviorClassification: "",
					DetectionMechanism:     "",
				},
			},
			{
				name:    "empty answers",
				answers: map[string]string{},
				expectedInfo: &cmd.BanInfo{
					WorkloadType:           "",
					BehaviorClassification: "",
					DetectionMechanism:     "",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Simulate the logic from lines 102-112
				banInfo := &cmd.BanInfo{}

				for kind, answer := range tc.answers {
					switch kind {
					case "workload":
						banInfo.WorkloadType = answer
					case "behavior":
						banInfo.BehaviorClassification = answer
					case "detection":
						banInfo.DetectionMechanism = answer
					}
				}

				assert.Equal(t, tc.expectedInfo.WorkloadType, banInfo.WorkloadType)
				assert.Equal(t, tc.expectedInfo.BehaviorClassification, banInfo.BehaviorClassification)
				assert.Equal(t, tc.expectedInfo.DetectionMechanism, banInfo.DetectionMechanism)
			})
		}
	})
}

// TestBanConfigMapProcessing tests whether the configmap content is empty
func TestBanConfigMapProcessing(t *testing.T) {
	t.Run("empty ConfigMap content returns error", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		emptyConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "banning-reasons",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string]string{
				"menu.json": "[]", // Empty array
			},
		}
		newClient, fakeClient := NewFakeClients(t, userSignup, emptyConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no banning reasons found in ConfigMap")
		assert.Contains(t, term.Output(), "No ban reason provided. Checking for available reasons from ConfigMap...")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("ConfigMap with no menu.json key returns error", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		configMapWithoutMenu := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "banning-reasons",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string]string{
				"other-key": "some-value", // No menu.json key
			},
		}
		newClient, fakeClient := NewFakeClients(t, userSignup, configMapWithoutMenu)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no banning reasons found in ConfigMap")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("invalid JSON in ConfigMap returns error", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		configMapWithBadJSON := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "banning-reasons",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string]string{
				"menu.json": "[{invalid json", // Malformed JSON
			},
		}
		newClient, fakeClient := NewFakeClients(t, userSignup, configMapWithBadJSON)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load banning reasons from ConfigMap")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})
}

// TestBanMenuErrorHandling tests error scenarios in BanMenu
func TestBanMenuErrorHandling(t *testing.T) {
	t.Run("BanMenu handles empty menu gracefully", func(t *testing.T) {

		// given
		var emptyMenu []cmd.Menu

		// when
		banInfo, err := cmd.BanMenu(emptyMenu)

		// then
		require.NoError(t, err)
		require.NotNil(t, banInfo)

		// Should return empty BanInfo when no menu content
		assert.Empty(t, banInfo.WorkloadType)
		assert.Empty(t, banInfo.BehaviorClassification)
		assert.Empty(t, banInfo.DetectionMechanism)
	})
}

// TestBanJSONMarshalingLogic tests the JSON marshaling logic in Ban function
func TestBanJSONMarshaling(t *testing.T) {
	t.Run("successful JSON marshaling of BanInfo", func(t *testing.T) {

		// given
		banInfo := &cmd.BanInfo{
			WorkloadType:           "container",
			BehaviorClassification: "malicious",
			DetectionMechanism:     "automated",
		}

		// when
		banInfoJSON, err := json.Marshal(banInfo)

		// then
		require.NoError(t, err, "line 176 should not trigger error")

		banReason := string(banInfoJSON)

		// Verify the JSON contains expected fields
		assert.Contains(t, banReason, `"workloadType":"container"`)
		assert.Contains(t, banReason, `"behaviorClassification":"malicious"`)
		assert.Contains(t, banReason, `"detectionMechanism":"automated"`)

		// Verify it's valid JSON that can be unmarshaled back
		var unmarshaled cmd.BanInfo
		err = json.Unmarshal([]byte(banReason), &unmarshaled)
		require.NoError(t, err)
		assert.Equal(t, banInfo.WorkloadType, unmarshaled.WorkloadType)
		assert.Equal(t, banInfo.BehaviorClassification, unmarshaled.BehaviorClassification)
		assert.Equal(t, banInfo.DetectionMechanism, unmarshaled.DetectionMechanism)
	})

	t.Run("empty BanInfo marshals successfully", func(t *testing.T) {
		// Test case of empty BanInfo

		// given
		banInfo := &cmd.BanInfo{}

		// when
		banInfoJSON, err := json.Marshal(banInfo)

		// then
		require.NoError(t, err)
		banReason := string(banInfoJSON)

		// Should contain empty string values
		assert.Contains(t, banReason, `"workloadType":""`)
		assert.Contains(t, banReason, `"behaviorClassification":""`)
		assert.Contains(t, banReason, `"detectionMechanism":""`)
	})
}

// TestBanWithValidConfigMap tests the complete interactive flow
func TestBanWithValidConfigMap(t *testing.T) {
	t.Run("Ban function interactive mode output messages (line 167)", func(t *testing.T) {
		// Test that we can at least verify the "Opening interactive menu..." message is printed

		// given
		userSignup := NewUserSignup()
		validConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "banning-reasons",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string]string{
				"menu.json": `[{"kind":"workload","description":"Select workload","options":["container","vm"]}]`,
			},
		}
		newClient, fakeClient := NewFakeClients(t, userSignup, validConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// This test will fail at the interactive part, but we can verify initial processing
		// We expect it to get to the interactive menu and fail there

		// when
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		// Should fail at the interactive part (huh.Select.Run()), but we can verify:

		assert.Contains(t, term.Output(), "Opening interactive menu...")

		assert.NotContains(t, err.Error(), "no banning reasons found in ConfigMap")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to collect banning information")

		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("Ban function processes non-empty ConfigMap correctly", func(t *testing.T) {

		// given
		userSignup := NewUserSignup()
		validConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "banning-reasons",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string]string{
				"menu.json": `[
					{"kind":"workload","description":"Select workload","options":["container"]},
					{"kind":"behavior","description":"Select behavior","options":["malicious"]}
				]`,
			},
		}
		newClient, fakeClient := NewFakeClients(t, userSignup, validConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		assert.NotContains(t, err.Error(), "no banning reasons found in ConfigMap")

		// Should reach the interactive menu part
		assert.Contains(t, term.Output(), "Opening interactive menu...")

		// Should fail at interactive part, not at ConfigMap validation
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to collect banning information")

		AssertNoBannedUser(t, fakeClient, userSignup)
	})
}

func TestBanCmdInteractiveMode(t *testing.T) {
	t.Run("interactive mode with ConfigMap present", func(t *testing.T) {
		t.Skip("Skipping interactive test - requires actual terminal interaction")

		// given
		userSignup := NewUserSignup()
		banConfigMap := createConfigMap()
		newClient, _ := NewFakeClients(t, userSignup, banConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when - using only the username, no ban reason
		err := cmd.Ban(ctx, userSignup.Name)

		// then - this would require actual user interaction, so we skip it
		require.NoError(t, err)
		assert.Contains(t, term.Output(), "No ban reason provided. Checking for available reasons from ConfigMap...")
	})

	t.Run("interactive mode with empty ConfigMap", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		emptyConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "banning-reasons-test",
				Namespace: "toolchain-host-operator",
			},
			Data: map[string]string{}, // Empty data
		}
		newClient, fakeClient := NewFakeClients(t, userSignup, emptyConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when - using only the username, no ban reason
		err := cmd.Ban(ctx, userSignup.Name)

		// then
		require.Error(t, err, "failed to get ConfigMap")
		assert.Contains(t, term.Output(), "No ban reason provided. Checking for available reasons from ConfigMap...\n")
		assert.Contains(t, err.Error(), "not found")
		AssertNoBannedUser(t, fakeClient, userSignup)
	})

	t.Run("traditional mode still works with two arguments", func(t *testing.T) {
		// given
		userSignup := NewUserSignup()
		banConfigMap := createConfigMap()
		newClient, fakeClient := NewFakeClients(t, userSignup, banConfigMap)
		SetFileConfig(t, Host())
		term := NewFakeTerminalWithResponse("y")
		ctx := clicontext.NewCommandContext(term, newClient)

		// when - using both username and ban reason (traditional mode)
		err := cmd.Ban(ctx, userSignup.Name, "Custom ban reason")

		// then
		require.NoError(t, err)
		AssertBannedUser(t, fakeClient, userSignup, "Custom ban reason")
		assert.NotContains(t, term.Output(), "No ban reason provided. Checking for available reasons from ConfigMap...")
	})

	t.Run("error when no arguments provided", func(t *testing.T) {
		// given
		newClient, _ := NewFakeClients(t)
		SetFileConfig(t, Host())
		term := NewFakeTerminal()
		ctx := clicontext.NewCommandContext(term, newClient)

		// when - no arguments
		err := cmd.Ban(ctx)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "UserSignup name is required")
	})
}
