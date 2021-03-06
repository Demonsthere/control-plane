package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/ptr"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/storage"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type handler struct {
	Instance   internal.Instance
	ersContext internal.ERSContext
}

func (h *handler) Handle(inst *internal.Instance, ers internal.ERSContext) error {
	h.Instance = *inst
	h.ersContext = ers
	return nil
}

func TestUpdateEndpoint_UpdateSuspension(t *testing.T) {
	// given
	instance := internal.Instance{
		InstanceID:    instanceID,
		ServicePlanID: TrialPlanID,
		Parameters: internal.ProvisioningParameters{
			PlanID: TrialPlanID,
			ErsContext: internal.ERSContext{
				TenantID:        "",
				SubAccountID:    "",
				GlobalAccountID: "",
				ServiceManager:  nil,
				Active:          nil,
			},
		},
	}
	st := storage.NewMemoryStorage()
	st.Instances().Insert(instance)
	st.Operations().InsertProvisioningOperation(fixProvisioningOperation("01"))
	st.Operations().InsertDeprovisioningOperation(fixSuspensionOperation())
	st.Operations().InsertProvisioningOperation(fixProvisioningOperation("02"))

	handler := &handler{}
	svc := NewUpdate(st.Instances(), st.Operations(), handler, true, logrus.New())

	// when
	svc.Update(context.Background(), instanceID, domain.UpdateDetails{
		ServiceID:       "",
		PlanID:          TrialPlanID,
		RawParameters:   nil,
		PreviousValues:  domain.PreviousValues{},
		RawContext:      json.RawMessage("{\"active\":false}"),
		MaintenanceInfo: nil,
	}, true)

	// then
	inst, err := st.Instances().GetByID(instanceID)
	require.NoError(t, err)
	// check if original ERS context is set again in the Instance entity
	assert.NotEmpty(t, inst.Parameters.ErsContext.ServiceManager.Credentials.BasicAuth.Password)
	// check if the handler was called
	assertServiceManagerCreds(t, handler.Instance.Parameters.ErsContext.ServiceManager)

	assert.Equal(t, internal.ERSContext{
		Active: ptr.Bool(false),
	}, handler.ersContext)

	require.NotNil(t, handler.Instance.Parameters.ErsContext.Active)
	assert.True(t, *handler.Instance.Parameters.ErsContext.Active)
}

func TestUpdateEndpoint_UpdateUnsuspension(t *testing.T) {
	// given
	instance := internal.Instance{
		InstanceID:    instanceID,
		ServicePlanID: TrialPlanID,
		Parameters: internal.ProvisioningParameters{
			PlanID: TrialPlanID,
			ErsContext: internal.ERSContext{
				TenantID:        "",
				SubAccountID:    "",
				GlobalAccountID: "",
				ServiceManager:  nil,
				Active:          nil,
			},
		},
	}
	st := storage.NewMemoryStorage()
	st.Instances().Insert(instance)
	st.Operations().InsertProvisioningOperation(fixProvisioningOperation("01"))
	st.Operations().InsertDeprovisioningOperation(fixSuspensionOperation())

	handler := &handler{}
	svc := NewUpdate(st.Instances(), st.Operations(), handler, true, logrus.New())

	// when
	svc.Update(context.Background(), instanceID, domain.UpdateDetails{
		ServiceID:       "",
		PlanID:          TrialPlanID,
		RawParameters:   nil,
		PreviousValues:  domain.PreviousValues{},
		RawContext:      json.RawMessage("{\"active\":true}"),
		MaintenanceInfo: nil,
	}, true)

	// then
	inst, err := st.Instances().GetByID(instanceID)
	require.NoError(t, err)
	// check if original ERS context is set again in the Instance entity
	assert.NotEmpty(t, inst.Parameters.ErsContext.ServiceManager.Credentials.BasicAuth.Password)
	// check if the handler was called
	assertServiceManagerCreds(t, handler.Instance.Parameters.ErsContext.ServiceManager)

	assert.Equal(t, internal.ERSContext{
		Active: ptr.Bool(true),
	}, handler.ersContext)

	require.NotNil(t, handler.Instance.Parameters.ErsContext.Active)
	assert.False(t, *handler.Instance.Parameters.ErsContext.Active)
}

func assertServiceManagerCreds(t *testing.T, dto *internal.ServiceManagerEntryDTO) {
	assert.Equal(t, &internal.ServiceManagerEntryDTO{
		Credentials: internal.ServiceManagerCredentials{
			BasicAuth: internal.ServiceManagerBasicAuth{
				Username: "u",
				Password: "p",
			},
		}}, dto)
}

func TestUpdateEndpoint_UpdateInstanceWithWrongActiveValue(t *testing.T) {
	// given
	instance := internal.Instance{
		InstanceID:    instanceID,
		ServicePlanID: TrialPlanID,
		Parameters: internal.ProvisioningParameters{
			PlanID: TrialPlanID,
			ErsContext: internal.ERSContext{
				TenantID:        "",
				SubAccountID:    "",
				GlobalAccountID: "",
				ServiceManager:  nil,
				Active:          ptr.Bool(false),
			},
		},
	}
	st := storage.NewMemoryStorage()
	st.Instances().Insert(instance)
	st.Operations().InsertProvisioningOperation(fixProvisioningOperation("01"))
	handler := &handler{}
	svc := NewUpdate(st.Instances(), st.Operations(), handler, true, logrus.New())

	// when
	svc.Update(context.Background(), instanceID, domain.UpdateDetails{
		ServiceID:       "",
		PlanID:          TrialPlanID,
		RawParameters:   nil,
		PreviousValues:  domain.PreviousValues{},
		RawContext:      json.RawMessage("{\"active\":false}"),
		MaintenanceInfo: nil,
	}, true)

	// then
	inst, _ := st.Instances().GetByID(instanceID)
	// check if original ERS context is set again in the Instance entity
	assert.NotEmpty(t, inst.Parameters.ErsContext.ServiceManager.Credentials.BasicAuth.Password)
	// check if the handler was called
	assertServiceManagerCreds(t, handler.Instance.Parameters.ErsContext.ServiceManager)
	assert.Equal(t, internal.ERSContext{
		Active: ptr.Bool(false),
	}, handler.ersContext)

	assert.True(t, *handler.Instance.Parameters.ErsContext.Active)
}

func fixProvisioningOperation(id string) internal.ProvisioningOperation {
	return internal.ProvisioningOperation{
		Operation: internal.Operation{
			ID:         id,
			CreatedAt:  time.Now(),
			InstanceID: instanceID,
			ProvisioningParameters: internal.ProvisioningParameters{
				ErsContext: internal.ERSContext{
					ServiceManager: &internal.ServiceManagerEntryDTO{
						Credentials: internal.ServiceManagerCredentials{
							BasicAuth: internal.ServiceManagerBasicAuth{
								Username: "u",
								Password: "p",
							},
						},
					},
				},
			},
		},
	}
}

func fixSuspensionOperation() internal.DeprovisioningOperation {
	return internal.DeprovisioningOperation{
		Operation: internal.Operation{
			CreatedAt:  time.Now(),
			InstanceID: instanceID,
			ProvisioningParameters: internal.ProvisioningParameters{
				ErsContext: internal.ERSContext{
					ServiceManager: &internal.ServiceManagerEntryDTO{
						Credentials: internal.ServiceManagerCredentials{
							BasicAuth: internal.ServiceManagerBasicAuth{
								Username: "u",
								Password: "p",
							},
						},
					},
				},
			},
		},
		Temporary: true,
	}
}
