package upgrade_kyma

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/common/hyperscaler"
	hyperscalerautomock "github.com/kyma-project/control-plane/components/kyma-environment-broker/common/hyperscaler/automock"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/common/hyperscaler/azure"
	azuretesting "github.com/kyma-project/control-plane/components/kyma-environment-broker/common/hyperscaler/azure/testing"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/storage"
)

const (
	subAccountID   = "12df5747-3efb-4df6-ad6f-4414bb661ce3"
	fixOperationID = "17f3ddba-1132-466d-a3c5-920f544d7ea6"
)

type wantStateFunction = func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error,
	azureClient azuretesting.FakeNamespaceClient)

func Test_StepsDeprovisionSucceeded(t *testing.T) {
	tests := []struct {
		name                string
		giveOperation       func() internal.UpgradeKymaOperation
		giveSteps           func(t *testing.T, memoryStorageOp storage.Operations, instanceStorage storage.Instances, accountProvider *hyperscalerautomock.AccountProvider) []DeprovisionAzureEventHubStep
		wantRepeatOperation bool
		wantStates          func(t *testing.T) []wantStateFunction
	}{
		{
			// 1. a ResourceGroup exists before we call the deprovisioning step
			// 2. resourceGroup is in deletion state during retry wait time before we call the deprovisioning step again
			// 3. expectation is that no new deprovisioning is triggered
			// 4. after calling step again - expectation is that the deprovisioning succeeded now
			name:          "ResourceGroupInDeletionMode",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveSteps: func(t *testing.T, memoryStorageOp storage.Operations, instanceStorage storage.Instances, accountProvider *hyperscalerautomock.AccountProvider) []DeprovisionAzureEventHubStep {
				namespaceClientResourceGroupExists := azuretesting.NewFakeNamespaceClientResourceGroupExists()
				namespaceClientResourceGroupInDeletionMode := azuretesting.NewFakeNamespaceClientResourceGroupInDeletionMode()
				namespaceClientResourceGroupDoesNotExist := azuretesting.NewFakeNamespaceClientResourceGroupDoesNotExist()

				stepResourceGroupExists := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClientResourceGroupExists), accountProvider)
				stepResourceGroupInDeletionMode := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClientResourceGroupInDeletionMode), accountProvider)
				stepResourceGroupDoesNotExist := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClientResourceGroupDoesNotExist), accountProvider)

				return []DeprovisionAzureEventHubStep{
					stepResourceGroupExists,
					stepResourceGroupInDeletionMode,
					stepResourceGroupDoesNotExist,
				}
			},
			wantStates: func(t *testing.T) []wantStateFunction {
				return []wantStateFunction{
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						ensureOperationIsRepeated(t, operation, when, err)
					},
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						assert.False(t, azureClient.DeleteResourceGroupCalled)
						ensureOperationIsRepeated(t, operation, when, err)
					},
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						ensureOperationSuccessful(t, operation, when, err)
					},
				}
			},
		},
		{
			// Idea:
			// 1. a ResourceGroup exists before we call the deprovisioning step
			// 2. resourceGroup got deleted during retry wait time before we call the deprovisioning step again
			// 3. expectation is that the deprovisioning succeeded now
			name:          "ResourceGroupExists",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveSteps: func(t *testing.T, memoryStorageOp storage.Operations, instanceStorage storage.Instances, accountProvider *hyperscalerautomock.AccountProvider) []DeprovisionAzureEventHubStep {

				namespaceClientResourceGroupExists := azuretesting.NewFakeNamespaceClientResourceGroupExists()
				namespaceClientResourceGroupDoesNotExist := azuretesting.NewFakeNamespaceClientResourceGroupDoesNotExist()

				stepResourceGroupExists := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClientResourceGroupExists), accountProvider)
				stepResourceGroupDoesNotExist := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClientResourceGroupDoesNotExist), accountProvider)
				return []DeprovisionAzureEventHubStep{
					stepResourceGroupExists,
					stepResourceGroupDoesNotExist,
				}
			},
			wantStates: func(t *testing.T) []wantStateFunction {
				return []wantStateFunction{
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						ensureOperationIsRepeated(t, operation, when, err)
					},
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						ensureOperationSuccessful(t, operation, when, err)
					},
				}
			},
		},
		{

			// Idea:
			// 1. a ResourceGroup does not exist before we call the deprovisioning step
			// 2. expectation is that the deprovisioning succeeded
			name:          "ResourceGroupDoesNotExist",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveSteps: func(t *testing.T, memoryStorageOp storage.Operations, instanceStorage storage.Instances, accountProvider *hyperscalerautomock.AccountProvider) []DeprovisionAzureEventHubStep {
				namespaceClient := azuretesting.NewFakeNamespaceClientResourceGroupDoesNotExist()
				step := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClient), accountProvider)

				return []DeprovisionAzureEventHubStep{
					step,
				}
			},
			wantStates: func(t *testing.T) []wantStateFunction {
				return []wantStateFunction{
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						ensureOperationSuccessful(t, operation, when, err)
					},
				}
			},
		},
		{
			name:          "Operation Event Hub already deleted",
			giveOperation: fixDeprovisioningOperationWithDeletedEventHub,
			giveSteps: func(t *testing.T, memoryStorageOp storage.Operations, instanceStorage storage.Instances, accountProvider *hyperscalerautomock.AccountProvider) []DeprovisionAzureEventHubStep {
				namespaceClient := azuretesting.NewFakeNamespaceClientResourceGroupDoesNotExist()
				step := fixEventHubStep(memoryStorageOp, instanceStorage, azuretesting.NewFakeHyperscalerProvider(namespaceClient), accountProvider)
				return []DeprovisionAzureEventHubStep{
					step,
				}
			},
			wantStates: func(t *testing.T) []wantStateFunction {
				return []wantStateFunction{
					func(t *testing.T, operation internal.UpgradeKymaOperation, when time.Duration, err error, azureClient azuretesting.FakeNamespaceClient) {
						ensureOperationSuccessful(t, operation, when, err)
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			memoryStorage := storage.NewMemoryStorage()
			accountProvider := fixAccountProvider()
			op := tt.giveOperation()
			// this is required to avoid storage retries (without this statement there will be an error => retry)
			err := memoryStorage.Operations().InsertUpgradeKymaOperation(op)
			require.NoError(t, err)
			err = memoryStorage.Instances().Insert(fixInstance())
			require.NoError(t, err)
			steps := tt.giveSteps(t, memoryStorage.Operations(), memoryStorage.Instances(), accountProvider)
			wantStates := tt.wantStates(t)
			for idx, step := range steps {
				// when
				op.UpdatedAt = time.Now()
				op, when, err := step.Run(op, fixLogger())
				require.NoError(t, err)

				fakeHyperscalerProvider, ok := step.HyperscalerProvider.(*azuretesting.FakeHyperscalerProvider)
				require.True(t, ok)
				fakeAzureClient, ok := fakeHyperscalerProvider.Client.(*azuretesting.FakeNamespaceClient)
				require.True(t, ok)

				// then
				wantStates[idx](t, op, when, err, *fakeAzureClient)
			}
		})
	}
}

func Test_StepsUnhappyPath(t *testing.T) {
	tests := []struct {
		name                string
		giveOperation       func() internal.UpgradeKymaOperation
		giveInstance        func() internal.Instance
		giveStep            func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep
		wantRepeatOperation bool
	}{
		{
			name:          "Operation already deprovisioned eventhub",
			giveOperation: fixDeprovisioningOperationWithDeletedEventHub,
			giveInstance:  fixInvalidInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProvider()
				return fixEventHubStep(storage.Operations(), storage.Instances(), azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientHappyPath()), accountProvider)
			},
			wantRepeatOperation: false,
		},
		{
			name:          "Operation provision parameter errors",
			giveOperation: fixDeprovisioningOperation,
			giveInstance:  fixInvalidInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProvider()
				return fixEventHubStep(storage.Operations(), storage.Instances(), azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientHappyPath()), accountProvider)
			},
			wantRepeatOperation: false,
		},
		{
			name:          "AccountProvider cannot get gardener credentials",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProviderGardenerCredentialsError()
				return fixEventHubStep(storage.Operations(), storage.Instances(), azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientHappyPath()), accountProvider)
			},
			wantRepeatOperation: true,
		},
		{
			name:          "Error while getting EventHubs Namespace credentials",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProviderGardenerCredentialsError()
				return NewDeprovisionAzureEventHubStep(storage.Operations(),
					// ups ... namespace cannot get listed
					azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientListError()),
					accountProvider,
					context.Background(),
				)
			},
			wantRepeatOperation: true,
		},
		{
			name:          "Error while getting config from Credentials",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProviderGardenerCredentialsHAPError()
				return NewDeprovisionAzureEventHubStep(storage.Operations(),
					azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceAccessKeysNil()),
					accountProvider,
					context.Background(),
				)
			},
			wantRepeatOperation: false,
		},
		{
			name:          "Error while getting client from HAP",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProvider()
				return NewDeprovisionAzureEventHubStep(storage.Operations(),
					// ups ... client cannot be created
					azuretesting.NewFakeHyperscalerProviderError(),
					accountProvider,
					context.Background(),
				)
			},
			wantRepeatOperation: false,
		},
		{
			name:          "Error while getting resource group",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProvider()
				return NewDeprovisionAzureEventHubStep(storage.Operations(),
					// ups ... can't get resource group
					azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientResourceGroupConnectionError()),
					accountProvider,
					context.Background(),
				)
			},
			wantRepeatOperation: true,
		},
		{
			name:          "Error while deleting resource group",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProvider()
				return NewDeprovisionAzureEventHubStep(storage.Operations(),
					// ups ... can't delete resource group
					azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientResourceGroupDeleteError()),
					accountProvider,
					context.Background(),
				)
			},
			wantRepeatOperation: true,
		},
		{
			name:          "Resource group properties is Nil",
			giveOperation: fixDeprovisioningOperationWithParameters,
			giveInstance:  fixInstance,
			giveStep: func(t *testing.T, storage storage.BrokerStorage) DeprovisionAzureEventHubStep {
				accountProvider := fixAccountProvider()
				return NewDeprovisionAzureEventHubStep(storage.Operations(),
					// ups ... can't delete resource group
					azuretesting.NewFakeHyperscalerProvider(azuretesting.NewFakeNamespaceClientResourceGroupPropertiesError()),
					accountProvider,
					context.Background(),
				)
			},
			wantRepeatOperation: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			memoryStorage := storage.NewMemoryStorage()
			op := tt.giveOperation()
			step := tt.giveStep(t, memoryStorage)
			// this is required to avoid storage retries (without this statement there will be an error => retry)
			err := memoryStorage.Operations().InsertUpgradeKymaOperation(op)
			require.NoError(t, err)
			err = memoryStorage.Instances().Insert(tt.giveInstance())
			require.NoError(t, err)

			// when
			op.UpdatedAt = time.Now()
			op, when, err := step.Run(op, fixLogger())
			require.NotNil(t, op)

			// then
			if tt.wantRepeatOperation {
				ensureOperationIsRepeated(t, op, when, err)
			} else {
				ensureOperationIsNotRepeated(t, err)
			}
		})
	}
}

func fixInstance() internal.Instance {
	var pp2 internal.ProvisioningParameters
	json.Unmarshal([]byte(
		`{
			"plan_id": "4deee563-e5ec-4731-b9b1-53b42d855f0c",
			"ers_context": {
				"subaccount_id": "`+subAccountID+`"
			},
			"parameters": {
				"name": "nachtmaar-15",
				"components": [],
				"region": "westeurope"
			}
		}`), &pp2)
	return internal.Instance{
		InstanceID: fixInstanceID,
		Parameters: pp2}
}

func fixInvalidInstance() internal.Instance {
	var pp2 internal.ProvisioningParameters
	json.Unmarshal([]byte(`}{INVALID JSON}{`), &pp2)
	return internal.Instance{
		InstanceID: fixInstanceID,
		Parameters: pp2}
}

func fixAccountProvider() *hyperscalerautomock.AccountProvider {
	accountProvider := hyperscalerautomock.AccountProvider{}
	accountProvider.On("GardenerCredentials", hyperscaler.Azure, mock.Anything).Return(hyperscaler.Credentials{
		HyperscalerType: hyperscaler.Azure,
		CredentialData: map[string][]byte{
			"subscriptionID": []byte("subscriptionID"),
			"clientID":       []byte("clientID"),
			"clientSecret":   []byte("clientSecret"),
			"tenantID":       []byte("tenantID"),
		},
	}, nil)
	return &accountProvider
}

func fixEventHubStep(memoryStorageOp storage.Operations, instanceStorage storage.Instances, hyperscalerProvider azure.HyperscalerProvider,
	accountProvider *hyperscalerautomock.AccountProvider) DeprovisionAzureEventHubStep {
	return NewDeprovisionAzureEventHubStep(memoryStorageOp, hyperscalerProvider, accountProvider, context.Background())
}

func fixLogger() logrus.FieldLogger {
	return logrus.StandardLogger()
}

func fixDeprovisioningOperationWithParameters() internal.UpgradeKymaOperation {
	return internal.UpgradeKymaOperation{
		Operation: internal.Operation{
			ID:                     fixOperationID,
			InstanceID:             fixInstanceID,
			ProvisionerOperationID: fixProvisionerOperationID,
			Description:            "",
			UpdatedAt:              time.Now(),
			ProvisioningParameters: internal.ProvisioningParameters{
				PlanID:         "",
				ServiceID:      "",
				ErsContext:     internal.ERSContext{},
				Parameters:     internal.ProvisioningParametersDTO{},
				PlatformRegion: "",
			},
		},
	}
}

func fixDeprovisioningOperation() internal.UpgradeKymaOperation {
	return internal.UpgradeKymaOperation{
		Operation: internal.Operation{
			ID:                     fixOperationID,
			InstanceID:             fixInstanceID,
			ProvisionerOperationID: fixProvisionerOperationID,
			CreatedAt:              time.Now(),
			UpdatedAt:              time.Now(),
		},
	}
}

func fixDeprovisioningOperationWithDeletedEventHub() internal.UpgradeKymaOperation {
	return internal.UpgradeKymaOperation{
		Operation: internal.Operation{InstanceDetails: internal.InstanceDetails{
			EventHub: internal.EventHub{
				Deleted: true,
			},
		}},
	}
}

// operationManager.OperationFailed(...)
// manager.go: if processedOperation.State != domain.InProgress { return 0, nil } => repeat
// queue.go: if err == nil && when != 0 => repeat

func ensureOperationIsRepeated(t *testing.T, op internal.UpgradeKymaOperation, when time.Duration, err error) {
	t.Helper()
	assert.Nil(t, err)
	assert.True(t, when != 0)
	assert.NotEqual(t, op.Operation.State, domain.Succeeded)
}

func ensureOperationIsNotRepeated(t *testing.T, err error) {
	t.Helper()
	assert.Nil(t, err)
}

func ensureOperationSuccessful(t *testing.T, op internal.UpgradeKymaOperation, when time.Duration, err error) {
	t.Helper()
	assert.Equal(t, when, time.Duration(0))
	assert.Equal(t, op.Operation.State, domain.LastOperationState(""))
	assert.Nil(t, err)
}

func fixAccountProviderGardenerCredentialsError() *hyperscalerautomock.AccountProvider {
	accountProvider := hyperscalerautomock.AccountProvider{}
	accountProvider.On("GardenerCredentials", hyperscaler.Azure, mock.Anything).Return(hyperscaler.Credentials{
		HyperscalerType: hyperscaler.Azure,
		CredentialData:  map[string][]byte{},
	}, fmt.Errorf("ups ... gardener credentials could not be retrieved"))
	return &accountProvider
}

func fixAccountProviderGardenerCredentialsHAPError() *hyperscalerautomock.AccountProvider {
	accountProvider := hyperscalerautomock.AccountProvider{}
	accountProvider.On("GardenerCredentials", hyperscaler.Azure, mock.Anything).Return(hyperscaler.Credentials{
		HyperscalerType: hyperscaler.AWS,
		CredentialData: map[string][]byte{
			"subscriptionID": []byte("subscriptionID"),
			"clientID":       []byte("clientID"),
			"clientSecret":   []byte("clientSecret"),
			"tenantID":       []byte("tenantID"),
		},
	}, nil)
	return &accountProvider
}
